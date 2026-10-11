package writeoff_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestWriteOffValidExpiredAndEmptyBatches(t *testing.T) {
	f := newFixture(t)
	r := f.create(t, "1.125", uuid.New())
	var m dto.StockMovement
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatal(err)
	}
	if r.Status != 201 || m.RemainingQuantityAfter != "8.875" || m.DeltaQuantity != "-1.125" ||
		m.Type != "manual_writeoff" ||
		*m.Note != "Порча продукта" ||
		m.Actor.ID != f.user ||
		m.ProductNameSnapshot != "Молоко" ||
		m.UnitSnapshot != "ml" {
		t.Fatalf("movement: %+v response %+v", m, r)
	}
	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()-interval '1 day' WHERE id=$1`, f.batch)
	if r = f.create(t, "8.875", uuid.New()); r.Status != 201 || f.balance(t) != "0.000" {
		t.Fatalf("expired: %+v", r)
	}
	r = f.create(t, "0.001", uuid.New())
	if r.Status != 409 || !bytes.Contains(r.Body, []byte("INSUFFICIENT_BATCH_STOCK")) || f.balance(t) != "0.000" ||
		f.counts(t) != [4]int{2, 2, 2, 3} {
		t.Fatalf("zero balance: %+v counts %v", r, f.counts(t))
	}
}

func TestWriteOffStrictHTTPAndRoles(t *testing.T) {
	f := newFixture(t)
	router := f.router()
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
		session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
		if err != nil {
			t.Fatal(err)
		}
		w := request(
			router,
			"POST",
			"/write-offs",
			string(f.body("1")),
			token,
			session.CSRFToken,
			"https://flagman.example",
		)
		want := 201
		if role == "order_operator" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("%s: %d %s", role, w.Code, w.Body)
		}
		for _, p := range [][3]string{{"", "", ""}, {"", session.CSRFToken, "https://flagman.example"}, {token, "", "https://flagman.example"}, {token, session.CSRFToken, "https://evil.example"}} {
			w = request(router, "POST", "/write-offs", string(f.body("1")), p[0], p[1], p[2])
			if w.Code != 401 && w.Code != 403 {
				t.Fatalf("auth: %d", w.Code)
			}
		}
	}
	f.exec(t, `UPDATE users SET role='admin' WHERE id=$1`, f.user)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{string(f.body("1"))[:len(f.body("1"))-1] + `,"quantity":"2"}`, "null", "[]", `{}`, `{"batch_id":null}`, strings.ReplaceAll(string(f.body("1")), `"quantity":"1"`, `"quantity":1`), strings.ReplaceAll(string(f.body("1")), `"quantity":"1"`, `"quantity":null`), strings.ReplaceAll(string(f.body("1")), `"reason":"  Порча продукта  "`, `"reason":" "`), strings.ReplaceAll(string(f.body("1")), `"reason":"  Порча продукта  "`, `"reason":"line\nbreak"`), strings.ReplaceAll(string(f.body("1")), `"reason":"  Порча продукта  "`, fmt.Sprintf(`"reason":"%s"`, strings.Repeat("я", 501))), strings.TrimSuffix(string(f.body("1")), "}") + `,"extra":true}`} {
		w := request(router, "POST", "/write-offs", body, token, session.CSRFToken, "https://flagman.example")
		if w.Code != 422 {
			t.Fatalf("invalid %s: %d %s", body, w.Code, w.Body)
		}
	}
	for _, amount := range []string{"0", "-1", "1e2", "1.0001", "100000000000", "", "NaN"} {
		w := request(
			router,
			"POST",
			"/write-offs",
			string(f.body(amount)),
			token,
			session.CSRFToken,
			"https://flagman.example",
		)
		if w.Code != 422 {
			t.Fatalf("%s: %d", amount, w.Code)
		}
	}
	w := request(router, "POST", "/write-offs", "{", token, session.CSRFToken, "https://flagman.example")
	if w.Code != 400 {
		t.Fatalf("JSON: %d", w.Code)
	}
	w = request(
		router,
		"POST",
		"/write-offs?unexpected=1",
		string(f.body("1")),
		token,
		session.CSRFToken,
		"https://flagman.example",
	)
	if w.Code != 422 {
		t.Fatalf("query: %d", w.Code)
	}
	if f.balance(t) != "7.000" || f.counts(t) != [4]int{3, 3, 3, 3} {
		t.Fatalf("invalid changed stock %s %v", f.balance(t), f.counts(t))
	}
}

func TestWriteOffPiecesMissingAndExcess(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `UPDATE products SET unit='pcs',display_unit='pcs' WHERE id=$1`, f.product)
	r := f.create(t, "0.5", uuid.New())
	if r.Status != 422 {
		t.Fatalf("pcs %d", r.Status)
	}
	r = f.create(t, "11", uuid.New())
	if r.Status != 409 || f.balance(t) != "10.000" {
		t.Fatalf("excess %+v", r)
	}
	r = f.create(t, "1.000", uuid.New())
	if r.Status != 201 {
		t.Fatalf("pcs integer %+v", r)
	}
	body := bytes.ReplaceAll(f.body("1"), []byte(f.batch.String()), []byte(uuid.NewString()))
	r, err := f.writeoff.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
	if err != nil || r.Status != 404 {
		t.Fatalf("missing %+v %v", r, err)
	}
}
