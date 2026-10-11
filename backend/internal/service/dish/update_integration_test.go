package dish_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
)

func (f *fixture) createDish(t *testing.T, p uuid.UUID) dto.Dish {
	t.Helper()
	d, err := f.dishes.Create(
		f.actor(),
		dto.CreateDishRequest{Name: "Original", Description: ptr("description"), Recipe: recipe(p, "1")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDishPatchImmutableVersionsAndPartialFields(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Flour", "g", true)
	p2 := f.product(t, "Water", "ml", true)
	d := f.createDish(t, p)
	next, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{
			ExpectedRevision: 1,
			Name:             ptr(" Updated "),
			Recipe:           ptr(recipe(p2, "2.25")),
			IsActive:         ptr(false),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Name != "Updated" || next.IsActive || next.Revision != 2 || *next.RecipeVersion != 2 ||
		len(next.Recipe) != 1 ||
		next.Recipe[0].ProductID != p2 ||
		next.Recipe[0].QuantityPerPortion != "2.250" ||
		next.Description == nil ||
		*next.Description != "description" {
		t.Fatalf("update: %+v", next)
	}
	var quantity string
	var id uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT ri.product_id,ri.quantity_per_portion::text FROM recipe_items ri JOIN recipe_versions rv ON rv.id=ri.recipe_version_id WHERE rv.dish_id=$1 AND rv.version=1`, d.ID).
		Scan(&id, &quantity); err != nil ||
		id != p ||
		quantity != "1.000" {
		t.Fatal("old recipe changed")
	}
	next, err = f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{ExpectedRevision: 2, Description: dto.Nullable[string]{Set: true}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Description != nil || next.Revision != 3 || *next.RecipeVersion != 2 || next.Name != "Updated" ||
		next.IsActive ||
		f.count(t, "recipe_versions") != 2 {
		t.Fatal("partial fields or metadata-only version")
	}
	_, err = f.dishes.Update(f.actor(), d.ID, dto.UpdateDishRequest{ExpectedRevision: 2, Name: ptr("stale")})
	var conflict *apperrors.RevisionConflict
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 3 {
		t.Fatalf("revision: %v", err)
	}
	for _, in := range []dto.UpdateDishRequest{{ExpectedRevision: 3}, {ExpectedRevision: 0, Name: ptr("x")}, {ExpectedRevision: 3, Recipe: ptr([]dto.RecipeInput{})}, {ExpectedRevision: 3, Recipe: ptr(recipe(uuid.New(), "1"))}, {ExpectedRevision: 3, Name: ptr("")}} {
		if _, err := f.dishes.Update(f.actor(), d.ID, in); err == nil {
			t.Fatal("invalid patch accepted")
		}
	}
	if _, err := f.dishes.Update(
		f.actor(),
		uuid.New(),
		dto.UpdateDishRequest{ExpectedRevision: 1, Name: ptr("x")},
	); !errors.Is(
		err,
		apperrors.ErrNotFound,
	) {
		t.Fatal("missing dish status")
	}
	if f.count(t, "recipe_versions") != 2 {
		t.Fatal("failed update created a version")
	}
}

func TestDishPatchRecipeFailureRollsBackMetadata(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	d := f.createDish(t, p)
	f.exec(
		t,
		`CREATE FUNCTION reject_new_recipe() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'recipe failure'; END $$`,
	)
	f.exec(
		t,
		`CREATE TRIGGER reject_new_recipe BEFORE INSERT ON recipe_items FOR EACH ROW EXECUTE FUNCTION reject_new_recipe()`,
	)
	if _, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{
			ExpectedRevision: 1,
			Name:             ptr("Changed"),
			Description:      dto.Nullable[string]{Set: true},
			IsActive:         ptr(false),
			Recipe:           ptr(recipe(p, "2")),
		},
	); err == nil {
		t.Fatal("failure accepted")
	}
	var name string
	var revision int
	var active bool
	var description *string
	if err := f.pool.QueryRow(f.ctx, `SELECT name,revision,is_active,description FROM dishes WHERE id=$1`, d.ID).
		Scan(&name, &revision, &active, &description); err != nil {
		t.Fatal(err)
	}
	if name != "Original" || revision != 1 || !active || description == nil || *description != "description" ||
		f.count(t, "recipe_versions") != 1 ||
		f.count(t, "recipe_items") != 1 {
		t.Fatal("partial patch survived")
	}
}

func TestDishActivationChecksSavedRecipeAndCatalogHistory(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	d := f.createDish(t, p)
	if _, err := f.catalog.UpdateProduct(
		f.actor(),
		p,
		&dto.UpdateProductRequest{ExpectedRevision: 1, Unit: ptr("ml"), DisplayUnit: ptr("ml")},
	); !errors.Is(
		err,
		apperrors.ErrUnitLocked,
	) {
		t.Fatalf("recipe failed to lock product unit: %v", err)
	}
	if _, err := f.catalog.UpdateProduct(
		f.actor(),
		p,
		&dto.UpdateProductRequest{ExpectedRevision: 1, IsActive: ptr(false)},
	); err == nil {
		t.Fatal("active dish ingredient disabled")
	}
	if _, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{ExpectedRevision: 1, IsActive: ptr(false)},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.catalog.UpdateProduct(
		f.actor(),
		p,
		&dto.UpdateProductRequest{ExpectedRevision: 1, IsActive: ptr(false)},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{ExpectedRevision: 2, IsActive: ptr(true)},
	); err == nil {
		t.Fatal("inactive ingredient reactivated")
	}
	p2 := f.product(t, "Replacement", "g", true)
	next, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{ExpectedRevision: 2, IsActive: ptr(true), Recipe: ptr(recipe(p2, "1"))},
	)
	if err != nil || !next.IsActive || next.Revision != 3 || *next.RecipeVersion != 2 {
		t.Fatalf("reactivation with replacement: %+v %v", next, err)
	}
}

func TestDishPatchHTTPPartialNullAndRevision(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	d := f.createDish(t, p)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	router := f.router()
	path := "/dishes/" + d.ID.String()
	w := request(
		router,
		"PATCH",
		path,
		`{"expected_revision":1,"description":null}`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 200 {
		t.Fatalf("clear description: %d %s", w.Code, w.Body.String())
	}
	w = request(
		router,
		"PATCH",
		path,
		`{"expected_revision":1,"name":"Stale"}`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"current_revision":2`) {
		t.Fatalf("revision body: %s", w.Body.String())
	}
	for _, body := range []string{`{"expected_revision":2}`, `{"expected_revision":null,"name":"X"}`, `{"expected_revision":2,"name":null}`, `{"expected_revision":2,"recipe":null}`, `{"expected_revision":2,"recipe":[]}`, `{"expected_revision":2,"is_active":null}`, fmt.Sprintf(`{"expected_revision":2,"recipe":[{"product_id":%q,"quantity_per_portion":"0"}]}`, p.String())} {
		if w := request(
			router,
			"PATCH",
			path,
			body,
			token,
			session.CSRFToken,
			"https://flagman.example",
		); w.Code != 422 {
			t.Fatalf("bad patch %s: %d", body, w.Code)
		}
	}
	if w := request(router, "PATCH", path, `{"expected_revision":2,"name":"X"}`, token, "", ""); w.Code != 403 {
		t.Fatal("patch without csrf")
	}
	for _, role := range []string{"manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		if w := request(
			router,
			"PATCH",
			path,
			`{"expected_revision":2,"name":"X"}`,
			token,
			session.CSRFToken,
			"https://flagman.example",
		); w.Code != 403 {
			t.Fatalf("patch by %s", role)
		}
	}
}
