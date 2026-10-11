package user_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func ptr[T any](v T) *T { return &v }

func createInput() dto.CreateUserRequest {
	return dto.CreateUserRequest{Name: " New user ", Email: " NEW@EXAMPLE.TEST ", Role: "manager", Password: "initial password 123"}
}

func TestUserMutationsRevisionsUniquenessAndPasswords(t *testing.T) {
	f := newFixture(t)
	ctx := f.actor()
	u, err := f.users.Create(ctx, createInput())
	if err != nil || u.Name != "New user" || u.Email != "new@example.test" || u.Revision != 1 || !u.IsActive {
		t.Fatal("create", u, err)
	}
	var hash string
	f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash)
	if hash == createInput().Password || !authservice.Verify(createInput().Password, hash) {
		t.Fatal("invalid password storage")
	}
	if _, err := f.users.Create(ctx, createInput()); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("duplicate email", err)
	}
	next, err := f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Name: ptr("Changed"), ExpectedRevision: 1})
	if err != nil || next.Revision != 2 || next.Email != u.Email || next.Role != u.Role {
		t.Fatal("patch", next, err)
	}
	_, err = f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Email: ptr("manager@example.test"), ExpectedRevision: 2})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("duplicate update", err)
	}
	current, err := f.users.Get(ctx, u.ID)
	if err != nil || current.Revision != 2 || current.Email != u.Email {
		t.Fatal("duplicate rollback")
	}
	_, err = f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Name: ptr("Stale"), ExpectedRevision: 1})
	var conflict *apperrors.RevisionConflict
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 2 {
		t.Fatal("revision conflict", err)
	}
	next, err = f.users.Password(ctx, u.ID, dto.UserPasswordRequest{Password: "replacement password", ExpectedRevision: 2})
	if err != nil || next.Revision != 3 {
		t.Fatal("password reset", err)
	}
	f.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash)
	if !authservice.Verify("replacement password", hash) || authservice.Verify(createInput().Password, hash) {
		t.Fatal("password not replaced")
	}
	_, err = f.users.Password(ctx, u.ID, dto.UserPasswordRequest{Password: "another password", ExpectedRevision: 2})
	if !errors.As(err, &conflict) || conflict.CurrentRevision != 3 {
		t.Fatal("password revision conflict")
	}
	_, err = f.users.Update(ctx, uuid.New(), dto.UpdateUserRequest{Name: ptr("Missing"), ExpectedRevision: 1})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("missing update", err)
	}
	raw, _ := json.Marshal(next)
	if strings.Contains(string(raw), "password") {
		t.Fatal("secret in mutation response")
	}
}

func TestConcurrentUserRevision(t *testing.T) {
	f := newFixture(t)
	ctx := f.actor()
	u, err := f.users.Create(ctx, createInput())
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"First", "Second"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			<-ready
			_, err := f.users.Update(ctx, u.ID, dto.UpdateUserRequest{Name: &name, ExpectedRevision: 1})
			results <- err
		}(name)
	}
	close(ready)
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		var conflict *apperrors.RevisionConflict
		switch {
		case err == nil:
			success++
		case errors.As(err, &conflict) && conflict.CurrentRevision == 2:
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("lost update", success, conflicts)
	}
}

func TestUserValidation(t *testing.T) {
	f := newFixture(t)
	for _, change := range []func(*dto.CreateUserRequest){
		func(r *dto.CreateUserRequest) { r.Name = " " }, func(r *dto.CreateUserRequest) { r.Name = strings.Repeat("я", 201) }, func(r *dto.CreateUserRequest) { r.Name = "bad\x00name" },
		func(r *dto.CreateUserRequest) { r.Email = "invalid" }, func(r *dto.CreateUserRequest) { r.Email = "Name <x@example.test>" }, func(r *dto.CreateUserRequest) { r.Email = strings.Repeat("x", 250) + "@test.example" },
		func(r *dto.CreateUserRequest) { r.Role = "root" }, func(r *dto.CreateUserRequest) { r.Password = "short" }, func(r *dto.CreateUserRequest) { r.Password = strings.Repeat("я", 129) },
	} {
		r := createInput()
		change(&r)
		_, err := f.users.Create(f.actor(), r)
		var field *validations.FieldError
		if !errors.As(err, &field) {
			t.Fatal("validation missing", err)
		}
	}
	for _, n := range []int{12, 128} {
		r := createInput()
		r.Email = uuid.NewString() + "@example.test"
		r.Password = strings.Repeat("я", n)
		if _, err := f.users.Create(f.actor(), r); err != nil {
			t.Fatal("Unicode boundary", n, err)
		}
	}
	for _, input := range []dto.UpdateUserRequest{{Name: ptr("name")}, {ExpectedRevision: 1}, {ExpectedRevision: 1, Role: ptr("root")}, {ExpectedRevision: 1, Email: ptr("bad")}} {
		if _, err := f.users.Update(f.actor(), f.user, input); err == nil {
			t.Fatal("invalid update accepted")
		}
	}
}

func TestUserMutationHTTPContractsAndRolePolicy(t *testing.T) {
	f := newFixture(t)
	r := f.router()
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) int {
		return request(r, method, path, body, token, session.CSRFToken, "https://flagman.example").Code
	}
	body := `{"name":"New","email":"new@example.test","role":"manager","password":"initial password 123"}`
	w := request(r, "POST", "/users", body, token, session.CSRFToken, "https://flagman.example")
	if w.Code != 201 || strings.Contains(w.Body.String(), "password") {
		t.Fatal("create HTTP", w.Code, w.Body)
	}
	var u dto.User
	if json.Unmarshal(w.Body.Bytes(), &u) != nil {
		t.Fatal("user response")
	}
	path := "/users/" + u.ID.String()
	if code := call("PATCH", path, `{"name":"Updated","expected_revision":1}`); code != 200 {
		t.Fatal("patch HTTP", code)
	}
	w = request(r, "PATCH", path, `{"name":"Stale","expected_revision":1}`, token, session.CSRFToken, "https://flagman.example")
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(w.Body.String(), `"current_revision":2`) {
		t.Fatal("conflict HTTP", w.Body)
	}
	if code := call("POST", path+"/password", `{"password":"another password","expected_revision":2}`); code != 200 {
		t.Fatal("reset HTTP", code)
	}
	for _, body := range []string{`null`, `[]`, `{"name":null}`, `{"name":"New","name":"Other"}`, `{"name":123}`, `{"password_hash":"x"}`, `{"name":"New"} {}`} {
		code := call("POST", "/users", body)
		if code != 422 && code != 400 {
			t.Fatal("JSON accepted", body, code)
		}
	}
	if call("POST", "/users", `{`) != 400 {
		t.Fatal("malformed JSON")
	}
	if call("PATCH", path, `{"name":"New","expected_revision":1.5}`) != 422 {
		t.Fatal("noninteger revision")
	}
	if call("POST", "/users", body) != 409 {
		t.Fatal("email uniqueness HTTP")
	}
	for _, route := range []struct{ method, path string }{{"POST", "/users"}, {"PATCH", path}, {"POST", path + "/password"}} {
		for _, origin := range []string{"", "https://evil.example"} {
			if w := request(r, route.method, route.path, body, token, session.CSRFToken, origin); w.Code != 403 {
				t.Fatal("Origin bypass")
			}
		}
		if w := request(r, route.method, route.path, body, token, "", "https://flagman.example"); w.Code != 403 {
			t.Fatal("CSRF bypass")
		}
		for _, role := range []string{"manager", "storekeeper", "order_operator"} {
			f.exec(t, `UPDATE users SET role=$2 WHERE id=$1`, f.user, role)
			if w := request(r, route.method, route.path, body, token, session.CSRFToken, "https://flagman.example"); w.Code != 403 {
				t.Fatal("mutation role bypass", role, w.Code)
			}
		}
		f.exec(t, `UPDATE users SET role='admin' WHERE id=$1`, f.user)
	}
}

func TestUserMutationHTTPFailureBoundaries(t *testing.T) {
	f := newFixture(t)
	session, token, err := f.service.Authenticate(f.ctx, "manager@example.test", f.password)
	if err != nil {
		t.Fatal(err)
	}
	r := f.router()
	for _, field := range []string{`"is_active":false`, `"role":"manager"`} {
		w := request(r, "PATCH", "/users/"+f.user.String(), `{`+field+`,"expected_revision":1}`, token, session.CSRFToken, "https://flagman.example")
		if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"LAST_ADMIN_PROTECTED"`) {
			t.Fatal("last-admin HTTP", w.Code, w.Body)
		}
	}
	for _, input := range []struct{ method, path, body string }{
		{"PATCH", "/users/" + uuid.NewString(), `{"name":"Missing","expected_revision":1}`},
		{"POST", "/users/" + uuid.NewString() + "/password", `{"password":"missing user's password","expected_revision":1}`},
	} {
		if w := request(r, input.method, input.path, input.body, token, session.CSRFToken, "https://flagman.example"); w.Code != 404 {
			t.Fatal("missing user HTTP", w.Code, w.Body)
		}
	}
	w := request(r, "POST", "/users", strings.Repeat("x", (1<<20)+1), token, session.CSRFToken, "https://flagman.example")
	if w.Code != 413 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("large user request", w.Code)
	}
	w = request(r, "POST", "/users", "{\"name\":\""+string([]byte{0xff})+"\"}", token, session.CSRFToken, "https://flagman.example")
	if w.Code != 400 {
		t.Fatal("invalid UTF-8 accepted", w.Code, w.Body)
	}
}
