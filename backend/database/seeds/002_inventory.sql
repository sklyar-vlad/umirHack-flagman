BEGIN;

SELECT pg_advisory_xact_lock(40401);

SET LOCAL TIME ZONE 'UTC';

CREATE TEMP TABLE seed_clock ON COMMIT DROP AS
SELECT COALESCE(
    (
        SELECT received_at
        FROM supplies
        WHERE id = '50000000-0000-4000-8000-000000000002'
    ),
    clock_timestamp()
) AS at_time;


INSERT INTO supplies (
    id,
    number,
    received_at,
    created_at,
    created_by
)
SELECT
    '50000000-0000-4000-8000-000000000001'::uuid,
    '104',
    at_time - interval '54 hours',
    at_time - interval '54 hours',
    '90000000-0000-4000-8000-000000000001'::uuid
FROM seed_clock
ON CONFLICT (id) DO NOTHING;

INSERT INTO supplies (
    id,
    number,
    received_at,
    created_at,
    created_by
)
SELECT
    '50000000-0000-4000-8000-000000000002'::uuid,
    '105',
    at_time,
    at_time,
    '90000000-0000-4000-8000-000000000001'::uuid
FROM seed_clock
ON CONFLICT (id) DO NOTHING;

WITH fixture (
    id,
    supply_id,
    product_id,
    line_number,
    quantity,
    produced_hours_ago,
    shelf_life_hours
) AS (
    VALUES
        (
            '60000000-0000-4000-8000-000000000001'::uuid,
            '50000000-0000-4000-8000-000000000001'::uuid,
            '20000000-0000-4000-8000-000000000001'::uuid,
            1,
            1800.000,
            54,
            72
        ),

        (
            '60000000-0000-4000-8000-000000000002',
            '50000000-0000-4000-8000-000000000001',
            '20000000-0000-4000-8000-000000000005',
            2,
            1000.000,
            78,
            72
        ),

        (
            '60000000-0000-4000-8000-000000000003',
            '50000000-0000-4000-8000-000000000002',
            '20000000-0000-4000-8000-000000000001',
            1,
            2200.000,
            0,
            72
        ),

        (
            '60000000-0000-4000-8000-000000000004',
            '50000000-0000-4000-8000-000000000002',
            '20000000-0000-4000-8000-000000000002',
            2,
            3000.000,
            0,
            168
        ),

        (
            '60000000-0000-4000-8000-000000000005',
            '50000000-0000-4000-8000-000000000002',
            '20000000-0000-4000-8000-000000000003',
            3,
            20.000,
            0,
            48
        ),

        (
            '60000000-0000-4000-8000-000000000006',
            '50000000-0000-4000-8000-000000000002',
            '20000000-0000-4000-8000-000000000006',
            4,
            1500.000,
            0,
            72
        ),

        (
            '60000000-0000-4000-8000-000000000007',
            '50000000-0000-4000-8000-000000000002',
            '20000000-0000-4000-8000-000000000007',
            5,
            2000.000,
            0,
            8760
        )
),
new_batches AS (
    INSERT INTO batches (
        id,
        supply_id,
        product_id,
        line_number,
        initial_quantity,
        remaining_quantity,
        produced_at,
        shelf_life_hours,
        expiry_source,
        expires_at,
        created_at
    )
    SELECT
        f.id,
        f.supply_id,
        f.product_id,
        f.line_number,
        f.quantity,
        f.quantity,
        c.at_time - f.produced_hours_ago * interval '1 hour',
        f.shelf_life_hours,
        'calculated',
        c.at_time
            - f.produced_hours_ago * interval '1 hour'
            + f.shelf_life_hours * interval '1 hour',
        s.received_at
    FROM fixture AS f
    JOIN supplies AS s ON s.id = f.supply_id
    CROSS JOIN seed_clock AS c
    ON CONFLICT (id) DO NOTHING
    RETURNING id, supply_id, product_id, initial_quantity
)
INSERT INTO stock_movements (
    batch_id,
    order_item_id,
    type,
    delta_quantity,
    remaining_quantity_after,
    product_name_snapshot,
    unit_snapshot,
    actor_id,
    occurred_at,
    note
)
SELECT
    b.id,
    NULL,
    'receipt',
    b.initial_quantity,
    b.initial_quantity,
    p.name,
    p.unit,
    s.created_by,
    s.received_at,
    'Демо-фикстура S04-T02'
FROM new_batches AS b
JOIN products AS p ON p.id = b.product_id
JOIN supplies AS s ON s.id = b.supply_id;

COMMIT;