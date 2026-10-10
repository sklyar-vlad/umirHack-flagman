-- +goose Up
CREATE TYPE product_unit AS ENUM (
    'g',
    'ml',
    'pcs'
);

CREATE TYPE display_unit AS ENUM (
    'g',
    'kg',
    'ml',
    'l',
    'pcs'
);

CREATE TYPE expiry_source AS ENUM (
    'explicit',
    'calculated'
);

CREATE TYPE movement_type AS ENUM (
    'receipt',
    'order_consumption',
    'expiration',
    'manual_writeoff'
);

CREATE TYPE user_role AS ENUM (
    'admin',
    'manager',
    'storekeeper',
    'order_operator'
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role user_role NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE product_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- +goose Down
DROP TABLE product_categories;
DROP TABLE users;

DROP TYPE user_role;
DROP TYPE movement_type;
DROP TYPE expiry_source;
DROP TYPE display_unit;
DROP TYPE product_unit;
