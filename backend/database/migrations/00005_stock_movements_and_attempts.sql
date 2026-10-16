-- +goose Up
CREATE TABLE stock_movements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL,
    order_item_id UUID,
    type movement_type NOT NULL,
    delta_quantity NUMERIC(14, 3) NOT NULL,
    remaining_quantity_after NUMERIC(14, 3) NOT NULL,
    product_name_snapshot VARCHAR(200) NOT NULL,
    unit_snapshot product_unit NOT NULL,
    actor_id UUID,
    occurred_at TIMESTAMPTZ NOT NULL,
    note VARCHAR(500),

    CONSTRAINT stock_movements_batch_fk
        FOREIGN KEY (batch_id)
        REFERENCES batches (id)
        ON DELETE RESTRICT,

    CONSTRAINT stock_movements_order_item_fk
        FOREIGN KEY (order_item_id)
        REFERENCES order_items (id)
        ON DELETE RESTRICT,

    CONSTRAINT stock_movements_actor_fk
        FOREIGN KEY (actor_id)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

CREATE TABLE order_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason_code VARCHAR(40) NOT NULL,
    items_snapshot JSONB NOT NULL,
    shortages JSONB NOT NULL DEFAULT '[]'::JSONB,
    issues JSONB NOT NULL DEFAULT '[]'::JSONB,

    CONSTRAINT order_attempts_created_by_fk
        FOREIGN KEY (created_by)
        REFERENCES users (id)
        ON DELETE RESTRICT
);
-- +goose Down
DROP TABLE order_attempts;
DROP TABLE stock_movements;