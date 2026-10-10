-- +goose Up
ALTER TABLE product_categories
    ADD CONSTRAINT categories_name_nonempty
        CHECK (length(btrim(name)) > 0),
    ADD CONSTRAINT categories_revision_positive
        CHECK (revision > 0);

ALTER TABLE products
    ADD CONSTRAINT products_name_nonempty
        CHECK (length(btrim(name)) > 0),
    ADD CONSTRAINT products_shelf_life_positive
        CHECK (
            standard_shelf_life_hours IS NULL
            OR standard_shelf_life_hours > 0
        ),
    ADD CONSTRAINT products_target_nonnegative
        CHECK (
            target_stock IS NULL
            OR target_stock >= 0
        ),
    ADD CONSTRAINT products_target_pcs_integer
        CHECK (
            unit <> 'pcs'
            OR target_stock IS NULL
            OR target_stock = trunc(target_stock)
        ),
    ADD CONSTRAINT products_display_unit_compatible
        CHECK (
            (unit = 'g' AND display_unit IN ('g', 'kg'))
            OR (unit = 'ml' AND display_unit IN ('ml', 'l'))
            OR (unit = 'pcs' AND display_unit = 'pcs')
        ),
    ADD CONSTRAINT products_revision_positive
        CHECK (revision > 0);

ALTER TABLE supplies
    ADD CONSTRAINT supplies_number_trimmed
        CHECK (
            length(btrim(number)) > 0
            AND number = btrim(number)
        );

ALTER TABLE batches
    ADD CONSTRAINT batches_line_positive
        CHECK (line_number > 0),
    ADD CONSTRAINT batches_initial_positive
        CHECK (initial_quantity > 0),
    ADD CONSTRAINT batches_remaining_range
        CHECK (
            remaining_quantity >= 0
            AND remaining_quantity <= initial_quantity
        ),
    ADD CONSTRAINT batches_expiry_after_production
        CHECK (
            produced_at IS NULL
            OR expires_at > produced_at
        ),
    ADD CONSTRAINT batches_expiry_input_consistent
        CHECK (
            (
                expiry_source = 'explicit'
                AND shelf_life_hours IS NULL
            )
            OR (
                expiry_source = 'calculated'
                AND produced_at IS NOT NULL
                AND shelf_life_hours IS NOT NULL
                AND shelf_life_hours > 0
            )
        );

ALTER TABLE dishes
    ADD CONSTRAINT dishes_name_nonempty
        CHECK (length(btrim(name)) > 0),
    ADD CONSTRAINT dishes_revision_positive
        CHECK (revision > 0);

ALTER TABLE recipe_versions
    ADD CONSTRAINT recipe_version_positive
        CHECK (version > 0);

ALTER TABLE recipe_items
    ADD CONSTRAINT recipe_quantity_positive
        CHECK (quantity_per_portion > 0);

ALTER TABLE order_items
    ADD CONSTRAINT order_portions_range
        CHECK (portions BETWEEN 1 AND 1000);

ALTER TABLE stock_movements
    ADD CONSTRAINT movement_remaining_nonnegative
        CHECK (remaining_quantity_after >= 0),
    ADD CONSTRAINT movement_sign
        CHECK (
            (type = 'receipt' AND delta_quantity > 0)
            OR (type <> 'receipt' AND delta_quantity < 0)
        ),
    ADD CONSTRAINT movement_order_link
        CHECK (
            (
                type = 'order_consumption'
                AND order_item_id IS NOT NULL
            )
            OR (
                type <> 'order_consumption'
                AND order_item_id IS NULL
            )
        ),
    ADD CONSTRAINT movement_manual_reason
        CHECK (
            type <> 'manual_writeoff'
            OR (
                note IS NOT NULL
                AND length(btrim(note)) > 0
            )
        );

ALTER TABLE users
    ADD CONSTRAINT users_email_normalized
        CHECK (email = lower(btrim(email))),
    ADD CONSTRAINT users_name_nonempty
        CHECK (length(btrim(name)) > 0),
    ADD CONSTRAINT users_revision_positive
        CHECK (revision > 0);

ALTER TABLE sessions
    ADD CONSTRAINT session_expiry_future
        CHECK (expires_at > created_at);

ALTER TABLE restaurant_settings
    ADD CONSTRAINT settings_singleton
        CHECK (id = 1),
    ADD CONSTRAINT settings_name_nonempty
        CHECK (length(btrim(restaurant_name)) > 0),
    ADD CONSTRAINT settings_threshold_range
        CHECK (expiring_threshold_hours BETWEEN 1 AND 720),
    ADD CONSTRAINT settings_revision_positive
        CHECK (revision > 0);

ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_result_status
        CHECK (response_status BETWEEN 200 AND 499),
    ADD CONSTRAINT idempotency_expiry_future
        CHECK (expires_at > created_at);

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_publication_consistent
        CHECK (
            (
                cursor IS NULL
                AND published_at IS NULL
            )
            OR (
                cursor IS NOT NULL
                AND cursor > 0
                AND published_at IS NOT NULL
            )
        );

-- +goose Down
ALTER TABLE outbox_events
    DROP CONSTRAINT outbox_publication_consistent;

ALTER TABLE idempotency_keys
    DROP CONSTRAINT idempotency_expiry_future,
    DROP CONSTRAINT idempotency_result_status;

ALTER TABLE restaurant_settings
    DROP CONSTRAINT settings_revision_positive,
    DROP CONSTRAINT settings_threshold_range,
    DROP CONSTRAINT settings_name_nonempty,
    DROP CONSTRAINT settings_singleton;

ALTER TABLE sessions
    DROP CONSTRAINT session_expiry_future;

ALTER TABLE users
    DROP CONSTRAINT users_revision_positive,
    DROP CONSTRAINT users_name_nonempty,
    DROP CONSTRAINT users_email_normalized;

ALTER TABLE stock_movements
    DROP CONSTRAINT movement_manual_reason,
    DROP CONSTRAINT movement_order_link,
    DROP CONSTRAINT movement_sign,
    DROP CONSTRAINT movement_remaining_nonnegative;

ALTER TABLE order_items
    DROP CONSTRAINT order_portions_range;

ALTER TABLE recipe_items
    DROP CONSTRAINT recipe_quantity_positive;

ALTER TABLE recipe_versions
    DROP CONSTRAINT recipe_version_positive;

ALTER TABLE dishes
    DROP CONSTRAINT dishes_revision_positive,
    DROP CONSTRAINT dishes_name_nonempty;

ALTER TABLE batches
    DROP CONSTRAINT batches_expiry_input_consistent,
    DROP CONSTRAINT batches_expiry_after_production,
    DROP CONSTRAINT batches_remaining_range,
    DROP CONSTRAINT batches_initial_positive,
    DROP CONSTRAINT batches_line_positive;

ALTER TABLE supplies
    DROP CONSTRAINT supplies_number_trimmed;

ALTER TABLE products
    DROP CONSTRAINT products_revision_positive,
    DROP CONSTRAINT products_display_unit_compatible,
    DROP CONSTRAINT products_target_pcs_integer,
    DROP CONSTRAINT products_target_nonnegative,
    DROP CONSTRAINT products_shelf_life_positive,
    DROP CONSTRAINT products_name_nonempty;

ALTER TABLE product_categories
    DROP CONSTRAINT categories_revision_positive,
    DROP CONSTRAINT categories_name_nonempty;