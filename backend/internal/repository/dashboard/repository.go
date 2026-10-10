package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Load reads each warehouse product aggregate once, including products without
// stock or a target. Recipes and procurement reuse these exact quantities.
func (r *Repository) Load(ctx context.Context, q repository.DBTX, now time.Time) (model.DashboardData, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return model.DashboardData{}, err
	}
	data := model.DashboardData{Stock: []model.ProcurementStock{}}
	if err := tx.QueryRow(ctx, `SELECT
		COALESCE(s.restaurant_name,''),COALESCE(s.timezone,'Europe/Moscow'),COALESCE(s.expiring_threshold_hours,24)
		FROM (SELECT 1) singleton LEFT JOIN restaurant_settings s ON s.id=1`).Scan(
		&data.Settings.RestaurantName, &data.Settings.Timezone, &data.Settings.ExpiringThresholdHours); err != nil {
		return model.DashboardData{}, fmt.Errorf("read dashboard settings: %w", err)
	}
	warningAt := now.Add(time.Duration(data.Settings.ExpiringThresholdHours) * time.Hour)
	rows, err := tx.Query(ctx, `WITH stock AS (
		SELECT product_id,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at>$1),0.000) AS available,
			COALESCE(SUM(remaining_quantity) FILTER (WHERE expires_at<=$1),0.000) AS expired,
			COUNT(*) FILTER (WHERE expires_at>$1 AND expires_at<=$2) AS expiring_count,
			COUNT(*) FILTER (WHERE expires_at<=$1) AS expired_count
		FROM batches WHERE remaining_quantity>0 GROUP BY product_id
	)
	SELECT p.id,p.name,p.unit::text,p.display_unit::text,p.target_stock::text,
		COALESCE(s.available,0.000)::text,COALESCE(s.expired,0.000)::text,
		COALESCE(s.expiring_count,0),COALESCE(s.expired_count,0)
	FROM products p LEFT JOIN stock s ON s.product_id=p.id ORDER BY p.name,p.id`, now.UTC(), warningAt.UTC())
	if err != nil {
		return model.DashboardData{}, fmt.Errorf("load dashboard stock: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stock model.ProcurementStock
		var expiring, expired int64
		p := &stock.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Unit, &p.DisplayUnit, &p.TargetStock,
			&stock.AvailableQuantity, &stock.ExpiredQuantity, &expiring, &expired); err != nil {
			return model.DashboardData{}, fmt.Errorf("scan dashboard stock: %w", err)
		}
		data.Stock = append(data.Stock, stock)
		data.ExpiringBatchesCount += expiring
		data.ExpiredBatchesCount += expired
	}
	if err := rows.Err(); err != nil {
		return model.DashboardData{}, fmt.Errorf("read dashboard stock: %w", err)
	}
	return data, nil
}

// LoadRecipes reads current active recipes without repeating the stock aggregate.
// Availability quantities are populated by the service from Load's snapshot.
func (r *Repository) LoadRecipes(ctx context.Context, q repository.DBTX) ([]model.Dish, error) {
	tx, err := repository.RequireTransaction(q)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id,d.name,d.description,d.is_active,d.revision,rv.version,rv.id,
		p.id,p.name,p.unit::text,ri.quantity_per_portion::text
		FROM dishes d
		LEFT JOIN LATERAL (SELECT id,version FROM recipe_versions WHERE dish_id=d.id ORDER BY version DESC LIMIT 1) rv ON true
		LEFT JOIN recipe_items ri ON ri.recipe_version_id=rv.id
		LEFT JOIN products p ON p.id=ri.product_id
		WHERE d.is_active ORDER BY d.name,d.id,p.name,p.id`)
	if err != nil {
		return nil, fmt.Errorf("load dashboard recipes: %w", err)
	}
	defer rows.Close()
	dishes := []model.Dish{}
	for rows.Next() {
		var dish model.Dish
		var productID *uuid.UUID
		var name, unit, required *string
		if err := rows.Scan(
			&dish.ID,
			&dish.Name,
			&dish.Description,
			&dish.IsActive,
			&dish.Revision,
			&dish.RecipeVersion,
			&dish.RecipeVersionID,
			&productID,
			&name,
			&unit,
			&required,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard recipe: %w", err)
		}
		if len(dishes) == 0 || dishes[len(dishes)-1].ID != dish.ID {
			dish.Recipe = []model.RecipeIngredient{}
			dishes = append(dishes, dish)
		}
		if productID != nil {
			last := &dishes[len(dishes)-1]
			last.Recipe = append(
				last.Recipe,
				model.RecipeIngredient{
					ProductID:          *productID,
					ProductName:        *name,
					Unit:               *unit,
					QuantityPerPortion: *required,
				},
			)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read dashboard recipes: %w", err)
	}
	return dishes, nil
}
