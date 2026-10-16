package catalog_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	procurementrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/procurement"
	procurementservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/procurement"
	"go.uber.org/zap"
)

func TestCatalogTargetsAndShelfLifeRecalculateOnlyNewFacts(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	p, err := f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{
			TargetStock:            dto.Nullable[string]{Set: true, Value: ptr("10.001")},
			StandardShelfLifeHours: dto.Nullable[int]{Set: true, Value: ptr(24)},
			ExpectedRevision:       1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	produced := now.Add(-time.Hour)
	input := dto.CreateSupplyRequest{
		Number:     "Before",
		ReceivedAt: now.Format(time.RFC3339Nano),
		Items: []dto.CreateSupplyItem{
			{ProductID: p.ID.String(), Quantity: "1.001", ProducedAt: ptr(produced.Format(time.RFC3339Nano))},
		},
	}
	first, err := f.supplies().Create(f.ctx, f.user, input)
	if err != nil {
		t.Fatal(err)
	}
	base := repository.NewRepository(f.pool, zap.NewNop())
	proc := procurementservice.NewService(procurementrepository.NewRepository(), menurepository.NewRepository(), base)
	check := func(target, quantity string) {
		t.Helper()
		list, err := proc.List(f.ctx, model.ProcurementFilter{Page: 1, PageSize: 50})
		if err != nil || list.Total != 1 || list.Items[0].TargetStock != target ||
			list.Items[0].RecommendedQuantity != quantity {
			t.Fatal(list, err)
		}
	}
	check("10.001", "9.000")
	p, err = f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{
			TargetStock:            dto.Nullable[string]{Set: true, Value: ptr("20.002")},
			StandardShelfLifeHours: dto.Nullable[int]{Set: true, Value: ptr(48)},
			Name:                   ptr("Renamed product"),
			ExpectedRevision:       2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	check("20.002", "19.001")
	input.Number = "After"
	second, err := f.supplies().Create(f.ctx, f.user, input)
	if err != nil {
		t.Fatal(err)
	}
	for i, batch := range []model.NewSupplyBatch{first.Batches[0], second.Batches[0]} {
		hours := 24
		if i == 1 {
			hours = 48
		}
		var savedHours int
		var expiry time.Time
		var name, unit, balance string
		if err := f.pool.QueryRow(f.ctx, `SELECT b.shelf_life_hours,b.expires_at,b.remaining_quantity::text,m.product_name_snapshot,m.unit_snapshot::text FROM batches b JOIN stock_movements m ON m.batch_id=b.id WHERE b.id=$1`, batch.ID).
			Scan(&savedHours, &expiry, &balance, &name, &unit); err != nil ||
			savedHours != hours ||
			!expiry.Equal(produced.Add(time.Duration(hours)*time.Hour)) ||
			balance != "1.001" ||
			unit != "ml" {
			t.Fatal("existing/new expiry facts", savedHours, expiry, balance, err)
		}
		if (i == 0 && name != "Молоко") || (i == 1 && name != "Renamed product") {
			t.Fatal("receipt name snapshot rewritten", name)
		}
	}
	input.Number = "Override"
	input.Items[0].ShelfLifeHours = ptr(12)
	override, err := f.supplies().Create(f.ctx, f.user, input)
	if err != nil || *override.Batches[0].Expiry.ShelfLifeHours != 12 {
		t.Fatal("explicit hours ignored", err)
	}
	p, err = f.catalog.UpdateProduct(
		f.actor(),
		p.ID,
		&dto.UpdateProductRequest{
			TargetStock:            dto.Nullable[string]{Set: true},
			StandardShelfLifeHours: dto.Nullable[int]{Set: true},
			ExpectedRevision:       3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	list, err := proc.List(f.ctx, model.ProcurementFilter{Page: 1, PageSize: 50})
	if err != nil || list.Total != 0 {
		t.Fatal("null target did not remove recommendation", err)
	}
	input.Number = "Missing standard"
	input.Items[0].ShelfLifeHours = nil
	_, err = f.supplies().Create(f.ctx, f.user, input)
	if !errors.Is(err, apperrors.ErrShelfLifeRequired) {
		t.Fatal("missing standard accepted", err)
	}
	input.Number = "Explicit expiry"
	input.Items[0].ProducedAt = nil
	input.Items[0].ExpiresAt = ptr(now.Add(2 * time.Hour).Format(time.RFC3339Nano))
	if _, err := f.supplies().Create(f.ctx, f.user, input); err != nil {
		t.Fatal("explicit expiry needs standard", err)
	}
}
