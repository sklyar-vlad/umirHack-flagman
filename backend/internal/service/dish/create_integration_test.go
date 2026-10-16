package dish_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
)

func (f *fixture) product(t *testing.T, name, unit string, active bool) uuid.UUID {
	t.Helper()
	c, p := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,$2)`, c, "Category "+p.String())
	f.exec(
		t,
		`INSERT INTO products(id,name,category_id,unit,display_unit,is_active) VALUES($1,$2,$3,$4::text::product_unit,$4::text::display_unit,$5)`,
		p,
		name,
		c,
		unit,
		active,
	)
	return p
}

func recipe(p uuid.UUID, quantity string) []dto.RecipeInput {
	return []dto.RecipeInput{{ProductID: p.String(), QuantityPerPortion: quantity}}
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateDishFirstRecipeAndValidation(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Flour", "g", true)
	pcs := f.product(t, "Egg", "pcs", true)
	inactive := f.product(t, "Inactive", "ml", false)
	d, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: "  Суп  ", Recipe: recipe(p, "0.001")})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Суп" || !d.IsActive || d.Revision != 1 || d.RecipeVersion == nil || *d.RecipeVersion != 1 ||
		d.Recipe[0].QuantityPerPortion != "0.001" ||
		d.Recipe[0].ProductName != "Flour" ||
		d.IsAvailable ||
		len(d.Shortages) != 1 ||
		d.Meta["server_time"] == nil {
		t.Fatalf("dish: %+v", d)
	}
	if f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 || f.count(t, "recipe_items") != 1 {
		t.Fatal("missing creation rows")
	}
	var owner uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT created_by FROM recipe_versions WHERE dish_id=$1`, d.ID).
		Scan(&owner); err != nil ||
		owner != f.user {
		t.Fatal("recipe actor")
	}
	if _, err := f.dishes.Create(
		f.actor(),
		dto.CreateDishRequest{Name: "сУП", Recipe: recipe(p, "1")},
	); !errors.Is(
		err,
		apperrors.ErrConflict,
	) {
		t.Fatalf("normalized name conflict: %v", err)
	}
	for i, in := range []dto.CreateDishRequest{
		{Name: "", Recipe: recipe(p, "1")},
		{Name: strings.Repeat("я", 201), Recipe: recipe(p, "1")},
		{Name: "empty"},
		{Name: "unknown", Recipe: recipe(uuid.New(), "1")},
		{Name: "inactive", Recipe: recipe(inactive, "1")},
		{Name: "zero", Recipe: recipe(p, "0.000")},
		{Name: "negative", Recipe: recipe(p, "-1")},
		{Name: "precision", Recipe: recipe(p, "0.0001")},
		{Name: "range", Recipe: recipe(p, "100000000000")},
		{Name: "pcs", Recipe: recipe(pcs, "0.001")},
		{Name: "description", Description: ptr(strings.Repeat("я", 2001)), Recipe: recipe(p, "1")},
	} {
		if _, err := f.dishes.Create(f.actor(), in); err == nil {
			t.Fatalf("invalid %d accepted", i)
		}
		if f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 || f.count(t, "recipe_items") != 1 {
			t.Fatal("validation wrote data")
		}
	}
	if _, err := f.dishes.Create(
		f.actor(),
		dto.CreateDishRequest{Name: "duplicate", Recipe: append(recipe(p, "1"), recipe(p, "2")...)},
	); !errors.Is(
		err,
		apperrors.ErrDuplicateIngredient,
	) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := f.dishes.Create(
		f.ctx,
		dto.CreateDishRequest{Name: "unauthorized", Recipe: recipe(p, "1")},
	); !errors.Is(
		err,
		apperrors.ErrUnauthorized,
	) {
		t.Fatal("service without actor allowed")
	}
	if _, err := f.dishes.Create(
		f.actor(),
		dto.CreateDishRequest{
			Name:        strings.Repeat("я", 200),
			Description: ptr(strings.Repeat("я", 2000)),
			IsActive:    ptr(false),
			Recipe:      recipe(pcs, "99999999999"),
		},
	); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestCreateDishAtomicRecipeAndConcurrentNames(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	f.exec(
		t,
		`CREATE FUNCTION reject_recipe() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'recipe failure'; END $$`,
	)
	f.exec(
		t,
		`CREATE TRIGGER reject_recipe BEFORE INSERT ON recipe_items FOR EACH ROW EXECUTE FUNCTION reject_recipe()`,
	)
	if _, err := f.dishes.Create(
		f.actor(),
		dto.CreateDishRequest{Name: "Roll back", Recipe: recipe(p, "1")},
	); err == nil {
		t.Fatal("failure not returned")
	}
	if f.count(t, "dishes") != 0 || f.count(t, "recipe_versions") != 0 || f.count(t, "recipe_items") != 0 {
		t.Fatal("partial creation survived")
	}
	f.exec(t, `DROP TRIGGER reject_recipe ON recipe_items`)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"Parallel", "  PARALLEL "} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: name, Recipe: recipe(p, "1")})
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, apperrors.ErrConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 {
		t.Fatal("concurrent name uniqueness")
	}
}

func TestCreateDishHTTPRolesShapeAndCSRF(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "Ingredient", "g", true)
	router := f.router()
	body := fmt.Sprintf(`{"name":"Dish","recipe":[{"product_id":%q,"quantity_per_portion":"1"}]}`, p.String())
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(router, "POST", "/dishes", body, token, session.CSRFToken, "https://flagman.example")
		want := 403
		if role == "admin" {
			want = 201
		}
		if w.Code != want {
			t.Fatalf("role %s: %d %s", role, w.Code, w.Body.String())
		}
	}
	f.exec(t, `UPDATE users SET role='admin' WHERE id=$1`, f.user)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	for _, credentials := range [][2]string{{"", ""}, {"https://flagman.example", ""}, {"https://flagman.example", "wrong"}, {"https://evil.example", session.CSRFToken}} {
		if w := request(router, "POST", "/dishes", body, token, credentials[1], credentials[0]); w.Code != 403 {
			t.Fatalf("csrf: %d", w.Code)
		}
	}
	if w := request(router, "POST", "/dishes", body, "", "", ""); w.Code != 401 {
		t.Fatal("anonymous allowed")
	}
	for _, body := range []string{`[]`, `null`, `{"name":null}`, `{"Name":"X"}`, `{"name":"X","name":"Y"}`, `{"name":"X","unknown":1}`, `{"recipe":null}`, `{"recipe":[null]}`, `{"recipe":[{"Product_id":"x"}]}`, `{"recipe":[{"product_id":null}]}`, `{"recipe":[{"quantity_per_portion":1}]}`, `{"recipe":[{"quantity_per_portion":"1","quantity_per_portion":"2"}]}`} {
		if w := request(
			router,
			"POST",
			"/dishes",
			body,
			token,
			session.CSRFToken,
			"https://flagman.example",
		); w.Code != 422 {
			t.Fatalf("shape %s: %d", body, w.Code)
		}
	}
	if w := request(
		router,
		"POST",
		"/dishes",
		`{`,
		token,
		session.CSRFToken,
		"https://flagman.example",
	); w.Code != 400 {
		t.Fatal("invalid JSON")
	}
	in := dto.CreateDishRequest{Name: "Duplicate", Recipe: append(recipe(p, "1"), recipe(p, "2")...)}
	data, _ := json.Marshal(in)

	var obj map[string]any
	_ = json.Unmarshal(data, &obj)
	delete(obj, "is_active")
	data, _ = json.Marshal(obj)
	w := request(router, "POST", "/dishes", string(data), token, session.CSRFToken, "https://flagman.example")
	if w.Code != 422 || !strings.Contains(w.Body.String(), "DUPLICATE_INGREDIENT") {
		t.Fatalf("duplicate response: %d %s", w.Code, w.Body.String())
	}
}

func TestDishRecipeEntryLimit(t *testing.T) {
	f := newFixture(t)
	c := uuid.New()
	f.exec(t, `INSERT INTO product_categories(id,name) VALUES($1,'Limit category')`, c)
	f.exec(
		t,
		`INSERT INTO products(name,category_id,unit,display_unit) SELECT 'Ingredient '||n,$1,'g','g' FROM generate_series(1,100) n`,
		c,
	)
	rows, err := f.pool.Query(f.ctx, `SELECT id FROM products ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	input := make([]dto.RecipeInput, 0, 100)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		input = append(input, recipe(id, "0.001")...)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	d, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: "100 ingredients", Recipe: input})
	if err != nil || len(d.Recipe) != 100 {
		t.Fatalf("upper recipe limit: %v", err)
	}
	oversize := append(append([]dto.RecipeInput{}, input...), recipe(uuid.New(), "1")...)
	if _, err := f.dishes.Create(f.actor(), dto.CreateDishRequest{Name: "Too many", Recipe: oversize}); err == nil {
		t.Fatal("101 ingredients accepted")
	}
	if _, err := f.dishes.Update(
		f.actor(),
		d.ID,
		dto.UpdateDishRequest{ExpectedRevision: 1, Recipe: &oversize},
	); err == nil {
		t.Fatal("oversize patch accepted")
	}
	if _, err := f.dishes.Preview(f.actor(), dto.PreviewDishRequest{Recipe: oversize}); err == nil {
		t.Fatal("oversize preview accepted")
	}
	if f.count(t, "dishes") != 1 || f.count(t, "recipe_versions") != 1 || f.count(t, "recipe_items") != 100 {
		t.Fatal("oversize recipe wrote")
	}
}
