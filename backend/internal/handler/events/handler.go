package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	eventservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/events"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Reader interface {
	Read(context.Context, int64) (model.EventPage, error)
}
type Sessions interface {
	CurrentSession(context.Context, string) (model.Session, error)
}
type Handler struct {
	reader    Reader
	sessions  Sessions
	lifecycle context.Context
	poll      time.Duration
	heartbeat time.Duration
}

func NewHandler(reader Reader, sessions Sessions, lifecycle context.Context) *Handler {
	return &Handler{reader: reader, sessions: sessions, lifecycle: lifecycle, poll: time.Second, heartbeat: 20 * time.Second}
}

func (h *Handler) Stream(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	stop := context.AfterFunc(h.lifecycle, cancel)
	defer stop()
	cookie, err := c.Request.Cookie(authservice.SessionCookieName)
	if err != nil {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	session, err := h.session(ctx, cookie.Value)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	after, replay, err := cursor(c.Request.Header)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	page, err := h.read(ctx, after)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if replay && after > page.HighWater {
		basehandler.WriteError(c, cursorError())
		return
	}
	if !replay {
		after = page.HighWater
	}
	controller := http.NewResponseController(c.Writer)
	if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	if err := write(controller, c.Writer, ": connected\n\n"); err != nil {
		return
	}
	if after < page.RetentionFloor {
		if err := resync(controller, c.Writer, page.HighWater, session.Actor.Role); err != nil {
			return
		}
		after = page.HighWater
	}
	lastHeartbeat, lastCheck := time.Now(), time.Now()
	heartbeat := func() error {
		if time.Since(lastHeartbeat) < h.heartbeat {
			return nil
		}
		if err := write(controller, c.Writer, ": heartbeat\n\n"); err != nil {
			return err
		}
		lastHeartbeat = time.Now()
		return nil
	}
	ticker := time.NewTicker(h.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		session, err = h.session(ctx, cookie.Value)
		if err != nil {
			return
		}
		lastCheck = time.Now()
		for {
			if ctx.Err() != nil {
				return
			}
			page, err = h.read(ctx, after)
			if err != nil {
				return
			}
			if after < page.RetentionFloor {
				if err := resync(controller, c.Writer, page.HighWater, session.Actor.Role); err != nil {
					return
				}
				after = page.HighWater
				break
			}
			for _, e := range page.Events {
				if time.Since(lastCheck) >= h.poll {
					session, err = h.session(ctx, cookie.Value)
					if err != nil {
						return
					}
					lastCheck = time.Now()
				}
				if ctx.Err() != nil {
					return
				}
				if err := heartbeat(); err != nil {
					return
				}
				payload, ok := eventservice.Filter(e, session.Actor.Role)
				if ok {
					data, err := json.Marshal(payload)
					if err != nil {
						return
					}
					if err := write(controller, c.Writer, fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", e.Cursor, e.Type, data)); err != nil {
						return
					}
				}
				after = e.Cursor
			}
			if err := heartbeat(); err != nil {
				return
			}
			if len(page.Events) < 100 {
				break
			}
			session, err = h.session(ctx, cookie.Value)
			if err != nil {
				return
			}
		}
	}
}
func (h *Handler) read(ctx context.Context, after int64) (model.EventPage, error) {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return h.reader.Read(bounded, after)
}
func (h *Handler) session(ctx context.Context, token string) (model.Session, error) {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return h.sessions.CurrentSession(bounded, token)
}
func write(controller *http.ResponseController, w http.ResponseWriter, data string) error {
	if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, data); err != nil {
		return err
	}
	return controller.Flush()
}

// Zero is a valid start cursor. Gaps caused by sequence rollback are valid too.
// Reject signs, whitespace, duplicate headers, overflow and non-decimal forms.
func cursor(headers http.Header) (int64, bool, error) {
	values := headers.Values("Last-Event-ID")
	if len(values) == 0 {
		return 0, false, nil
	}
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 19 {
		return 0, true, cursorError()
	}
	for _, c := range values[0] {
		if c < '0' || c > '9' {
			return 0, true, cursorError()
		}
	}
	n, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		return 0, true, cursorError()
	}
	return n, true, nil
}
func cursorError() error {
	return &validations.FieldError{Field: "Last-Event-ID", Message: "Ожидается сохранённый неотрицательный cursor"}
}
func resync(controller *http.ResponseController, w http.ResponseWriter, high int64, role string) error {
	p, _ := eventservice.Filter(model.PublishedEvent{Type: model.InventoryTimeChanged, Payload: model.OutboxPayload{
		OccurredAt: time.Now().UTC(), Topics: []model.EventTopic{model.TopicInventory, model.TopicBatches, model.TopicDashboard, model.TopicProducts, model.TopicProductCategories, model.TopicSettings, model.TopicDishes, model.TopicStopList, model.TopicHotOffers, model.TopicOrders, model.TopicSupplies, model.TopicStockMovements, model.TopicProcurementRecommendations, model.TopicUsers}}}, role)
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return write(controller, w, fmt.Sprintf("id: %d\nevent: resync.required\ndata: %s\n\n", high, data))
}
