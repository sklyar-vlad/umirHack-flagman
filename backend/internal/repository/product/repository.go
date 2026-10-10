package product

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

func (r *Repository) List(
	ctx context.Context,
	q repository.DBTX,
	filter model.ProductFilter,
) ([]model.Product, int64, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, 0, err
	}
	offset, err := validations.PaginationOffset(dto.Pagination{Page: filter.Page, PageSize: filter.PageSize})
	if err != nil {
		return nil, 0, err
	}
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if filter.Search != nil && *filter.Search != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*filter.Search)
		args = append(args, "%"+literal+"%")
		conditions = append(conditions, fmt.Sprintf(`p.name ILIKE $%d ESCAPE E'\\'`, len(args)))
	}
	if filter.CategoryID != nil {
		args = append(args, filter.CategoryID.String())
		conditions = append(conditions, fmt.Sprintf("p.category_id = $%d", len(args)))
	}
	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		conditions = append(conditions, fmt.Sprintf("p.is_active = $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int64
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM products p"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	products := make([]model.Product, 0)
	if total == 0 {
		return products, total, nil
	}
	query := fmt.Sprintf(`SELECT p.id,p.name,c.id,c.name,p.unit::text,p.display_unit::text,
		p.standard_shelf_life_hours,p.target_stock::text,p.is_active,p.revision,p.created_at
		FROM products p JOIN product_categories c ON c.id=p.category_id
		%s ORDER BY p.name ASC,p.id ASC LIMIT $%d OFFSET $%d`, where, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, offset)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Category.ID, &p.Category.Name, &p.Unit, &p.DisplayUnit,
			&p.StandardShelfLifeHours, &p.TargetStock, &p.IsActive, &p.Revision, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read products: %w", err)
	}
	return products, total, nil
}
