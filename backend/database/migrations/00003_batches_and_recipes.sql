-- +goose Up
CREATE TABLE batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supply_id UUID NOT NULL,
    product_id UUID NOT NULL,
    line_number INTEGER NOT NULL,
    initial_quantity NUMERIC(14, 3) NOT NULL,
    remaining_quantity NUMERIC(14, 3) NOT NULL,
    produced_at TIMESTAMPTZ,
    shelf_life_hours INTEGER,
    expiry_source expiry_source NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT batches_supply_fk
        FOREIGN KEY (supply_id)
        REFERENCES supplies (id)
        ON DELETE RESTRICT,

    CONSTRAINT batches_product_fk
        FOREIGN KEY (product_id)
        REFERENCES products (id)
        ON DELETE RESTRICT,

    CONSTRAINT batches_supply_line_unique
        UNIQUE (supply_id, line_number)
);

CREATE TABLE recipe_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dish_id UUID NOT NULL,
    version INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID NOT NULL,

    CONSTRAINT recipe_versions_dish_fk
        FOREIGN KEY (dish_id)
        REFERENCES dishes (id)
        ON DELETE RESTRICT,

    CONSTRAINT recipe_versions_created_by_fk
        FOREIGN KEY (created_by)
        REFERENCES users (id)
        ON DELETE RESTRICT,

    CONSTRAINT recipe_versions_dish_version_unique
        UNIQUE (dish_id, version),
    
    CONSTRAINT recipe_versions_id_dish_unique
        UNIQUE (id, dish_id)
);

CREATE TABLE recipe_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_version_id UUID NOT NULL,
    product_id UUID NOT NULL,
    quantity_per_portion NUMERIC(14, 3) NOT NULL,

    CONSTRAINT recipe_items_recipe_version_fk
        FOREIGN KEY (recipe_version_id)
        REFERENCES recipe_versions (id)
        ON DELETE RESTRICT,

    CONSTRAINT recipe_items_product_fk
        FOREIGN KEY (product_id)
        REFERENCES products (id)
        ON DELETE RESTRICT,

    CONSTRAINT recipe_items_version_product_unique
        UNIQUE (recipe_version_id, product_id)
);

-- +goose Down
DROP TABLE recipe_items;
DROP TABLE recipe_versions;
DROP TABLE batches;
