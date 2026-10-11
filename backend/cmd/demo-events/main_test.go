package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

func TestCLIRequiresExplicitDemoConfigurationBeforeNetwork(t *testing.T) {
	contacted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true }))
	defer server.Close()
	base := []string{"--confirm-demo", "--demo-url", server.URL, "--origin", "http://frontend.test"}
	for _, tc := range []struct {
		env  string
		args []string
	}{
		{"demo", nil},
		{"production", base},
		{"demo", []string{"--confirm-demo", "--origin", "http://frontend.test"}},
		{"demo", append(append([]string{}, base...), "--count", "0")},
		{"demo", append(append([]string{}, base...), "--interval", "-1s")},
		{"demo", append(append([]string{}, base...), "--dish-id", "../users")},
		{"demo", append(append([]string{}, base...), "unexpected")},
	} {
		var out, errOut bytes.Buffer
		code := run(context.Background(), tc.args, func(key string) string {
			if key == "ENV" {
				return tc.env
			}
			return "private-password"
		}, &out, &errOut)
		if code != 2 || contacted {
			t.Fatalf("unsafe CLI: code=%d contacted=%v", code, contacted)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(
		context.Background(),
		[]string{"--help"},
		func(string) string { return "" },
		&out,
		&errOut,
	); code != 0 {
		t.Fatalf("help: %d", code)
	}
}

func TestCLIConfiguresBoundedRunAndCancellation(t *testing.T) {
	events := 0
	logout := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "private-cookie", Path: "/"})
			_ = json.NewEncoder(w).
				Encode(dto.AuthResponse{CSRFToken: "csrf", User: dto.User{Email: "manager@flagman.invalid", Role: "manager", IsActive: true}})
		case "/api/v1/auth/logout":
			logout = true
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/orders":
			events++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
		default:
			_ = json.NewEncoder(w).
				Encode(dto.Dish{ID: uuid.MustParse("30000000-0000-4000-8000-000000000001"), IsActive: true, RecipeVersion: new(1), Availability: dto.Availability{AvailablePortions: "1"}})
		}
	}))
	defer server.Close()
	args := []string{
		"--confirm-demo",
		"--demo-url",
		server.URL,
		"--origin",
		"http://frontend.test",
		"--dish-id",
		"30000000-0000-4000-8000-000000000001",
		"--scenario",
		"depletion",
		"--count",
		"2",
		"--interval",
		"0",
	}
	getenv := func(key string) string {
		if key == "ENV" {
			return "demo"
		}
		return "test-password"
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), args, getenv, &out, &errOut); code != 0 || events != 2 || !logout {
		t.Fatalf("CLI run: code=%d events=%d logout=%v error=%s", code, events, logout, errOut.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx, args, getenv, &out, &errOut); code != 130 || events != 2 {
		t.Fatalf("canceled CLI: %d", code)
	}
}

func TestExecutableSIGTERMStopsIntervalAndLogsOut(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "demo-events")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
	loggedOut := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "private-cookie", Path: "/"})
			_ = json.NewEncoder(w).
				Encode(dto.AuthResponse{CSRFToken: "csrf", User: dto.User{Email: "manager@flagman.invalid", Role: "manager", IsActive: true}})
		case "/api/v1/auth/logout":
			loggedOut <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/orders":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
		default:
			_ = json.NewEncoder(w).
				Encode(dto.Dish{ID: uuid.MustParse("30000000-0000-4000-8000-000000000001"), IsActive: true, RecipeVersion: new(1), Availability: dto.Availability{AvailablePortions: "1"}})
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		binary,
		"--confirm-demo",
		"--demo-url",
		server.URL,
		"--origin",
		"http://frontend.test",
		"--dish-id",
		"30000000-0000-4000-8000-000000000001",
		"--scenario",
		"depletion",
		"--count",
		"100",
		"--interval",
		"1h",
	)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "ENV=") && !strings.HasPrefix(entry, "DEMO_MANAGER_PASSWORD=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "ENV=demo", "DEMO_MANAGER_PASSWORD=test-password")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	found := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "order created id=") {
			found = true
			break
		}
	}
	if !found {
		_ = cmd.Wait()
		t.Fatalf("CLI emitted no order: %s", stderr.String())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 130 || !strings.Contains(stderr.String(), "demo-events stopped") {
		t.Fatalf("signal handling: %v %s", err, stderr.String())
	}
	select {
	case <-loggedOut:
	case <-time.After(time.Second):
		t.Fatal("SIGTERM did not revoke session")
	}
}

func TestTaskDemoEventsEntryPoint(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task runner not installed")
	}
	events := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var input dto.LoginRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Password != "test-password" {
				t.Error("Task did not inherit demo password")
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "private-cookie", Path: "/"})
			_ = json.NewEncoder(w).
				Encode(dto.AuthResponse{CSRFToken: "csrf", User: dto.User{Email: "manager@flagman.invalid", Role: "manager", IsActive: true}})
		case "/api/v1/auth/logout":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/orders":
			events++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dto.OrderDetail{ID: uuid.New()})
		default:
			_ = json.NewEncoder(w).
				Encode(dto.Dish{ID: uuid.MustParse("30000000-0000-4000-8000-000000000001"), IsActive: true, RecipeVersion: new(1), Availability: dto.Availability{AvailablePortions: "1"}})
		}
	}))
	defer server.Close()
	taskfile, err := filepath.Abs("../../../Taskfile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		task,
		"--taskfile",
		taskfile,
		"demo-events",
		"CONFIRM=demo",
		"DEMO_URL="+server.URL,
		"ORIGIN=http://frontend.test",
		"SCENARIO=depletion",
		"COUNT=2",
		"INTERVAL=0",
		"DISH_ID=30000000-0000-4000-8000-000000000001",
	)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "ENV=") && !strings.HasPrefix(entry, "DEMO_MANAGER_PASSWORD=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "ENV=demo", "DEMO_MANAGER_PASSWORD=test-password")
	output, err := cmd.CombinedOutput()
	if err != nil || events != 2 || strings.Count(string(output), "order created id=") != 2 ||
		strings.Contains(string(output), "test-password") {
		t.Fatalf("Task entry point: %v events=%d output=%s", err, events, output)
	}
}
