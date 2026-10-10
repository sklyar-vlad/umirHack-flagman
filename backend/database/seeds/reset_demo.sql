BEGIN;

SET LOCAL lock_timeout = '5s';

DO $$
BEGIN
    IF current_database() <> 'postgres_demo' THEN
        RAISE EXCEPTION
            'Demo reset is forbidden for database %',
            current_database();
    END IF;
END;
$$;

SELECT pg_advisory_xact_lock(40401);

DROP SCHEMA public CASCADE;

CREATE SCHEMA public AUTHORIZATION pg_database_owner;
GRANT USAGE ON SCHEMA public TO PUBLIC;

COMMIT;