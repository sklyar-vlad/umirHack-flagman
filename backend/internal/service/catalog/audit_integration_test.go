package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	cataloghandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/catalog"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCatalogSafeAtomicAuditMetadataAndSnapshots(t *testing.T) {
	f := newFixture(t)
	ctx := requestctx.WithID(f.actor(), "catalog-audit")
	c, err := f.catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "Before category"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.catalog.UpdateCategory(ctx, c.ID, dto.UpdateCategoryRequest{Name: "After category", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.catalog.CreateProduct(ctx, dto.CreateProductRequest{Name: "Before product", CategoryID: c.ID, Unit: "g", DisplayUnit: "kg", TargetStock: ptr("10.001"), StandardShelfLifeHours: ptr(24)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.catalog.UpdateProduct(ctx, p.ID, dto.UpdateProductRequest{Name: ptr("After product"), TargetStock: dto.Nullable[string]{Set: true}, ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "after CATEGORY"})
	f.catalog.UpdateProduct(ctx, p.ID, dto.UpdateProductRequest{Name: ptr("Stale"), ExpectedRevision: 1})
	f.catalog.UpdateProduct(ctx, p.ID, dto.UpdateProductRequest{TargetStock: dto.Nullable[string]{Set: true, Value: ptr("-1")}, ExpectedRevision: 2})
	rows, err := f.pool.Query(ctx, `SELECT actor_id,action,entity_type,entity_id,request_id,before_data,after_data FROM audit_log ORDER BY created_at,id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var actor, entity uuid.UUID
		var action, typ, rid string
		var before, after []byte
		if err := rows.Scan(&actor, &action, &typ, &entity, &rid, &before, &after); err != nil {
			t.Fatal(err)
		}
		expectedID, expectedType, fields := c.ID, "product_category", 2
		if i >= 2 {
			expectedID, expectedType, fields = p.ID, "product", 8
		}
		expectedAction := "create"
		if i%2 == 1 {
			expectedAction = "update"
		}
		if i > 3 || actor != f.user || entity != expectedID || typ != expectedType || action != expectedAction || rid != "catalog-audit" {
			t.Fatal("metadata", i, action, typ, rid)
		}
		var next, old map[string]any
		if err := json.Unmarshal(after, &next); err != nil || len(next) != fields || next["revision"] != float64(i%2+1) {
			t.Fatal(string(after), err)
		}
		if i%2 == 0 {
			if before != nil {
				t.Fatal("create before")
			}
		} else {
			if err := json.Unmarshal(before, &old); err != nil || old["revision"] != float64(1) {
				t.Fatal("old snapshot", err)
			}
			if next["name"] == old["name"] {
				t.Fatal("before overwritten")
			}
		}
		if i == 3 && (next["target_stock"] != nil || old["target_stock"] != "10.001" || old["standard_shelf_life_hours"] != float64(24)) {
			t.Fatal("nullable snapshot", next, old)
		}
		i++
	}
	if rows.Err() != nil || i != 4 {
		t.Fatal("audit count", i, rows.Err())
	}
}

func TestCatalogAuditFailureRollsBackAndHidesDatabaseDetails(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, op := range []string{"create category", "update category", "create product", "update product"} {
			t.Run(op+map[bool]string{false: " immediate", true: " deferred"}[deferred], func(t *testing.T) {
				f := newFixture(t)
				p := f.product(t)
				session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
				if err != nil {
					t.Fatal(err)
				}
				f.exec(t, `CREATE FUNCTION reject_catalog_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'catalog private database detail'; END $$`)
				if deferred {
					f.exec(t, `CREATE CONSTRAINT TRIGGER reject_audit AFTER INSERT ON audit_log DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_catalog_audit()`)
				} else {
					f.exec(t, `CREATE TRIGGER reject_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_catalog_audit()`)
				}
				core, logs := observer.New(zap.DebugLevel)
				r := gin.New()
				r.Use(basehandler.RequestID(), basehandler.RequestLogger(zap.New(core)))
				access := authhandler.NewHandler(f.service)
				api := r.Group("/api/v1", access.RequireRouteAccess)
				basehandler.RegisterCatalogRoutes(api, cataloghandler.NewHandler(f.catalog))
				method, path, body := "POST", "/product-categories", `{"name":"Failed"}`
				switch op {
				case "update category":
					method = "PATCH"
					path += "/" + p.Category.ID.String()
					body = `{"name":"Failed","expected_revision":1}`
				case "create product":
					path = "/products"
					body = `{"name":"Failed","category_id":"` + p.Category.ID.String() + `","unit":"g","display_unit":"g"}`
				case "update product":
					method = "PATCH"
					path = "/products/" + p.ID.String()
					body = `{"name":"Failed","target_stock":"123","is_active":false,"expected_revision":1}`
				}
				w := request(r, method, path, body, token, session.CSRFToken, "https://flagman.example")
				if w.Code != 500 || !stringsContains(w.Body.String(), "INTERNAL_ERROR") {
					t.Fatal(w.Code, w.Body)
				}
				got, err := f.catalog.GetProduct(f.actor(), p.ID)
				if err != nil || got.Name != p.Name || got.Revision != 1 || got.TargetStock != nil || !got.IsActive {
					t.Fatal("product partial write", got, err)
				}
				category, err := f.catalog.GetCategory(f.actor(), p.Category.ID)
				if err != nil || category.Revision != 1 || category.Name != p.Category.Name {
					t.Fatal("category partial write", category, err)
				}
				var products, categories, audits int
				if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM products),(SELECT count(*) FROM product_categories),(SELECT count(*) FROM audit_log)`).Scan(&products, &categories, &audits); err != nil || products != 1 || categories != 1 || audits != 2 {
					t.Fatal("rollback counts", products, categories, audits, err)
				}
				for _, entry := range logs.All() {
					data, _ := json.Marshal(entry.ContextMap())
					for _, secret := range []string{"catalog private database detail", token, session.CSRFToken} {
						if stringsContains(entry.Message+string(data), secret) {
							t.Fatal("private details logged")
						}
					}
				}
			})
		}
	}
}
