-- +goose Up
CREATE TABLE integration_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ
);
CREATE TABLE incoming_supplies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id UUID NOT NULL REFERENCES integration_tokens(id) ON DELETE RESTRICT,
    external_id VARCHAR(100) NOT NULL,
    number VARCHAR(80) NOT NULL,
    request_hash BYTEA NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','partial','received')),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(token_id,external_id)
);
CREATE TABLE incoming_supply_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incoming_supply_id UUID NOT NULL REFERENCES incoming_supplies(id) ON DELETE RESTRICT,
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(14,3) NOT NULL CHECK (quantity > 0),
    unit_snapshot product_unit NOT NULL,
    accepted_quantity NUMERIC(14,3) NOT NULL DEFAULT 0 CHECK (accepted_quantity >= 0 AND accepted_quantity <= quantity),
    expires_at TIMESTAMPTZ NOT NULL,
    UNIQUE(incoming_supply_id,line_number)
);
CREATE TABLE incoming_supply_receipts (
    incoming_supply_id UUID NOT NULL REFERENCES incoming_supplies(id) ON DELETE RESTRICT,
    supply_id UUID NOT NULL UNIQUE REFERENCES supplies(id) ON DELETE RESTRICT,
    PRIMARY KEY(incoming_supply_id,supply_id)
);
CREATE TABLE integration_products (
    token_id UUID NOT NULL REFERENCES integration_tokens(id) ON DELETE RESTRICT,
    external_id VARCHAR(100) NOT NULL,
    request_hash BYTEA NOT NULL,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    PRIMARY KEY(token_id,external_id)
);
CREATE TABLE hot_offer_preferences (
    dish_id UUID PRIMARY KEY REFERENCES dishes(id) ON DELETE RESTRICT,
    is_published BOOLEAN NOT NULL DEFAULT true,
    is_manual BOOLEAN NOT NULL DEFAULT false,
    message VARCHAR(500) NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX incoming_supplies_created ON incoming_supplies(created_at DESC,id);
CREATE INDEX incoming_supply_items_product ON incoming_supply_items(product_id);
-- +goose Down
DROP TABLE hot_offer_preferences;
DROP TABLE integration_products;
DROP TABLE incoming_supply_receipts;
DROP TABLE incoming_supply_items;
DROP TABLE incoming_supplies;
DROP TABLE integration_tokens;
