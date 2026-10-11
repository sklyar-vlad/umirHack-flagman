package auditlog_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	catalogrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/catalog"
	dishrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dish"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	supplyrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	userrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/user"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	catalogservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/catalog"
	dishservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dish"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	userservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/user"
	"go.uber.org/zap"
)

func TestAuditListsEveryImplementedBusinessMutationWithoutSecrets(t *testing.T) {
	f := newFixture(t)
	ctx := requestctx.WithID(f.actor(), "business-request")
	base := repository.NewRepository(f.pool, zap.NewNop())
	catalog := catalogservice.NewService(catalogrepo.NewRepository(), base, base)
	dishes := dishservice.NewService(dishrepo.NewRepository(), menurepo.NewRepository(), base, base)
	supplies := supplyservice.NewService(supplyrepo.NewRepository(), base, idempotency.NewService(base))
	orders := orderservice.NewCreateService(orderrepo.NewRepository(), base, idempotency.NewService(base))
	users := userservice.NewService(userrepo.NewRepository(), base, base)
	category, err := catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "Category"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := catalog.CreateProduct(ctx, dto.CreateProductRequest{Name: "Original ingredient", CategoryID: category.ID, Unit: "g", DisplayUnit: "g"})
	if err != nil {
		t.Fatal(err)
	}
	dish, err := dishes.Create(ctx, dto.CreateDishRequest{Name: "Original dish", Recipe: []dto.RecipeInput{{ProductID: product.ID.String(), QuantityPerPortion: "1"}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	supply, err := supplies.Create(ctx, f.user, dto.CreateSupplyRequest{Number: "SUP-1", ReceivedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Items: []dto.CreateSupplyItem{{ProductID: product.ID.String(), Quantity: "10", ExpiresAt: ptr(now.Add(24 * time.Hour).Format(time.RFC3339Nano))}}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(dto.OrderRequest{Items: []dto.OrderRequestItem{{DishID: dish.ID.String(), RecipeVersion: dish.RecipeVersion, Portions: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	order, err := orders.CreateIdempotent(ctx, f.user, key, body)
	if err != nil || order.Status != 201 {
		t.Fatal(order.Status, err)
	}
	replay, err := orders.CreateIdempotent(ctx, f.user, key, body)
	if err != nil || replay.Status != 201 {
		t.Fatal("replay", err)
	}
	created, err := users.Create(ctx, dto.CreateUserRequest{Name: "Other user", Email: "other@example.test", Role: "manager", Password: "private original password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.Update(ctx, created.ID, dto.UpdateUserRequest{Name: ptr("Edited user"), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Password(ctx, created.ID, dto.UserPasswordRequest{Password: "private reset password", ExpectedRevision: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.UpdateCategory(ctx, category.ID, dto.UpdateCategoryRequest{Name: "Edited category", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.UpdateProduct(ctx, product.ID, dto.UpdateProductRequest{Name: ptr("Edited ingredient"), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := dishes.Update(ctx, dish.ID, dto.UpdateDishRequest{Name: ptr("Edited dish"), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.settings.Update(ctx, dto.UpdateSettingsRequest{Timezone: ptr("UTC"), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	// Refused and read-only operations belong outside the successful change log.
	if _, err := catalog.UpdateProduct(ctx, product.ID, dto.UpdateProductRequest{Name: ptr("Stale"), ExpectedRevision: 1}); err == nil {
		t.Fatal("stale accepted")
	}
	if _, err := dishes.Preview(ctx, dto.PreviewDishRequest{Recipe: []dto.RecipeInput{{ProductID: product.ID.String(), QuantityPerPortion: "1"}}}); err != nil {
		t.Fatal(err)
	}
	got, total, err := f.audit.List(ctx, model.AuditLogFilter{Page: 1, PageSize: 100})
	if err != nil || total != 12 || len(got) != 12 {
		t.Fatal("missing or extra mutation audit", total, len(got), err)
	}
	expected := map[string]int{"product_category/create": 1, "product_category/update": 1, "product/create": 1, "product/update": 1, "dish/create": 1, "dish/update": 1, "supply/create": 1, "order/create": 1, "user/create": 1, "user/update": 1, "user/password_change": 1, "settings/update": 1}
	for _, entry := range got {
		expected[entry.EntityType+"/"+entry.Action]--
		if entry.Actor == nil || entry.Actor.ID != f.user || entry.RequestID == nil || *entry.RequestID != "business-request" || len(entry.After) == 0 {
			t.Fatal("audit metadata", entry)
		}
		if entry.Action == "create" && entry.Before != nil || entry.Action != "create" && len(entry.Before) == 0 {
			t.Fatal("before/after contract", entry)
		}
		if entry.EntityType == "settings" {
			if entry.EntityID != nil {
				t.Fatal("singleton entity")
			}
		} else if entry.EntityID == nil {
			t.Fatal("missing entity")
		}
		if entry.EntityType == "order" && !strings.Contains(string(entry.After), "Original dish") {
			t.Fatal("historical audit rewritten")
		}
		if entry.EntityType == "supply" && (entry.EntityID == nil || *entry.EntityID != supply.ID || !strings.Contains(string(entry.After), "10.000")) {
			t.Fatal("supply snapshot")
		}
	}
	for action, count := range expected {
		if count != 0 {
			t.Fatal("audit coverage", action, count)
		}
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{"private original password", "private reset password", f.hash, "password_hash", "session_token", "csrf_token"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret in audit")
		}
	}
}
