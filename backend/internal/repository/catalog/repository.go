package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Hold actor rights for the mutation, including all subsequent catalog lock waits.
func (r *Repository) LockActor(ctx context.Context, q repository.DBTX, id uuid.UUID) error {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return err
	}
	var active bool
	var role string
	err = tx.QueryRow(ctx, `SELECT is_active,role::text FROM users WHERE id=$1 FOR SHARE`, id).Scan(&active, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock catalog actor: %w", err)
	}
	if !active || role != "admin" {
		return apperrors.ErrForbidden
	}
	return nil
}

func (r *Repository) ListCategories(ctx context.Context, q repository.DBTX, f model.CategoryFilter) ([]model.CatalogCategory, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: f.Page, PageSize: f.PageSize})
	if err != nil {
		return nil, 0, err
	}
	search := ""
	if f.Search != nil {
		search = *f.Search
	}
	literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
	pattern := "%" + literal + "%"
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_categories WHERE name ILIKE $1 ESCAPE E'\\'`, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count categories: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id,name,revision FROM product_categories WHERE name ILIKE $1 ESCAPE E'\\' ORDER BY name,id LIMIT $2 OFFSET $3`, pattern, f.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	items := make([]model.CatalogCategory, 0)
	for rows.Next() {
		var c model.CatalogCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Revision); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}

func (r *Repository) GetCategory(ctx context.Context, q repository.DBTX, id uuid.UUID, lock string) (model.CatalogCategory, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.CatalogCategory{}, err
	}
	query := `SELECT id,name,revision FROM product_categories WHERE id=$1`
	switch lock {
	case "update":
		query += " FOR UPDATE"
	case "share":
		query += " FOR SHARE"
	case "":
	default:
		return model.CatalogCategory{}, fmt.Errorf("invalid category lock")
	}
	var c model.CatalogCategory
	err = tx.QueryRow(ctx, query, id).Scan(&c.ID, &c.Name, &c.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, apperrors.ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("get category: %w", err)
	}
	return c, nil
}

func (r *Repository) CreateCategory(ctx context.Context, q repository.DBTX, name string) (model.CatalogCategory, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.CatalogCategory{}, err
	}
	var c model.CatalogCategory
	err = tx.QueryRow(ctx, `INSERT INTO product_categories(name) VALUES($1) RETURNING id,name,revision`, name).Scan(&c.ID, &c.Name, &c.Revision)
	return c, err
}

func (r *Repository) UpdateCategory(ctx context.Context, q repository.DBTX, c model.CatalogCategory) (model.CatalogCategory, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return c, err
	}
	err = tx.QueryRow(ctx, `UPDATE product_categories SET name=$2,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING id,name,revision`, c.ID, c.Name).Scan(&c.ID, &c.Name, &c.Revision)
	return c, err
}

func (r *Repository) GetProduct(ctx context.Context, q repository.DBTX, id uuid.UUID, lock bool) (model.Product, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.Product{}, err
	}
	query := `SELECT p.id,p.name,c.id,c.name,p.unit::text,p.display_unit::text,p.standard_shelf_life_hours,p.target_stock::text,p.is_active,p.revision,p.created_at FROM products p JOIN product_categories c ON c.id=p.category_id WHERE p.id=$1`
	if lock {
		query += " FOR UPDATE OF p"
	}
	var p model.Product
	err = tx.QueryRow(ctx, query, id).Scan(&p.ID, &p.Name, &p.Category.ID, &p.Category.Name, &p.Unit, &p.DisplayUnit, &p.StandardShelfLifeHours, &p.TargetStock, &p.IsActive, &p.Revision, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("get catalog product: %w", err)
	}
	return p, nil
}

func (r *Repository) CreateProduct(ctx context.Context, q repository.DBTX, p model.Product) (model.Product, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return p, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO products(name,category_id,unit,display_unit,standard_shelf_life_hours,target_stock,is_active) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, p.Name, p.Category.ID, p.Unit, p.DisplayUnit, p.StandardShelfLifeHours, p.TargetStock, p.IsActive).Scan(&p.ID)
	if err != nil {
		return p, err
	}
	return r.GetProduct(ctx, tx, p.ID, false)
}

func (r *Repository) UpdateProduct(ctx context.Context, q repository.DBTX, p model.Product) (model.Product, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return p, err
	}
	_, err = tx.Exec(ctx, `UPDATE products SET name=$2,category_id=$3,unit=$4,display_unit=$5,standard_shelf_life_hours=$6,target_stock=$7,is_active=$8,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, p.ID, p.Name, p.Category.ID, p.Unit, p.DisplayUnit, p.StandardShelfLifeHours, p.TargetStock, p.IsActive)
	if err != nil {
		return p, err
	}
	return r.GetProduct(ctx, tx, p.ID, false)
}
