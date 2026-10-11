package auditlog

import (
	"context"
	"fmt"
	"strings"

	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }
func conditions(f model.AuditLogFilter) (string, []any) {
	parts := []string{}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		parts = append(parts, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Search)
		add(`(COALESCE(u.name,'system Система') ILIKE $%[1]d ESCAPE E'\\' OR a.action ILIKE $%[1]d ESCAPE E'\\' OR a.entity_type ILIKE $%[1]d ESCAPE E'\\' OR a.entity_id::text ILIKE $%[1]d ESCAPE E'\\' OR a.request_id ILIKE $%[1]d ESCAPE E'\\')`, "%"+escaped+"%")
	}
	if f.SystemActor {
		parts = append(parts, "a.actor_id IS NULL")
	} else if f.ActorID != nil {
		add("a.actor_id=$%d", *f.ActorID)
	}
	if f.Action != "" {
		add("a.action=$%d", string(f.Action))
	}
	if f.EntityType != "" {
		add("a.entity_type=$%d", string(f.EntityType))
	}
	if f.EntityID != nil {
		add("a.entity_id=$%d", *f.EntityID)
	}
	if f.DateFrom != nil {
		add("a.created_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("a.created_at < $%d", *f.DateTo)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}
func (r *Repository) List(ctx context.Context, q repository.DBTX, f model.AuditLogFilter) ([]model.AuditLogRecord, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	where, args := conditions(f)
	const from = ` FROM audit_log a LEFT JOIN users u ON u.id=a.actor_id`
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*)`+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit log: %w", err)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT a.id,a.created_at,a.actor_id,u.name,a.action,a.entity_type,a.entity_id,a.request_id,a.before_data,a.after_data`+from+where+` ORDER BY a.created_at DESC,a.id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2), append(args, f.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit log: %w", err)
	}
	defer rows.Close()
	result := make([]model.AuditLogRecord, 0)
	for rows.Next() {
		var v model.AuditLogRecord
		if err := rows.Scan(&v.ID, &v.CreatedAt, &v.ActorID, &v.ActorName, &v.Action, &v.EntityType, &v.EntityID, &v.RequestID, &v.Before, &v.After); err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read audit log: %w", err)
	}
	return result, total, nil
}
