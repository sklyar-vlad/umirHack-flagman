-- +goose Up
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID,
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID,
    request_id VARCHAR(128),
    before_data JSONB,
    after_data JSONB,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT audit_log_actor_fk
        FOREIGN KEY (actor_id)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    endpoint VARCHAR(200) NOT NULL,
    key UUID NOT NULL,
    request_hash VARCHAR(128) NOT NULL,
    response_status INTEGER NOT NULL,
    response_body JSONB NOT NULL,
    response_headers JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT idempotency_keys_user_fk
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE RESTRICT,

    CONSTRAINT idempotency_keys_scope_unique
        UNIQUE (user_id, endpoint, key)
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cursor BIGINT UNIQUE,
    event_type VARCHAR(100) NOT NULL,
    actor_id UUID,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,

    CONSTRAINT outbox_events_actor_fk
        FOREIGN KEY (actor_id)
        REFERENCES users (id)
        ON DELETE RESTRICT
);

-- +goose Down
DROP SEQUENCE outbox_cursor_seq;

DROP TABLE outbox_events;
DROP TABLE idempotency_keys;
DROP TABLE audit_log;