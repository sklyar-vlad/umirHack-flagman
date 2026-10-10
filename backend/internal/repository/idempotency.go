package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func (r *repository) TryIdempotencyLock(ctx context.Context, q DBTX, scope *model.IdempotencyScope) (bool, error) {
	tx, err := RequireTransaction(q)
	if err != nil {
		return false, err
	}
	// The lock belongs to the transaction, including requests without a stored
	// result. All application instances derive the same key; no local mutex.
	identity := "flagman:idempotency:v1\x00" + scope.UserID.String() + "\x00" + scope.Endpoint() + "\x00" + scope.Key.String()
	hash := sha256.Sum256([]byte(identity))
	lockID := int64(binary.BigEndian.Uint64(hash[:8]))
	var acquired bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&acquired); err != nil {
		return false, fmt.Errorf("lock idempotency scope: %w", err)
	}
	return acquired, nil
}

func (r *repository) FindIdempotencyResult(
	ctx context.Context,
	q DBTX,
	scope *model.IdempotencyScope,
) (*model.IdempotencyRecord, error) {
	tx, err := RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	const query = `
		SELECT request_hash, response_status, response_body, response_headers
		FROM idempotency_keys
		WHERE user_id = $1 AND endpoint = $2 AND key = $3
			AND expires_at > clock_timestamp()
	`
	var record model.IdempotencyRecord
	var headers []byte
	err = tx.QueryRow(ctx, query, scope.UserID.String(), scope.Endpoint(), scope.Key.String()).
		Scan(&record.RequestHash, &record.Response.Status, &record.Response.Body, &headers)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read idempotency result: %w", err)
	}
	if err := restoreResponseHeaders(&record.Response, headers); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *repository) SaveIdempotencyResult(
	ctx context.Context,
	q DBTX,
	scope *model.IdempotencyScope,
	requestHash string,
	response *model.IdempotencyResponse,
) (model.IdempotencyResponse, error) {
	tx, err := RequireTransaction(q)
	if err != nil {
		return model.IdempotencyResponse{}, err
	}
	// Only Location is persisted: authorization, cookies and per-request
	// headers must never be replayed from an idempotency record.
	headers := map[string]string{}
	if response.Location != "" {
		headers["Location"] = response.Location
	}
	headerJSON, err := json.Marshal(headers)
	if err != nil {
		return model.IdempotencyResponse{}, fmt.Errorf("encode idempotency headers: %w", err)
	}
	body := response.Body
	if len(body) == 0 {
		body = json.RawMessage("null")
	}
	const query = `
		INSERT INTO idempotency_keys (
			user_id, endpoint, key, request_hash, response_status, response_body,
			response_headers, created_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, clock_timestamp(), clock_timestamp() + INTERVAL '24 hours')
		ON CONFLICT (user_id, endpoint, key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash, response_status = EXCLUDED.response_status,
			response_body = EXCLUDED.response_body, response_headers = EXCLUDED.response_headers,
			created_at = EXCLUDED.created_at, expires_at = EXCLUDED.expires_at
		WHERE idempotency_keys.expires_at <= clock_timestamp()
		RETURNING response_status, response_body, response_headers
	`
	var saved model.IdempotencyResponse
	var savedHeaders []byte
	err = tx.QueryRow(ctx, query, scope.UserID.String(), scope.Endpoint(), scope.Key.String(),
		requestHash, response.Status, body, headerJSON).Scan(&saved.Status, &saved.Body, &savedHeaders)
	if err != nil {
		return model.IdempotencyResponse{}, fmt.Errorf("save idempotency result: %w", err)
	}
	// Return PostgreSQL's JSONB representation on the first request as well,
	// so a retry receives exactly the same bytes, not just equivalent JSON.
	if err := restoreResponseHeaders(&saved, savedHeaders); err != nil {
		return model.IdempotencyResponse{}, err
	}
	return saved, nil
}

func restoreResponseHeaders(response *model.IdempotencyResponse, raw []byte) error {
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return fmt.Errorf("decode idempotency headers: %w", err)
	}
	response.Location = headers["Location"]
	if response.Status == 204 || response.Status == 205 || response.Status == 304 {
		response.Body = nil
	}
	return nil
}

// DeleteExpiredIdempotencyResults removes only the response cache. Domain
// history (including order_attempts), audit and outbox have independent lifetimes.
func (r *repository) DeleteExpiredIdempotencyResults(ctx context.Context, q DBTX, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("idempotency cleanup limit must be between 1 and 1000")
	}
	tx, err := RequireTransaction(q)
	if err != nil {
		return 0, err
	}
	const query = `
		WITH expired AS (
			SELECT id FROM idempotency_keys
			WHERE expires_at <= statement_timestamp()
			ORDER BY expires_at, id LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM idempotency_keys AS cache USING expired
		WHERE cache.id = expired.id AND cache.expires_at <= statement_timestamp()
	`
	result, err := tx.Exec(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency results: %w", err)
	}
	return result.RowsAffected(), nil
}
