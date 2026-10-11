-- +goose Up
ALTER TABLE outbox_events ADD CONSTRAINT outbox_publication_pair
    CHECK ((cursor IS NULL AND published_at IS NULL) OR (cursor IS NOT NULL AND cursor > 0 AND published_at IS NOT NULL));
CREATE INDEX outbox_pending_idx ON outbox_events(created_at,id) WHERE published_at IS NULL;
CREATE INDEX outbox_retention_idx ON outbox_events(published_at,cursor) WHERE published_at IS NOT NULL;

-- Cursor state survives deletion of every retained event and sequence gaps.
CREATE TABLE outbox_publication_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    high_water BIGINT NOT NULL DEFAULT 0 CHECK (high_water >= 0),
    retention_floor BIGINT NOT NULL DEFAULT 0 CHECK (retention_floor >= 0 AND retention_floor <= high_water),
    last_time_event_at TIMESTAMPTZ
);
INSERT INTO outbox_publication_state(id,high_water)
    SELECT 1,COALESCE(MAX(cursor),0) FROM outbox_events WHERE published_at IS NOT NULL;

-- +goose Down
DROP TABLE outbox_publication_state;
DROP INDEX outbox_retention_idx;
DROP INDEX outbox_pending_idx;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_publication_pair;
