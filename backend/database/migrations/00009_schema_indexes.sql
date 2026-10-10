-- +goose Up
CREATE UNIQUE INDEX categories_name_ci_uidx
    ON product_categories (lower(name));

CREATE UNIQUE INDEX products_name_ci_uidx
    ON products (lower(name));

CREATE UNIQUE INDEX dishes_name_ci_uidx
    ON dishes (lower(name));

CREATE INDEX products_category_idx
    ON products (category_id);

CREATE INDEX supplies_created_at_id_idx
    ON supplies (created_at, id);

CREATE INDEX batches_fefo_idx
    ON batches (product_id, expires_at, id)
    WHERE remaining_quantity > 0;

CREATE INDEX recipe_items_product_idx
    ON recipe_items (product_id);

CREATE INDEX orders_created_at_id_idx
    ON orders (created_at, id);

CREATE INDEX order_items_recipe_dish_idx
    ON order_items (recipe_version_id, dish_id);

CREATE INDEX stock_movements_batch_occurred_at_id_idx
    ON stock_movements (batch_id, occurred_at, id);

CREATE INDEX stock_movements_order_item_idx
    ON stock_movements (order_item_id);

CREATE INDEX stock_movements_type_occurred_at_idx
    ON stock_movements (type, occurred_at);

CREATE UNIQUE INDEX batch_receipt_once_idx
    ON stock_movements (batch_id)
    WHERE type = 'receipt';

CREATE INDEX order_attempts_created_at_id_idx
    ON order_attempts (created_at, id);

CREATE INDEX sessions_user_idx
    ON sessions (user_id);

CREATE INDEX sessions_expires_at_idx
    ON sessions (expires_at);

CREATE INDEX audit_log_created_at_id_idx
    ON audit_log (created_at, id);

CREATE INDEX audit_log_entity_created_at_idx
    ON audit_log (entity_type, entity_id, created_at);

CREATE INDEX audit_log_actor_created_at_idx
    ON audit_log (actor_id, created_at);

CREATE INDEX idempotency_keys_expires_at_idx
    ON idempotency_keys (expires_at);

CREATE INDEX outbox_events_publication_created_at_id_idx
    ON outbox_events (published_at, created_at, id);

CREATE INDEX outbox_unpublished_idx
    ON outbox_events (created_at, id)
    WHERE published_at IS NULL;

-- +goose Down
DROP INDEX outbox_unpublished_idx;
DROP INDEX outbox_events_publication_created_at_id_idx;

DROP INDEX idempotency_keys_expires_at_idx;

DROP INDEX audit_log_actor_created_at_idx;
DROP INDEX audit_log_entity_created_at_idx;
DROP INDEX audit_log_created_at_id_idx;

DROP INDEX sessions_expires_at_idx;
DROP INDEX sessions_user_idx;

DROP INDEX order_attempts_created_at_id_idx;

DROP INDEX batch_receipt_once_idx;
DROP INDEX stock_movements_type_occurred_at_idx;
DROP INDEX stock_movements_order_item_idx;
DROP INDEX stock_movements_batch_occurred_at_id_idx;

DROP INDEX order_items_recipe_dish_idx;
DROP INDEX orders_created_at_id_idx;
DROP INDEX recipe_items_product_idx;

DROP INDEX batches_fefo_idx;

DROP INDEX supplies_created_at_id_idx;
DROP INDEX products_category_idx;

DROP INDEX dishes_name_ci_uidx;
DROP INDEX products_name_ci_uidx;
DROP INDEX categories_name_ci_uidx;