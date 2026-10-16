-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL,
    unit product_unit NOT NULL,
    display_unit display_unit NOT NULL,
    standard_shelf_life_hours INTEGER,
    target_stock NUMERIC(14, 3),
    is_active BOOLEAN NOT NULL DEFAULT true,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT products_category_fk
        FOREIGN KEY (category_id)
        REFERENCES product_categories (id)
        ON DELETE RESTRICT
);

CREATE TABLE supplies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number VARCHAR(80) NOT NULL UNIQUE,
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID NOT NULL,

    CONSTRAINT supplies_created_by_fk
        FOREIGN KEY (created_by)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

CREATE TABLE dishes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    description VARCHAR(2000),
    is_active BOOLEAN NOT NULL DEFAULT true,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- +goose Down
DROP TABLE dishes;
DROP TABLE supplies;
DROP TABLE products;