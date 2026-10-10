package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestIdempotencyHTTPConflictResponses(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		code       string
		retryAfter string
	}{
		{"in progress", apperrors.ErrRequestInProgress, "REQUEST_IN_PROGRESS", "1"},
		{"different body", apperrors.ErrIdempotencyConflict, "IDEMPOTENCY_CONFLICT", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestID())
			router.POST("/test", func(c *gin.Context) { writeError(c, fmt.Errorf("wrapped: %w", test.err)) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", http.NoBody))
			var response dto.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusConflict || response.Error.Code != test.code ||
				recorder.Header().
					Get("Retry-After") !=
					test.retryAfter || response.RequestID != recorder.Header().Get("X-Request-ID") {
				t.Fatalf("incorrect idempotency error: %d, %+v, %v", recorder.Code, response, recorder.Header())
			}
		})
	}
}

func TestReadIdempotencyKey(t *testing.T) {
	valid := uuid.New()
	for _, test := range []struct {
		name   string
		values []string
		valid  bool
	}{
		{"valid", []string{valid.String()}, true},
		{"missing", nil, false},
		{"empty", []string{""}, false},
		{"malformed", []string{"not-a-uuid"}, false},
		{"zero", []string{uuid.Nil.String()}, false},
		{"repeated", []string{valid.String(), valid.String()}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
			for _, value := range test.values {
				c.Request.Header.Add("Idempotency-Key", value)
			}
			key, err := ReadIdempotencyKey(c)
			if test.valid {
				if err != nil || key != valid {
					t.Fatalf("valid key rejected: %v", err)
				}
			} else if err == nil || key != uuid.Nil {
				t.Fatalf("invalid key accepted: %v", key)
			}
		})
	}
}

func TestWriteIdempotencyResponseKeepsBodyAndFreshRequestID(t *testing.T) {
	saved := model.IdempotencyResponse{
		Status: http.StatusCreated, Body: json.RawMessage(`{"id":"saved","request_id":"original"}`),
		Location: "/api/v1/supplies/saved",
	}
	router := gin.New()
	router.Use(RequestID())
	router.POST("/test", func(c *gin.Context) { WriteIdempotencyResponse(c, &saved) })
	var previousID string
	for range 2 {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", http.NoBody))
		requestID := recorder.Header().Get("X-Request-ID")
		if recorder.Code != saved.Status || !bytes.Equal(recorder.Body.Bytes(), saved.Body) ||
			recorder.Header().
				Get("Location") !=
				saved.Location || requestID == "" || requestID == "original" || requestID == previousID {
			t.Fatalf(
				"saved response changed or stale request header: %s, %v",
				recorder.Body.String(),
				recorder.Header(),
			)
		}
		previousID = requestID
	}
}
