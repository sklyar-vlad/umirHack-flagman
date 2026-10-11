package events

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

// Cancel from a flushed frame to test a live retention change without socket timers.
type backlogWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w backlogWriter) SetWriteDeadline(time.Time) error { return nil }
func (w backlogWriter) FlushError() error                { return nil }
func (w backlogWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "event:") {
		w.cancel()
	}
	return w.ResponseRecorder.Write(data)
}

type backlogReader struct{ calls int }

func (r *backlogReader) Read(context.Context, int64) (model.EventPage, error) {
	r.calls++
	if r.calls == 1 {
		return model.EventPage{HighWater: 100}, nil
	}
	// Retention moves past the connection's current cursor between two polls.
	return model.EventPage{HighWater: 200, RetentionFloor: 150}, nil
}

type activeSession struct{}

func (activeSession) CurrentSession(context.Context, string) (model.Session, error) {
	return model.Session{Actor: model.Actor{Role: "storekeeper"}}, nil
}

func TestLiveRetentionOvertakesConnectionWithRoleFilteredResync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := httptest.NewRecorder()
	w := backlogWriter{recorder, cancel}
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/events", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "flagman_session", Value: "fake-test-token"})
	c.Request = req.WithContext(ctx)
	h := NewHandler(&backlogReader{}, activeSession{}, ctx)
	h.poll = time.Millisecond
	h.Stream(c)
	body := recorder.Body.String()
	if !strings.Contains(body, "id: 200\nevent: resync.required\n") || strings.Contains(body, "hot-offers") ||
		strings.Contains(body, "orders") ||
		strings.Contains(body, "users") ||
		!strings.Contains(body, "procurement-recommendations") {
		t.Fatal(body)
	}
}

type expiredSession struct{ calls int }

func (s *expiredSession) CurrentSession(context.Context, string) (model.Session, error) {
	s.calls++
	if s.calls > 1 {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	return model.Session{Actor: model.Actor{Role: "admin"}}, nil
}

func TestSessionRevokedBeforeFirstReplayClosesWithoutEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(backlogWriter{recorder, cancel})
	c.Request = httptest.NewRequest(http.MethodGet, "/events", http.NoBody).WithContext(ctx)
	c.Request.AddCookie(&http.Cookie{Name: "flagman_session", Value: "fake-test-token"})
	c.Request.Header.Set("Last-Event-ID", "0")
	session := &expiredSession{}
	h := NewHandler(&backlogReader{}, session, ctx)
	h.poll = time.Millisecond
	h.Stream(c)
	if strings.Contains(recorder.Body.String(), "event:") || session.calls != 2 {
		t.Fatal(recorder.Body.String(), session.calls)
	}
}

type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (deadlineRecorder) SetWriteDeadline(time.Time) error { return nil }
func (deadlineRecorder) FlushError() error                { return nil }

type fullReplay struct{ calls int }

func (r *fullReplay) Read(context.Context, int64) (model.EventPage, error) {
	r.calls++
	p := model.EventPage{HighWater: 100}
	if r.calls == 1 {
		return p, nil
	}
	for cursor := int64(1); cursor <= 100; cursor++ {
		p.Events = append(
			p.Events,
			model.PublishedEvent{
				Cursor:  cursor,
				Type:    model.InventoryChanged,
				Payload: model.OutboxPayload{Topics: []model.EventTopic{model.TopicInventory}},
			},
		)
	}
	return p, nil
}

type revokedDuringReplay struct{ calls int }

func (s *revokedDuringReplay) CurrentSession(context.Context, string) (model.Session, error) {
	s.calls++
	if s.calls > 3 {
		return model.Session{}, apperrors.ErrUnauthorized
	}
	return model.Session{Actor: model.Actor{Role: "admin"}}, nil
}

func TestSessionRevokedWhileDrainingReplayDoesNotFinishPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(deadlineRecorder{recorder})
	c.Request = httptest.NewRequest(http.MethodGet, "/events", http.NoBody).WithContext(ctx)
	c.Request.AddCookie(&http.Cookie{Name: "flagman_session", Value: "test-token"})
	c.Request.Header.Set("Last-Event-ID", "0")
	session := &revokedDuringReplay{}
	h := NewHandler(&fullReplay{}, session, ctx)
	h.poll = time.Nanosecond
	h.Stream(c)
	if frames := strings.Count(recorder.Body.String(), "event: inventory.changed"); frames != 1 || session.calls != 4 {
		t.Fatal("revoked session drained backlog", frames, session.calls)
	}
}
