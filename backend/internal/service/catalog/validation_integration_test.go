package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestProductValidationExactBoundsAndCompatibility(t *testing.T) {
	f := newFixture(t)
	c := f.category(t)
	valid := dto.CreateProductRequest{Name: "Product", CategoryID: c.ID, Unit: "g", DisplayUnit: "kg"}
	for _, test := range []struct {
		field  string
		change func(*dto.CreateProductRequest)
	}{
		{"unit", func(p *dto.CreateProductRequest) { p.Unit = "kg" }},
		{"display_unit", func(p *dto.CreateProductRequest) { p.Unit = "ml" }},
		{"display_unit", func(p *dto.CreateProductRequest) { p.DisplayUnit = "ml" }},
		{"standard_shelf_life_hours", func(p *dto.CreateProductRequest) { p.StandardShelfLifeHours = ptr(0) }},
		{"standard_shelf_life_hours", func(p *dto.CreateProductRequest) { p.StandardShelfLifeHours = ptr(-1) }},
		{"standard_shelf_life_hours", func(p *dto.CreateProductRequest) { p.StandardShelfLifeHours = ptr(2147483648) }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.TargetStock = ptr("-1") }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.TargetStock = ptr("100000000000") }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.TargetStock = ptr("0.0001") }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.TargetStock = ptr("1e3") }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.TargetStock = ptr(" 1") }},
		{"target_stock", func(p *dto.CreateProductRequest) { p.Unit = "pcs"; p.DisplayUnit = "pcs"; p.TargetStock = ptr("1.001") }},
	} {
		input := valid
		test.change(&input)
		_, err := f.catalog.CreateProduct(f.actor(), input)
		if err == nil || !strings.Contains(err.Error(), test.field) {
			t.Fatal(test.field, err)
		}
	}
	for i, pair := range [][2]string{{"g", "g"}, {"g", "kg"}, {"ml", "ml"}, {"ml", "l"}, {"pcs", "pcs"}} {
		input := valid
		input.Name = pair[0] + pair[1]
		input.Unit = pair[0]
		input.DisplayUnit = pair[1]
		input.StandardShelfLifeHours = ptr(2147483647)
		target := "99999999999.999"
		if pair[0] == "pcs" {
			target = "99999999999.000"
		}
		input.TargetStock = &target
		p, err := f.catalog.CreateProduct(f.actor(), input)
		if err != nil || *p.TargetStock != target || *p.StandardShelfLifeHours != 2147483647 {
			t.Fatal(i, p, err)
		}
	}
	input := valid
	input.TargetStock = ptr("0")
	p, err := f.catalog.CreateProduct(f.actor(), input)
	if err != nil || *p.TargetStock != "0.000" {
		t.Fatal("zero target", p, err)
	}
	// Validation applies to the merged PATCH state, not only supplied fields.
	_, err = f.catalog.UpdateProduct(f.actor(), p.ID, dto.UpdateProductRequest{Unit: ptr("ml"), ExpectedRevision: 1})
	if err == nil {
		t.Fatal("incompatible partial update")
	}
	got, err := f.catalog.GetProduct(f.actor(), p.ID)
	if err != nil || got.Revision != 1 || got.Unit != "g" {
		t.Fatal("failed patch changed product", got, err)
	}
}

func TestProductHTTPNullablePatchAndStrictTypes(t *testing.T) {
	f := newFixture(t)
	p := f.product(t)
	r := f.router()
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	path := "/products/" + p.ID.String()
	for _, body := range []string{
		`{"target_stock":1,"expected_revision":1}`, `{"standard_shelf_life_hours":1.5,"expected_revision":1}`, `{"standard_shelf_life_hours":0,"expected_revision":1}`, `{"target_stock":"0.0001","expected_revision":1}`, `{"display_unit":"g","expected_revision":1}`, `{"name":null,"expected_revision":1}`, `{"unit":null,"expected_revision":1}`, `{"category_id":null,"expected_revision":1}`, `{"is_active":null,"expected_revision":1}`, `{"target_stock":null,"target_stock":"1","expected_revision":1}`, `{"name":"x","NAME":"y","expected_revision":1}`, `{"name":"x","Expected_Revision":1}`, `{"revision":2,"expected_revision":1}`, `{"expected_revision":null}`, `{"expected_revision":1}`, `{"name":"x","expected_revision":1.5}`, `{"name":"x","expected_revision":2147483647}`,
	} {
		if w := request(r, "PATCH", path, body, token, session.CSRFToken, "https://flagman.example"); w.Code != 422 {
			t.Fatal(body, w.Code, w.Body)
		}
	}
	for _, body := range []string{`{"name":`, `{"name":"x"} {}`} {
		if w := request(r, "PATCH", path, body, token, session.CSRFToken, "https://flagman.example"); w.Code != 400 {
			t.Fatal("JSON", w.Code)
		}
	}
	w := request(r, "PATCH", path, `{"target_stock":"0.001","standard_shelf_life_hours":72,"expected_revision":1}`, token, session.CSRFToken, "https://flagman.example")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var result dto.Product
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || *result.TargetStock != "0.001" || *result.StandardShelfLifeHours != 72 {
		t.Fatal(result, err)
	}
	w = request(r, "PATCH", path, `{"display_unit":"ml","expected_revision":2}`, token, session.CSRFToken, "https://flagman.example")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || *result.TargetStock != "0.001" || *result.StandardShelfLifeHours != 72 {
		t.Fatal("absent fields erased", err)
	}
	w = request(r, "PATCH", path, `{"target_stock":null,"standard_shelf_life_hours":null,"expected_revision":3}`, token, session.CSRFToken, "https://flagman.example")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.TargetStock != nil || result.StandardShelfLifeHours != nil || result.DisplayUnit != "ml" {
		t.Fatal("explicit null ignored", result, err)
	}
}
