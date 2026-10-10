-- +goose Up
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    csrf_token VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,

    CONSTRAINT sessions_user_fk
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

CREATE TABLE restaurant_settings (
    id INTEGER PRIMARY KEY DEFAULT 1,
    restaurant_name VARCHAR(200) NOT NULL,
    timezone VARCHAR(100) NOT NULL DEFAULT 'Europe/Moscow',
    expiring_threshold_hours INTEGER NOT NULL DEFAULT 24,
    auto_writeoff_expired BOOLEAN NOT NULL DEFAULT false,
    revision INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE restaurant_settings;
DROP TABLE sessions;