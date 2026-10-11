package events

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	authrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	eventrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	eventservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/events"
	"go.uber.org/zap"
)

func (f *fixture) stream(
	t *testing.T,
	role string,
) (*httptest.Server, *eventservice.Service, string, context.CancelFunc) {
	t.Helper()
	user := uuid.New()
	token := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'User','user@example.test','unused',$2)`,
		user,
		role,
	)
	f.exec(t, `INSERT INTO restaurant_settings(restaurant_name) VALUES('Test')`)
	f.exec(
		t,
		`INSERT INTO sessions(user_id,token_hash,csrf_token,expires_at) VALUES($1,$2,'unused',clock_timestamp()+interval '1 hour')`,
		user,
		authservice.SessionTokenHash(token),
	)
	service := eventservice.NewService(
		eventrepo.NewRepository(),
		repository.NewRepository(f.pool, zap.NewNop()),
		zap.NewNop(),
	)
	lifecycle, cancel := context.WithCancel(f.ctx)
	server := f.serve(t, service, lifecycle)
	t.Cleanup(cancel)
	return server, service, token, cancel
}

func (f *fixture) serve(t *testing.T, service *eventservice.Service, lifecycle context.Context) *httptest.Server {
	t.Helper()
	auth, err := authservice.NewService(authrepo.NewRepository(f.pool), "https://flagman.example")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(service, auth, lifecycle)
	h.poll = 10 * time.Millisecond
	r := gin.New()
	access := authhandler.NewHandler(auth)
	r.Use(access.CORS())
	api := r.Group("/api/v1", access.RequireRouteAccess)
	basehandler.RegisterEventRoutes(api, h)
	server := httptest.NewUnstartedServer(r)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func open(t *testing.T, server *httptest.Server, token, cursor string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL+"/api/v1/events",
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: authservice.SessionCookieName, Value: token})
	}
	if cursor != "" {
		req.Header.Set("Last-Event-ID", cursor)
	}
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

type wireEvent struct {
	ID      int64
	Type    string
	Payload model.OutboxPayload
}

func next(t *testing.T, r *bufio.Reader) wireEvent {
	t.Helper()
	e := wireEvent{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal("read stream", err)
		}
		if line == "\n" && e.Type != "" {
			return e
		}
		if strings.HasPrefix(line, "id: ") {
			e.ID, err = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id: ")), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(line, "event: ") {
			e.Type = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e.Payload); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func (f *fixture) append(t *testing.T, s *eventservice.Service, typ string, payload *model.OutboxPayload) int64 {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	f.exec(
		t,
		`INSERT INTO outbox_events(id,event_type,payload,created_at) VALUES($1,$2,$3,clock_timestamp())`,
		id,
		typ,
		data,
	)
	if err := s.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	var cursor int64
	if err := f.pool.QueryRow(f.ctx, `SELECT cursor FROM outbox_events WHERE id=$1`, id).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	return cursor
}

func TestLiveSSEOrderedFramesAllRolesAndCancellation(t *testing.T) {
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		t.Run(role, func(t *testing.T) {
			f := newFixture(t)
			server, s, token, cancel := f.stream(t, role)
			if resp := open(t, server, "", ""); resp.StatusCode != http.StatusUnauthorized {
				t.Fatal("public stream", resp.StatusCode)
			}
			resp := open(t, server, token, "")
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" ||
				resp.Header.Get("Cache-Control") != "no-cache" ||
				resp.Header.Get("X-Accel-Buffering") != "no" {
				t.Fatal(resp.StatusCode, resp.Header)
			}
			r := bufio.NewReader(resp.Body)
			id := uuid.New()
			payload := model.OutboxPayload{
				OccurredAt: time.Now().UTC(),
				RequestID:  "private",
				Topics:     []model.EventTopic{model.TopicUsers},
				UserIDs:    []uuid.UUID{id},
			}
			hidden := f.append(t, s, "users.changed", &payload)
			payload.Topics = []model.EventTopic{
				model.TopicInventory,
				model.TopicDishes,
				model.TopicOrders,
				model.TopicSupplies,
				model.TopicUsers,
			}
			payload.ProductIDs = []uuid.UUID{id}
			payload.DishIDs = []uuid.UUID{id}
			payload.OrderIDs = []uuid.UUID{id}
			payload.SupplyIDs = []uuid.UUID{id}
			visible := f.append(t, s, "inventory.changed", &payload)
			if role == "admin" {
				got := next(t, r)
				if got.ID != hidden {
					t.Fatal("unordered admin stream", got)
				}
			}
			got := next(t, r)
			if got.ID != visible || got.Type != "inventory.changed" || got.Payload.RequestID != "" {
				t.Fatal(got)
			}
			if role == "storekeeper" &&
				(len(got.Payload.DishIDs) != 0 || len(got.Payload.OrderIDs) != 0 || len(got.Payload.UserIDs) != 0) {
				t.Fatal("menu or user IDs leaked", got)
			}
			if role == "order_operator" && (len(got.Payload.SupplyIDs) != 0 || len(got.Payload.UserIDs) != 0) {
				t.Fatal("warehouse or user IDs leaked", got)
			}
			cancel()
			if _, err := io.ReadAll(resp.Body); err != nil {
				t.Fatal("lifecycle closure", err)
			}
		})
	}
}
