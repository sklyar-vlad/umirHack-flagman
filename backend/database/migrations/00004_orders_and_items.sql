-- +goose Up
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number BIGINT GENERATED ALWAYS AS IDENTITY NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID NOT NULL,

    CONSTRAINT orders_created_by_fk
        FOREIGN KEY (created_by)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

CREATE TABLE order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    dish_id UUID NOT NULL,
    recipe_version_id UUID NOT NULL,
    dish_name_snapshot VARCHAR(200) NOT NULL,
    portions INTEGER NOT NULL,

    CONSTRAINT order_items_order_fk
        FOREIGN KEY (order_id)
        REFERENCES orders (id)
        ON DELETE RESTRICT,

    CONSTRAINT order_items_dish_fk
        FOREIGN KEY (dish_id)
        REFERENCES dishes (id)
        ON DELETE RESTRICT,

    CONSTRAINT order_items_recipe_dish_fk
        FOREIGN KEY (recipe_version_id, dish_id)
        REFERENCES recipe_versions (id, dish_id)
        ON DELETE RESTRICT,

    CONSTRAINT order_items_order_dish_unique
        UNIQUE(order_id, dish_id)
);

-- +goose Down
DROP TABLE order_items;
DROP TABLE orders;