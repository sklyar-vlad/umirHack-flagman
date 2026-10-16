-- +goose Up

CREATE VIEW monitoring_stock AS
 SELECT p.id AS product_id,p.name AS product_name,p.unit::text AS unit,p.is_active,p.target_stock,
 COALESCE(sum(b.remaining_quantity) FILTER (WHERE b.expires_at>statement_timestamp()),0) AS valid_quantity,
 COALESCE(sum(b.remaining_quantity) FILTER (WHERE b.expires_at<=statement_timestamp()),0) AS expired_quantity
 FROM products p LEFT JOIN batches b ON b.product_id=p.id AND b.remaining_quantity>0
 GROUP BY p.id;

CREATE VIEW monitoring_problem_batches AS
 SELECT b.id AS batch_id,p.id AS product_id,p.name AS product_name,p.unit::text AS unit,
 b.remaining_quantity,b.expires_at,
 CASE WHEN b.expires_at<=statement_timestamp() THEN 'expired' ELSE 'expiring' END AS state
 FROM batches b JOIN products p ON p.id=b.product_id CROSS JOIN restaurant_settings s
 WHERE s.id=1 AND b.remaining_quantity>0
 AND b.expires_at<=statement_timestamp()+s.expiring_threshold_hours*interval '1 hour';

CREATE VIEW monitoring_stop_list AS
 WITH latest AS (SELECT DISTINCT ON(dish_id) id,dish_id FROM recipe_versions ORDER BY dish_id,version DESC),
 availability AS (
 SELECT l.dish_id,count(ri.id) AS ingredients,
 COALESCE(bool_or(NOT p.is_active OR s.valid_quantity<ri.quantity_per_portion),true) AS shortage
 FROM latest l LEFT JOIN recipe_items ri ON ri.recipe_version_id=l.id
 LEFT JOIN products p ON p.id=ri.product_id LEFT JOIN monitoring_stock s ON s.product_id=p.id GROUP BY l.dish_id)
 SELECT d.id AS dish_id,d.name AS dish_name,
 CASE WHEN COALESCE(a.ingredients,0)=0 THEN 'recipe_missing' ELSE 'ingredient_shortage' END AS reason
 FROM dishes d LEFT JOIN availability a ON a.dish_id=d.id
 WHERE d.is_active AND (COALESCE(a.ingredients,0)=0 OR a.shortage);

CREATE VIEW monitoring_losses AS
 SELECT date_trunc('day',m.occurred_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS time,
 b.product_id,m.unit_snapshot::text AS unit,m.type::text AS reason,
 sum(-m.delta_quantity) AS quantity,count(*) AS movements
 FROM stock_movements m JOIN batches b ON b.id=m.batch_id
 WHERE m.type IN ('expiration','manual_writeoff')
 GROUP BY 1,b.product_id,m.unit_snapshot,m.type;

REVOKE ALL ON monitoring_stock,monitoring_problem_batches,monitoring_stop_list,monitoring_losses FROM PUBLIC;
-- +goose Down
DROP VIEW monitoring_losses,monitoring_stop_list,monitoring_problem_batches,monitoring_stock;
