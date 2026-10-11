package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const safeColumns = `id,name,email,role::text,is_active,revision,created_at,last_login_at`

func destinations(u *model.User) []any {
	return []any{&u.ID, &u.Name, &u.Email, &u.Role, &u.IsActive, &u.Revision, &u.CreatedAt, &u.LastLoginAt}
}

func userError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) {
		if pgerr.Code == "23505" && pgerr.ConstraintName == "users_email_key" {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("%s: database error (SQLSTATE %s)", operation, pgerr.Code)
	}
	return fmt.Errorf("%s: database operation failed", operation)
}

func (r *Repository) Get(ctx context.Context, q repository.DBTX, id uuid.UUID, lock bool) (model.User, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.User{}, err
	}
	sql := `SELECT ` + safeColumns + ` FROM users WHERE id=$1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var u model.User
	if err := tx.QueryRow(ctx, sql, id).Scan(destinations(&u)...); err != nil {
		return model.User{}, userError("read user", err)
	}
	return u, nil
}

func (r *Repository) List(ctx context.Context, q repository.DBTX, f model.UserFilter) ([]model.User, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	conditions := []string{}
	args := []any{}
	if f.Search != nil && *f.Search != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*f.Search)
		args = append(args, "%"+literal+"%")
		conditions = append(conditions, fmt.Sprintf(`(name ILIKE $%d ESCAPE E'\\' OR email ILIKE $%d ESCAPE E'\\')`, len(args), len(args)))
	}
	if f.Role != nil {
		args = append(args, *f.Role)
		conditions = append(conditions, fmt.Sprintf(`role=$%d::user_role`, len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		conditions = append(conditions, fmt.Sprintf(`is_active=$%d`, len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, userError("count users", err)
	}
	direction := "ASC"
	if f.Descending {
		direction = "DESC"
	}
	sql := fmt.Sprintf(`SELECT %s FROM users%s ORDER BY name %s,id ASC LIMIT $%d OFFSET $%d`, safeColumns, where, direction, len(args)+1, len(args)+2)
	args = append(args, f.PageSize, offset)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, userError("list users", err)
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(destinations(&u)...); err != nil {
			return nil, 0, userError("scan user", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, userError("read users", err)
	}
	return users, total, nil
}

func (r *Repository) Create(ctx context.Context, q repository.DBTX, u model.User, hash string) (model.User, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.User{}, err
	}
	var result model.User
	err = tx.QueryRow(ctx, `INSERT INTO users(name,email,role,is_active,password_hash) VALUES($1,$2,$3,$4,$5) RETURNING `+safeColumns, u.Name, u.Email, u.Role, u.IsActive, hash).Scan(destinations(&result)...)
	if err != nil {
		return model.User{}, userError("create user", err)
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, q repository.DBTX, u model.User) (model.User, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.User{}, err
	}
	var result model.User
	err = tx.QueryRow(ctx, `UPDATE users SET name=$2,email=$3,role=$4,is_active=$5,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+safeColumns, u.ID, u.Name, u.Email, u.Role, u.IsActive).Scan(destinations(&result)...)
	if err != nil {
		return model.User{}, userError("update user", err)
	}
	return result, nil
}

func (r *Repository) SetPassword(ctx context.Context, q repository.DBTX, id uuid.UUID, hash string) (model.User, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.User{}, err
	}
	var result model.User
	err = tx.QueryRow(ctx, `UPDATE users SET password_hash=$2,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+safeColumns, id, hash).Scan(destinations(&result)...)
	if err != nil {
		return model.User{}, userError("reset password", err)
	}
	return result, nil
}

// The two-int advisory namespace is distinct from the bigint idempotency locks.
// Every administration write acquires this lock before any user row lock.
func (r *Repository) LockAdministration(ctx context.Context, q repository.DBTX) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(160016,0)`)
	if err != nil {
		return userError("lock administration", err)
	}
	return nil
}

func (r *Repository) CountActiveAdmins(ctx context.Context, q repository.DBTX) (int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin' AND is_active`).Scan(&count); err != nil {
		return 0, userError("count active administrators", err)
	}
	return count, nil
}

func (r *Repository) RevokeSessions(ctx context.Context, q repository.DBTX, id uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	if err != nil {
		return userError("revoke user sessions", err)
	}
	return nil
}
