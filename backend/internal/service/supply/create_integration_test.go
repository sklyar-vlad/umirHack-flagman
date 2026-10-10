package supply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
	"go.uber.org/zap"
)

type supplyFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	service *Service
	repo    *supplyrepository.Repository
	actor   uuid.UUID
	low     uuid.UUID
	high    uuid.UUID
	schema  string
	input   dto.CreateSupplyRequest
}

// Each test migrates and drops only its own randomly named schema.
func newSupplyFixture(t *testing.T) *supplyFixture {
	t.Helper()
	dsn := os.Getenv("SUPPLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPLY_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(); cancel() })
	schema := "supply_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); pool.Close() })
	files, err := filepath.Glob("../../../database/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, file := range files {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(script), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("migrate %s: %v", file, err)
		}
	}
	f := &supplyFixture{
		ctx: requestctx.WithID(ctx, "supply-integration"), pool: pool, schema: schema,
		repo: supplyrepository.NewRepository(), actor: uuid.New(),
		low:  uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		high: uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role)
		VALUES ($1, 'Приёмщик', 'supply-test@example.test', 'fixture-hash', 'storekeeper')
	`, f.actor.String())
	if err != nil {
		t.Fatal(err)
	}
	var category uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO product_categories (name) VALUES ('Тест') RETURNING id`).
		Scan(&category); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO products (id, category_id, name, unit, display_unit, standard_shelf_life_hours)
		VALUES ($1, $3, 'Моцарелла', 'g', 'kg', 72), ($2, $3, 'Сливки', 'ml', 'l', NULL)
	`, f.low.String(), f.high.String(), category.String())
	if err != nil {
		t.Fatal(err)
	}
	f.service = NewService(
		f.repo,
		repository.NewRepository(pool, zap.NewNop()),
		idempotency.NewService(repository.NewRepository(pool, zap.NewNop())),
	)
	now := time.Now().UTC().Truncate(time.Microsecond)
	f.service.now = func() time.Time { return now }
	expires := now.Add(24 * time.Hour).Format(time.RFC3339Nano)
	produced := now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
	f.input = dto.CreateSupplyRequest{
		Number: "  TEST-104  ", ReceivedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
		Items: []dto.CreateSupplyItem{
			{ProductID: f.high.String(), Quantity: "1.250", ExpiresAt: &expires},
			{ProductID: f.low.String(), Quantity: "200.125", ProducedAt: &produced},
			{ProductID: f.low.String(), Quantity: "2", ExpiresAt: &expires},
		},
	}
	return f
}

func TestCreateSupplyIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	result, err := f.service.Create(f.ctx, f.actor, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == uuid.Nil || result.Number != "TEST-104" || len(result.Batches) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	var actor uuid.UUID
	var number string
	var received, created time.Time
	err = f.pool.QueryRow(f.ctx, `
		SELECT number, received_at, created_at, created_by FROM supplies WHERE id = $1
	`, result.ID.String()).Scan(&number, &received, &created, &actor)
	if err != nil || number != result.Number || actor != f.actor || !received.Equal(result.ReceivedAt) ||
		!created.Equal(result.CreatedAt) {
		t.Fatalf("supply metadata mismatch: %v", err)
	}
	for i, batch := range result.Batches {
		var productID, movementActor uuid.UUID
		var line int
		var quantity, remaining, delta, after, name, unit, source string
		var produced *time.Time
		var hours *int
		var expiry, occurred time.Time
		err := f.pool.QueryRow(f.ctx, `
			SELECT b.product_id, b.line_number, b.initial_quantity::text, b.remaining_quantity::text,
				b.produced_at, b.shelf_life_hours, b.expiry_source::text, b.expires_at,
				m.delta_quantity::text, m.remaining_quantity_after::text,
				m.product_name_snapshot, m.unit_snapshot::text, m.actor_id, m.occurred_at
			FROM batches b JOIN stock_movements m ON m.batch_id = b.id AND m.type = 'receipt'
			WHERE b.id = $1
		`, batch.ID.String()).Scan(&productID, &line, &quantity, &remaining,
			&produced, &hours, &source, &expiry, &delta, &after, &name, &unit, &movementActor, &occurred)
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("%d.%03d", batch.QuantityMilli/1000, batch.QuantityMilli%1000)
		if productID != batch.ProductID || line != i+1 || movementActor != f.actor {
			t.Fatalf("batch/receipt metadata mismatch for line %d", i+1)
		}
		if quantity != want || remaining != want || delta != want || after != want {
			t.Fatalf("batch/receipt quantity mismatch for line %d", i+1)
		}
		if !occurred.Equal(received) || !expiry.Equal(batch.Expiry.ExpiresAt) || source != string(batch.Expiry.Source) {
			t.Fatalf("batch/receipt dates mismatch for line %d", i+1)
		}
		if i == 0 && (name != "Сливки" || unit != "ml" || produced != nil || hours != nil) {
			t.Fatal("explicit expiry or receipt snapshot mismatch")
		}
		if i == 1 &&
			(name != "Моцарелла" || unit != "g" || produced == nil || hours == nil || *hours != 72 || !produced.Equal(*batch.Expiry.ProducedAt)) {
			t.Fatal("calculated expiry or receipt snapshot mismatch")
		}
	}
	var auditActor, entityID uuid.UUID
	var action, entityType, requestID string
	var beforeNull bool
	var afterJSON []byte
	err = f.pool.QueryRow(f.ctx, `
		SELECT actor_id, action, entity_type, entity_id, request_id, before_data IS NULL, after_data
		FROM audit_log
	`).Scan(&auditActor, &action, &entityType, &entityID, &requestID, &beforeNull, &afterJSON)
	if err != nil || auditActor != f.actor || entityID != result.ID || action != "create" || entityType != "supply" ||
		requestID != "supply-integration" ||
		!beforeNull {
		t.Fatalf("audit metadata mismatch: %v", err)
	}
	var snapshot model.SupplyAuditSnapshot
	if err := json.Unmarshal(afterJSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Number != result.Number || !snapshot.ReceivedAt.Equal(received) || len(snapshot.Batches) != 3 {
		t.Fatalf("audit snapshot mismatch: %+v", snapshot)
	}
	for i, batch := range result.Batches {
		if snapshot.Batches[i].BatchID != batch.ID || snapshot.Batches[i].ProductID != batch.ProductID ||
			snapshot.Batches[i].LineNumber != i+1 || !snapshot.Batches[i].ExpiresAt.Equal(batch.Expiry.ExpiresAt) {
			t.Fatalf("audit batch mismatch on line %d", i+1)
		}
	}
	var eventType string
	var eventActor uuid.UUID
	var cursor *int64
	var published *time.Time
	var payloadJSON []byte
	err = f.pool.QueryRow(f.ctx, `SELECT event_type, actor_id, cursor, published_at, payload FROM outbox_events`).
		Scan(&eventType, &eventActor, &cursor, &published, &payloadJSON)
	if err != nil || eventType != "inventory.changed" || eventActor != f.actor || cursor != nil || published != nil {
		t.Fatalf("outbox metadata mismatch: %v", err)
	}
	var payload model.OutboxPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	wantTopics := []model.EventTopic{
		model.TopicInventory, model.TopicBatches, model.TopicDashboard, model.TopicDishes,
		model.TopicStopList, model.TopicHotOffers, model.TopicProcurementRecommendations,
		model.TopicStockMovements, model.TopicSupplies,
	}
	if payload.RequestID != requestID || payload.OccurredAt.IsZero() || !slices.Equal(payload.Topics, wantTopics) ||
		!slices.Equal(
			payload.ProductIDs,
			[]uuid.UUID{f.low, f.high},
		) || !slices.Equal(payload.SupplyIDs, []uuid.UUID{result.ID}) {
		t.Fatalf("outbox payload mismatch: %+v", payload)
	}
	for i, batch := range result.Batches {
		if len(payload.BatchIDs) != 3 || payload.BatchIDs[i] != batch.ID {
			t.Fatal("outbox must include every created batch")
		}
	}
	assertSupplyCounts(t, f, 1, 3)
	// Journal facts remain stable after a product rename/unit edit.
	if _, err := f.pool.Exec(
		f.ctx,
		`UPDATE products SET name = 'Переименовано', unit = 'ml', display_unit = 'ml' WHERE id = $1`,
		f.low.String(),
	); err != nil {
		t.Fatal(err)
	}
	var snapshots int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM stock_movements WHERE product_name_snapshot = 'Моцарелла' AND unit_snapshot = 'g'`).
		Scan(&snapshots); err != nil ||
		snapshots != 2 {
		t.Fatalf("movement snapshots changed: %v", err)
	}
}

func assertSupplyCounts(t *testing.T, f *supplyFixture, supplies, batches int) {
	t.Helper()
	for _, check := range []struct {
		table string
		want  int
	}{
		{"supplies", supplies},
		{"batches", batches},
		{"stock_movements", batches},
		{"audit_log", supplies},
		{"outbox_events", supplies},
	} {
		var count int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+pgx.Identifier{check.table}.Sanitize()).
			Scan(&count); err != nil ||
			count != check.want {
			t.Fatalf("%s: got %d, want %d, error %v", check.table, count, check.want, err)
		}
	}
	var mismatches int
	err := f.pool.QueryRow(f.ctx, `
		SELECT count(*) FROM batches b
		WHERE b.remaining_quantity <> COALESCE((SELECT sum(delta_quantity) FROM stock_movements WHERE batch_id = b.id), 0)
	`).Scan(&mismatches)
	if err != nil || mismatches != 0 {
		t.Fatalf("journal disagrees with stock: %v", err)
	}
}

func TestCreateSupplyRollbackIntegration(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{"supply", "ALTER TABLE supplies ADD CHECK (false)"},
		{"second batch", "ALTER TABLE batches ADD CHECK (line_number <> 2)"},
		{"second receipt", "ALTER TABLE stock_movements ADD CHECK (product_name_snapshot <> 'Моцарелла')"},
		{"audit", "ALTER TABLE audit_log ADD CHECK (false)"},
		{"outbox", "ALTER TABLE outbox_events ADD CHECK (false)"},
		{"commit", `
			CREATE FUNCTION reject_supply_commit() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'forced supply commit failure'; END;
			$$;
			CREATE CONSTRAINT TRIGGER reject_supply_commit AFTER INSERT ON supplies
			DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_supply_commit();
		`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSupplyFixture(t)
			if _, err := f.pool.Exec(f.ctx, test.sql); err != nil {
				t.Fatal(err)
			}
			result, err := f.service.Create(f.ctx, f.actor, f.input)
			if err == nil || result.ID != uuid.Nil || len(result.Batches) != 0 {
				t.Fatalf("failed write returned a result: %+v, %v", result, err)
			}
			assertSupplyCounts(t, f, 0, 0)
		})
	}
}

func TestCreateSupplyRejectsInvalidLineIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	expires := f.service.now().Add(-time.Minute).Format(time.RFC3339Nano)
	f.input.Items[2].ExpiresAt = &expires
	_, err := f.service.Create(f.ctx, f.actor, f.input)
	var fieldErr *validations.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "items[2].expires_at" {
		t.Fatalf("expected expiry validation failure, got %v", err)
	}
	assertSupplyCounts(t, f, 0, 0)
}

type numberBarrierRepository struct {
	Repository
	ready   chan struct{}
	release chan struct{}
}

func (r *numberBarrierRepository) NumberExists(ctx context.Context, tx repository.DBTX, number string) (bool, error) {
	exists, err := r.Repository.NumberExists(ctx, tx, number)
	if err != nil {
		return false, err
	}
	r.ready <- struct{}{}
	select {
	case <-r.release:
		return exists, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func TestCreateSupplyConcurrentNumberIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	barrier := &numberBarrierRepository{Repository: f.repo, ready: make(chan struct{}, 2), release: make(chan struct{})}
	s := NewService(barrier, repository.NewRepository(f.pool, zap.NewNop()), nil)
	s.now = f.service.now
	results := make(chan error, 2)
	// Disjoint products permit both prechecks to see the number as available.
	for _, item := range []dto.CreateSupplyItem{f.input.Items[0], f.input.Items[1]} {
		input := f.input
		input.Items = []dto.CreateSupplyItem{item}
		go func() { _, err := s.Create(f.ctx, f.actor, input); results <- err }()
	}
	for range 2 {
		select {
		case <-barrier.ready:
		case <-f.ctx.Done():
			t.Fatal("number prechecks did not run concurrently")
		}
	}
	close(barrier.release)
	var successes, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, apperrors.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes %d, conflicts %d", successes, conflicts)
	}
	assertSupplyCounts(t, f, 1, 1)
}

func waitForProductLock(t *testing.T, f *supplyFixture) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := f.pool.QueryRow(f.ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE application_name = $1 AND wait_event_type = 'Lock' AND query LIKE '%FROM products%')
		`, f.schema).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatal("product query never waited for a row lock")
		}
	}
}

func TestSupplyProductLockOrderIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(f.ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE`, f.high.String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		shared := repository.NewRepository(f.pool, zap.NewNop())
		result <- shared.WithinTx(ctx, nil, func(tx repository.DBTX) error {
			_, err := f.repo.FindProducts(ctx, tx, []uuid.UUID{f.high, f.low})
			return err
		})
	}()
	waitForProductLock(t, f)
	probe, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = probe.Exec(f.ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE NOWAIT`, f.low.String())
	probe.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("lower ID must already be locked before waiting for higher ID: %v", err)
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestCreateSupplyChecksExpiryAfterLockIntegration(t *testing.T) {
	f := newSupplyFixture(t)
	f.input.Items = f.input.Items[:1]
	holder, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(f.ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE`, f.high.String()); err != nil {
		t.Fatal(err)
	}
	// Inject a later server T without making the test wait for a real expiry.
	expires, err := time.Parse(time.RFC3339Nano, *f.input.Items[0].ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	checked := make(chan struct{}, 1)
	f.service.now = func() time.Time { checked <- struct{}{}; return expires.Add(time.Minute) }
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := f.service.Create(ctx, f.actor, f.input); result <- err }()
	waitForProductLock(t, f)
	select {
	case <-checked:
		t.Fatal("server time was fixed before obtaining product locks")
	default:
	}
	if err := holder.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	var fieldErr *validations.FieldError
	if err := <-result; !errors.As(err, &fieldErr) || fieldErr.Field != "items[0].expires_at" {
		t.Fatalf("expected expiry rejection after lock wait: %v", err)
	}
	assertSupplyCounts(t, f, 0, 0)
}
