package integration

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }
func (r *Repository) LockActor(ctx context.Context, q repository.DBTX, id uuid.UUID, admin bool) error {
	var role string
	err := q.QueryRow(ctx, `SELECT role::text FROM users WHERE id=$1 AND is_active FOR SHARE`, id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrForbidden
	}
	if err != nil {
		return err
	}
	if role != "admin" && (admin || role != "manager" && role != "storekeeper") {
		return apperrors.ErrForbidden
	}
	return nil
}

func (r *Repository) Authenticate(
	ctx context.Context,
	q repository.DBTX,
	hash []byte,
	lock bool,
) (dto.IntegrationPrincipal, error) {
	var p dto.IntegrationPrincipal
	sql := `SELECT t.id,u.id FROM integration_tokens t JOIN users u ON u.id=t.created_by WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND u.is_active AND u.role='admin'`
	if lock {
		sql += " FOR SHARE OF t,u"
	}
	err := q.QueryRow(ctx, sql, hash).Scan(&p.TokenID, &p.ActorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.ErrUnauthorized
	}
	return p, err
}

func (r *Repository) LockToken(ctx context.Context, q repository.DBTX, p dto.IntegrationPrincipal) error {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT t.id FROM integration_tokens t JOIN users u ON u.id=t.created_by WHERE t.id=$1 AND t.created_by=$2 AND t.revoked_at IS NULL AND u.is_active AND u.role='admin' FOR SHARE OF t,u`, p.TokenID, p.ActorID).
		Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrUnauthorized
	}
	return err
}

func (r *Repository) CreateToken(
	ctx context.Context,
	q repository.DBTX,
	actor uuid.UUID,
	name string,
	hash []byte,
) (dto.IntegrationToken, error) {
	var t dto.IntegrationToken
	err := q.QueryRow(ctx, `INSERT INTO integration_tokens(name,token_hash,created_by) VALUES($1,$2,$3) RETURNING id,name,created_at,revoked_at`, name, hash, actor).
		Scan(&t.ID, &t.Name, &t.CreatedAt, &t.RevokedAt)
	return t, err
}

func (r *Repository) Tokens(ctx context.Context, q repository.DBTX) ([]dto.IntegrationToken, error) {
	rows, err := q.Query(
		ctx,
		`SELECT id,name,created_at,revoked_at FROM integration_tokens ORDER BY created_at DESC,id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dto.IntegrationToken{}
	for rows.Next() {
		var t dto.IntegrationToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *Repository) Revoke(ctx context.Context, q repository.DBTX, id uuid.UUID) error {
	tag, err := q.Exec(
		ctx,
		`UPDATE integration_tokens SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE id=$1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *Repository) LockExternal(ctx context.Context, q repository.DBTX, token uuid.UUID, kind, id string) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, token.String()+":"+kind+":"+id)
	return err
}

func (r *Repository) ProductMapping(
	ctx context.Context,
	q repository.DBTX,
	token uuid.UUID,
	external string,
) (uuid.UUID, []byte, error) {
	var id uuid.UUID
	var hash []byte
	err := q.QueryRow(ctx, `SELECT product_id,request_hash FROM integration_products WHERE token_id=$1 AND external_id=$2`, token, external).
		Scan(&id, &hash)
	return id, hash, err
}

func (r *Repository) MapProduct(
	ctx context.Context,
	q repository.DBTX,
	token uuid.UUID,
	external string,
	hash []byte,
	id uuid.UUID,
) error {
	_, err := q.Exec(
		ctx,
		`INSERT INTO integration_products(token_id,external_id,request_hash,product_id) VALUES($1,$2,$3,$4)`,
		token,
		external,
		hash,
		id,
	)
	return err
}

func (r *Repository) FindIncoming(
	ctx context.Context,
	q repository.DBTX,
	token uuid.UUID,
	external string,
) (uuid.UUID, []byte, error) {
	var id uuid.UUID
	var hash []byte
	err := q.QueryRow(ctx, `SELECT id,request_hash FROM incoming_supplies WHERE token_id=$1 AND external_id=$2`, token, external).
		Scan(&id, &hash)
	return id, hash, err
}

func (r *Repository) InsertIncoming(
	ctx context.Context,
	q repository.DBTX,
	token uuid.UUID,
	input dto.IncomingSupplyRequest,
	hash []byte,
) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `INSERT INTO incoming_supplies(token_id,external_id,number,request_hash) VALUES($1,$2,$3,$4) RETURNING id`, token, input.ExternalID, input.Number, hash).
		Scan(&id)
	if err != nil {
		return id, err
	}
	for i, item := range input.Items {
		if _, err = q.Exec(
			ctx,
			`INSERT INTO incoming_supply_items(incoming_supply_id,line_number,product_id,quantity,expires_at,unit_snapshot) VALUES($1,$2,$3,$4,$5::timestamptz,(SELECT unit FROM products WHERE id=$3))`,
			id,
			i+1,
			item.ProductID,
			item.Quantity,
			item.ExpiresAt,
		); err != nil {
			return id, err
		}
	}
	return id, nil
}

func (r *Repository) GetIncoming(
	ctx context.Context,
	q repository.DBTX,
	id uuid.UUID,
	lock bool,
) (dto.IncomingSupply, error) {
	var doc dto.IncomingSupply
	sql := `SELECT s.id,s.external_id,s.number,t.name,s.status,s.revision,s.created_at FROM incoming_supplies s JOIN integration_tokens t ON t.id=s.token_id WHERE s.id=$1`
	if lock {
		sql += " FOR UPDATE OF s"
	}
	err := q.QueryRow(ctx, sql, id).
		Scan(&doc.ID, &doc.ExternalID, &doc.Number, &doc.Source, &doc.Status, &doc.Revision, &doc.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return doc, apperrors.ErrNotFound
	}
	if err != nil {
		return doc, err
	}
	rows, err := q.Query(
		ctx,
		`SELECT i.id,i.product_id,p.name,i.unit_snapshot::text,i.quantity::text,i.accepted_quantity::text,i.expires_at,i.line_number FROM incoming_supply_items i JOIN products p ON p.id=i.product_id WHERE i.incoming_supply_id=$1 ORDER BY i.line_number`,
		id,
	)
	if err != nil {
		return doc, err
	}
	doc.Items = []dto.IncomingSupplyItem{}
	for rows.Next() {
		var item dto.IncomingSupplyItem
		if err := rows.Scan(
			&item.ID,
			&item.ProductID,
			&item.ProductName,
			&item.Unit,
			&item.Quantity,
			&item.AcceptedQuantity,
			&item.ExpiresAt,
			&item.LineNumber,
		); err != nil {
			rows.Close()
			return doc, err
		}
		doc.Items = append(doc.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return doc, err
	}
	rows, err = q.Query(
		ctx,
		`SELECT r.supply_id FROM incoming_supply_receipts r JOIN supplies s ON s.id=r.supply_id WHERE r.incoming_supply_id=$1 ORDER BY s.created_at,s.id`,
		id,
	)
	if err != nil {
		return doc, err
	}
	defer rows.Close()
	doc.ReceiptIDs = []uuid.UUID{}
	for rows.Next() {
		var receipt uuid.UUID
		if err := rows.Scan(&receipt); err != nil {
			return doc, err
		}
		doc.ReceiptIDs = append(doc.ReceiptIDs, receipt)
	}
	return doc, rows.Err()
}

func (r *Repository) ListIncoming(
	ctx context.Context,
	q repository.DBTX,
	status string,
	page, size int,
) (dto.ListResponse[dto.IncomingSupply], error) {
	result := dto.ListResponse[dto.IncomingSupply]{
		Items:    []dto.IncomingSupply{},
		Page:     page,
		PageSize: size,
		Meta:     map[string]any{},
	}
	err := q.QueryRow(ctx, `SELECT count(*) FROM incoming_supplies WHERE ($1='all' OR status=$1)`, status).
		Scan(&result.Total)
	if err != nil {
		return result, err
	}
	rows, err := q.Query(
		ctx,
		`SELECT id FROM incoming_supplies WHERE ($1='all' OR status=$1) ORDER BY created_at DESC,id LIMIT $2 OFFSET $3`,
		status,
		size,
		(int64(page)-1)*int64(size),
	)
	if err != nil {
		return result, err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		doc, err := r.GetIncoming(ctx, q, id, false)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, doc)
	}
	return result, nil
}

func (r *Repository) AcceptLine(ctx context.Context, q repository.DBTX, id uuid.UUID, quantity string) error {
	_, err := q.Exec(
		ctx,
		`UPDATE incoming_supply_items SET accepted_quantity=accepted_quantity+$2::numeric WHERE id=$1`,
		id,
		quantity,
	)
	return err
}

func (r *Repository) FinishReceipt(ctx context.Context, q repository.DBTX, id, supply uuid.UUID) error {
	_, err := q.Exec(
		ctx,
		`INSERT INTO incoming_supply_receipts(incoming_supply_id,supply_id) VALUES($1,$2)`,
		id,
		supply,
	)
	if err != nil {
		return err
	}
	_, err = q.Exec(
		ctx,
		`UPDATE incoming_supplies s SET revision=revision+1,status=CASE WHEN EXISTS(SELECT 1 FROM incoming_supply_items i WHERE i.incoming_supply_id=s.id AND i.accepted_quantity<i.quantity) THEN 'partial' ELSE 'received' END WHERE s.id=$1`,
		id,
	)
	return err
}

func (r *Repository) Token(ctx context.Context, q repository.DBTX, id uuid.UUID) (dto.IntegrationToken, error) {
	var result dto.IntegrationToken
	err := q.QueryRow(ctx, `SELECT id,name,created_at,revoked_at FROM integration_tokens WHERE id=$1 FOR UPDATE`, id).
		Scan(&result.ID, &result.Name, &result.CreatedAt, &result.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, apperrors.ErrNotFound
	}
	return result, err
}
