package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	eventrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
	eventservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/events"
	"go.uber.org/zap"
)

func TestHTTPRestartPublishesCommittedPendingAfterReversedBusinessCommits(t *testing.T) {
	f := newFixture(t)
	server, s, token, cancel := f.stream(t, "admin")
	response := open(t, server, token, "")
	r := bufio.NewReader(response.Body)
	first, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(f.ctx)
	second, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Rollback(f.ctx)
	writer := repository.NewRepository(f.pool, zap.NewNop())
	earlyID, lateID := uuid.New(), uuid.New()
	if err := writer.AppendOutbox(
		f.ctx,
		first,
		&model.OutboxEvent{Type: model.InventoryChanged, ProductIDs: []uuid.UUID{earlyID}},
	); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendOutbox(
		f.ctx,
		second,
		&model.OutboxEvent{Type: model.InventoryChanged, ProductIDs: []uuid.UUID{lateID}},
	); err != nil {
		t.Fatal(err)
	}
	if err := second.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	before := next(t, r)
	if len(before.Payload.ProductIDs) != 1 || before.Payload.ProductIDs[0] != lateID {
		t.Fatal("uncommitted row was published", before)
	}
	if err := first.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}

	cancel()
	response.Body.Close()
	server.Close()
	var pending int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM outbox_events WHERE cursor IS NULL AND published_at IS NULL`).
		Scan(&pending); err != nil ||
		pending != 1 {
		t.Fatal(pending, err)
	}
	fresh := eventservice.NewService(
		eventrepo.NewRepository(),
		repository.NewRepository(f.pool, zap.NewNop()),
		zap.NewNop(),
	)
	ctx, stop := context.WithCancel(f.ctx)
	defer stop()
	restarted := f.serve(t, fresh, ctx)
	response = open(t, restarted, token, strconv.FormatInt(before.ID, 10))
	if err := fresh.Dispatch(f.ctx); err != nil {
		t.Fatal(err)
	}
	after := next(t, bufio.NewReader(response.Body))
	if after.ID <= before.ID || len(after.Payload.ProductIDs) != 1 || after.Payload.ProductIDs[0] != earlyID {
		t.Fatal("late commit missed after restart", after, before)
	}
	response.Body.Close()

	response = open(t, restarted, token, strconv.FormatInt(before.ID, 10))
	duplicate := next(t, bufio.NewReader(response.Body))
	left, _ := json.Marshal(after)
	right, _ := json.Marshal(duplicate)
	if !bytes.Equal(left, right) {
		t.Fatal("reconnect changed durable delivery", after, duplicate)
	}
}
