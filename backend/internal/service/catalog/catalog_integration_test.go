package catalog_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func ptr[T any](v T) *T { return &v }
func (f *fixture) category(t *testing.T) dto.CatalogCategory {
	t.Helper()
	c, err := f.catalog.CreateCategory(f.actor(), dto.CreateCategoryRequest{Name: "Тест"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) product(t *testing.T) dto.Product {
	t.Helper()
	c := f.category(t)
	p, err := f.catalog.CreateProduct(f.actor(), dto.CreateProductRequest{Name: "Молоко", CategoryID: c.ID, Unit: "ml", DisplayUnit: "l"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCatalogCRUDNormalizedNamesAndLiteralSearch(t *testing.T) {
	f := newFixture(t)
	ctx := f.actor()
	c, err := f.catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "  Мясо_%  "})
	if err != nil || c.Name != "Мясо_%" || c.Revision != 1 {
		t.Fatal(c, err)
	}
	_, err = f.catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "мЯсО_%"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("category name uniqueness", err)
	}
	other, err := f.catalog.CreateCategory(ctx, dto.CreateCategoryRequest{Name: "Другая"})
	if err != nil {
		t.Fatal(err)
	}
	for _, search := range []string{"_%", "мЯсО", "\\"} {
		items, total, err := f.catalog.ListCategories(ctx, model.CategoryFilter{Search: &search, Page: 1, PageSize: 1})
		expected := int64(1)
		if search == "\\" {
			expected = 0
		}
		if err != nil || total != expected || items == nil || len(items) != int(expected) {
			t.Fatal(search, items, total, err)
		}
	}
	items, total, err := f.catalog.ListCategories(ctx, model.CategoryFilter{Page: 99, PageSize: 1})
	if err != nil || total != 2 || items == nil || len(items) != 0 {
		t.Fatal("pagination", err)
	}
	updated, err := f.catalog.UpdateCategory(ctx, c.ID, dto.UpdateCategoryRequest{Name: "Свежая", ExpectedRevision: 1})
	if err != nil || updated.Revision != 2 {
		t.Fatal(updated, err)
	}
	_, err = f.catalog.UpdateCategory(ctx, c.ID, dto.UpdateCategoryRequest{Name: other.Name, ExpectedRevision: 2})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal(err)
	}
	got, err := f.catalog.GetCategory(ctx, c.ID)
	if err != nil || got != updated {
		t.Fatal(got, err)
	}
	p, err := f.catalog.CreateProduct(ctx, dto.CreateProductRequest{Name: "  Говядина  ", CategoryID: c.ID, Unit: "g", DisplayUnit: "kg"})
	if err != nil || p.Name != "Говядина" || p.Category.Name != "Свежая" || p.TargetStock != nil || !p.IsActive || p.Revision != 1 {
		t.Fatal(p, err)
	}
	_, err = f.catalog.CreateProduct(ctx, dto.CreateProductRequest{Name: "говядина", CategoryID: other.ID, Unit: "g", DisplayUnit: "kg"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("global product uniqueness", err)
	}
	p, err = f.catalog.UpdateProduct(ctx, p.ID, dto.UpdateProductRequest{CategoryID: &other.ID, Name: ptr("Свежая говядина"), ExpectedRevision: 1})
	if err != nil || p.Category.ID != other.ID || p.Category.Name != other.Name || p.Revision != 2 {
		t.Fatal(p, err)
	}
	gotP, err := f.catalog.GetProduct(ctx, p.ID)
	if err != nil || gotP.ID != p.ID || gotP.Unit != "g" || gotP.Name != p.Name {
		t.Fatal(gotP, err)
	}
	_, err = f.catalog.CreateProduct(ctx, dto.CreateProductRequest{Name: "Missing", CategoryID: uuid.New(), Unit: "g", DisplayUnit: "g"})
	if err == nil {
		t.Fatal("missing category accepted")
	}
	_, err = f.catalog.GetProduct(ctx, uuid.New())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal(err)
	}
	_, err = f.catalog.GetCategory(ctx, uuid.New())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestCatalogConcurrentNames(t *testing.T) {
	f := newFixture(t)
	c := f.category(t)
	for _, product := range []bool{false, true} {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, name := range []string{" Duplicate ", "dUPLICATE"} {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				var err error
				if product {
					_, err = f.catalog.CreateProduct(f.actor(), dto.CreateProductRequest{Name: name, CategoryID: c.ID, Unit: "g", DisplayUnit: "g"})
				} else {
					_, err = f.catalog.CreateCategory(f.actor(), dto.CreateCategoryRequest{Name: name})
				}
				results <- err
			}(name)
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, apperrors.ErrConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal(success, conflict)
		}
	}
}

func TestCatalogHTTPReadWriteRolePolicyAndShape(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	r := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/product-categories", "/product-categories/" + p.Category.ID.String(), "/products/" + p.ID.String()} {
			if w := request(r, "GET", path, "", token, "", ""); w.Code != 200 {
				t.Fatal(role, path, w.Code, w.Body)
			}
			if w := request(r, "GET", path, "", "", "", ""); w.Code != 401 {
				t.Fatal("public read")
			}
		}
		for _, test := range []struct{ method, path, body string }{{"POST", "/product-categories", `{"name":"HTTP ` + role + `"}`}, {"PATCH", "/product-categories/" + p.Category.ID.String(), `{"name":"Changed ` + role + `","expected_revision":1}`}, {"POST", "/products", `{"name":"HTTP product ` + role + `","category_id":"` + p.Category.ID.String() + `","unit":"g","display_unit":"g"}`}, {"PATCH", "/products/" + p.ID.String(), `{"name":"Changed product","expected_revision":1}`}} {
			if w := request(r, test.method, test.path, test.body, token, "", ""); w.Code != 403 {
				t.Fatal("missing CSRF/origin", w.Code)
			}
			if w := request(r, test.method, test.path, test.body, token, session.CSRFToken, "https://foreign.example"); w.Code != 403 {
				t.Fatal("foreign Origin", w.Code)
			}
			w := request(r, test.method, test.path, test.body, token, session.CSRFToken, "https://flagman.example")
			expected := 403
			if role == "admin" {
				expected = 200
				if test.method == "POST" {
					expected = 201
				}
			}
			if w.Code != expected {
				t.Fatal(role, test.path, w.Code, w.Body)
			}
		}
		if role == "admin" {
			for _, path := range []string{"/product-categories?page_size=101", "/product-categories?search=a&search=b", "/product-categories?sort=id", "/product-categories?unknown=1", "/product-categories/bad", "/products/bad"} {
				if w := request(r, "GET", path, "", token, "", ""); w.Code != 422 {
					t.Fatal(path, w.Code)
				}
			}
			for _, body := range []string{`[]`, `{}`, `{"name":null}`, `{"name":"a","name":"b"}`, `{"name":"a","extra":1}`, `{"name":" "}`, `{"name":"` + strings.Repeat("Я", 201) + `"}`} {
				if w := request(r, "POST", "/product-categories", body, token, session.CSRFToken, "https://flagman.example"); w.Code != 422 {
					t.Fatal(body, w.Code, w.Body)
				}
			}
			w := request(r, "GET", "/product-categories?page_size=1", "", token, "", "")
			var list dto.CategoryListResponse
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 1 || list.Total != 2 {
				t.Fatal(w.Body, err)
			}
		}
	}
}
