package menu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Load reads recipes and stock from the same transaction snapshot. Pagination
// follows the shared availability calculation in the service, so total includes
// availability filters and dishes with missing recipes.
func (r *Repository) Load(ctx context.Context, q repository.DBTX, filter model.DishFilter, now time.Time) ([]model.Dish, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	args := []any{now.UTC()}
	conditions := []string{}
	if filter.ID != nil {
		args = append(args, filter.ID.String())
		conditions = append(conditions, fmt.Sprintf("d.id=$%d", len(args)))
	}
	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		conditions = append(conditions, fmt.Sprintf("d.is_active=$%d", len(args)))
	}
	if filter.Search != nil && *filter.Search != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*filter.Search)
		args = append(args, "%"+literal+"%")
		conditions = append(conditions, fmt.Sprintf(`d.name ILIKE $%d ESCAPE E'\\'`, len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	query := `WITH stock AS (
		SELECT product_id,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at>$1),0.000) AS available,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at<=$1),0.000) AS expired
		FROM batches WHERE remaining_quantity>0 GROUP BY product_id
	)
	SELECT d.id,d.name,d.description,d.is_active,d.revision,rv.version,
		p.id,p.name,p.unit::text,ri.quantity_per_portion::text,
		COALESCE(s.available,0.000)::text,COALESCE(s.expired,0.000)::text
	FROM dishes d
	LEFT JOIN LATERAL (SELECT id,version FROM recipe_versions WHERE dish_id=d.id ORDER BY version DESC LIMIT 1) rv ON true
	LEFT JOIN recipe_items ri ON ri.recipe_version_id=rv.id
	LEFT JOIN products p ON p.id=ri.product_id
	LEFT JOIN stock s ON s.product_id=p.id` + where + ` ORDER BY d.name,d.id,p.name,p.id`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load menu: %w", err)
	}
	defer rows.Close()
	dishes := make([]model.Dish, 0)
	for rows.Next() {
		var dish model.Dish
		var productID *uuid.UUID
		var productName, unit, required *string
		var available, expired string
		if err := rows.Scan(&dish.ID, &dish.Name, &dish.Description, &dish.IsActive, &dish.Revision, &dish.RecipeVersion,
			&productID, &productName, &unit, &required, &available, &expired); err != nil {
			return nil, fmt.Errorf("scan menu: %w", err)
		}
		if len(dishes) == 0 || dishes[len(dishes)-1].ID != dish.ID {
			dish.Recipe = []model.RecipeIngredient{}
			dishes = append(dishes, dish)
		}
		if productID != nil {
			last := &dishes[len(dishes)-1]
			last.Recipe = append(last.Recipe, model.RecipeIngredient{
				ProductID: *productID, ProductName: *productName, Unit: *unit, QuantityPerPortion: *required,
				AvailableQuantity: available, ExpiredQuantity: expired,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read menu: %w", err)
	}
	return dishes, nil
}
