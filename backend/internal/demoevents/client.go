// Package demoevents creates demo traffic exclusively through the public API.
package demoevents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
)

const demoEmail = "manager@flagman.invalid"

// Client retains credentials only in memory and never follows redirects.
type Client struct {
	base   string
	origin string
	csrf   string
	http   *http.Client
}

// APIError omits untrusted server messages, bodies and transport error details.
type APIError struct {
	Status  int
	Code    string
	Refusal bool
}

func (e *APIError) Error() string {
	kind := "technical"
	if e.Refusal {
		kind = "refused"
	}
	return fmt.Sprintf("%s status=%d code=%s", kind, e.Status, e.Code)
}

func NewClient(environment, destination, origin string) (*Client, error) {
	if environment != "demo" {
		return nil, errors.New("demo-events requires ENV=demo")
	}
	base, err := url.Parse(destination)
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" ||
		base.User != nil ||
		base.RawQuery != "" ||
		base.Fragment != "" ||
		(base.Path != "" && base.Path != "/") {
		return nil, errors.New(
			"explicit --demo-url must be an HTTP(S) origin without credentials, path, query or fragment",
		)
	}
	frontend, err := url.Parse(origin)
	if err != nil || frontend == nil || (frontend.Scheme != "http" && frontend.Scheme != "https") ||
		frontend.Hostname() == "" ||
		frontend.User != nil ||
		frontend.Path != "" ||
		frontend.RawQuery != "" ||
		frontend.Fragment != "" {
		return nil, errors.New("--origin must be the configured frontend HTTP(S) origin")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.New("cannot initialize session cookies")
	}
	return &Client{
		base:   strings.TrimSuffix(base.String(), "/") + "/api/v1",
		origin: origin,
		http: &http.Client{
			Jar:           jar,
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) Login(ctx context.Context, password string) error {
	if password == "" {
		return errors.New("DEMO_MANAGER_PASSWORD is required")
	}
	var result dto.AuthResponse
	if err := c.request(
		ctx,
		http.MethodPost,
		"/auth/login",
		"",
		dto.LoginRequest{Email: demoEmail, Password: password},
		&result,
	); err != nil {
		return err
	}
	if result.CSRFToken == "" || result.User.Email != demoEmail || result.User.Role != "manager" ||
		!result.User.IsActive {
		return errors.New("technical: login did not return the demo manager session")
	}
	c.csrf = result.CSRFToken
	return nil
}

func (c *Client) Logout(ctx context.Context) error {
	defer func() { c.csrf = "" }()
	return c.request(ctx, http.MethodPost, "/auth/logout", "", nil, nil)
}

func (c *Client) request(ctx context.Context, method, path, key string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return errors.New("technical: cannot encode API request")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("technical: cannot construct API request")
	}
	req.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Origin", c.origin)
		req.Header.Set("Content-Type", "application/json")
		if c.csrf != "" {
			req.Header.Set("X-CSRF-Token", c.csrf)
		}
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &APIError{Code: "TRANSPORT_ERROR"}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return &APIError{Status: resp.StatusCode, Code: invalidResponseCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var response dto.ErrorResponse
		_ = json.Unmarshal(raw, &response)
		code := safeCode(response.Error.Code)
		refusal := method == http.MethodPost && path == "/orders" && resp.StatusCode == http.StatusConflict &&
			(code == "INSUFFICIENT_STOCK" || code == "RECIPE_CHANGED" || code == "DISH_UNAVAILABLE")
		return &APIError{Status: resp.StatusCode, Code: code, Refusal: refusal}
	}
	if output != nil && json.Unmarshal(raw, output) != nil {
		return &APIError{Status: resp.StatusCode, Code: invalidResponseCode}
	}
	return nil
}

func safeCode(code string) string {
	switch code {
	case "INSUFFICIENT_STOCK",
		"RECIPE_CHANGED",
		"DISH_UNAVAILABLE",
		"VALIDATION_ERROR",
		"INVALID_JSON",
		"SHELF_LIFE_REQUIRED",
		"UNAUTHORIZED",
		"FORBIDDEN",
		"NOT_FOUND",
		"IDEMPOTENCY_CONFLICT",
		"REQUEST_IN_PROGRESS",
		"CONFLICT",
		"SERVICE_UNAVAILABLE",
		"INTERNAL_ERROR",
		"TOO_MANY_REQUESTS",
		"PAYLOAD_TOO_LARGE":
		return code
	default:
		return "HTTP_ERROR"
	}
}

func (c *Client) dish(ctx context.Context, id string) (dto.Dish, error) {
	if id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return dto.Dish{}, errors.New("--dish-id must be a nonzero UUID")
		}
		var dish dto.Dish
		err = c.request(ctx, http.MethodGet, "/dishes/"+parsed.String(), "", nil, &dish)
		return dish, err
	}
	for page := 1; ; page++ {
		var result dto.DishListResponse
		if err := c.request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/dishes?page=%d&page_size=100", page),
			"",
			nil,
			&result,
		); err != nil {
			return dto.Dish{}, err
		}
		for i := range result.Items {
			dish := &result.Items[i]
			if dish.IsActive && dish.RecipeVersion != nil {
				return *dish, nil
			}
		}
		if len(result.Items) == 0 || int64(page)*100 >= result.Total {
			return dto.Dish{}, errors.New("no active demo dish with a recipe; run demo seed first")
		}
	}
}

func basket(dish *dto.Dish, portions int) dto.OrderRequest {
	return dto.OrderRequest{
		Items: []dto.OrderRequestItem{
			{DishID: dish.ID.String(), Portions: portions, RecipeVersion: dish.RecipeVersion},
		},
	}
}

func (c *Client) order(ctx context.Context, dish *dto.Dish, portions int, out io.Writer) error {
	key := uuid.NewString()
	var result dto.OrderDetail
	err := c.request(ctx, http.MethodPost, "/orders", key, basket(dish, portions), &result)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Refusal {
			_, writeErr := fmt.Fprintf(out, "order refused reason=%s key=%s\n", api.Code, key)
			return writeErr
		}
		return err
	}
	if result.ID == uuid.Nil {
		return &APIError{Code: invalidResponseCode}
	}
	_, err = fmt.Fprintf(out, "order created id=%s key=%s portions=%d\n", result.ID, key, portions)
	return err
}

// Order creates one ordinary order with a fresh key; it never reserves stock.
func (c *Client) Order(ctx context.Context, dishID string, out io.Writer) error {
	dish, err := c.dish(ctx, dishID)
	if err != nil {
		return err
	}
	return c.order(ctx, &dish, 1, out)
}
