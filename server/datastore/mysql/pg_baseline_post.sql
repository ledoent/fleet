-- Post-baseline fixups for PostgreSQL deployments.
--
-- Runs on every startup, idempotent. Skips objects already owned by the
-- connecting role, so it is a no-op when there is no work to do.
--
-- Required because earlier baseline loads ran as `postgres` (superuser),
-- leaving the application user unable to RENAME tables for atomic swaps
-- (used by host_counts cron) and unable to ALTER its own schema.

DO $$
DECLARE
    app_role text := current_user;
    obj record;
BEGIN
    FOR obj IN
        SELECT tablename FROM pg_tables
        WHERE schemaname = 'public' AND tableowner != app_role
    LOOP
        EXECUTE format('ALTER TABLE public.%I OWNER TO %I', obj.tablename, app_role);
    END LOOP;

    FOR obj IN
        SELECT sequencename FROM pg_sequences
        WHERE schemaname = 'public' AND sequenceowner != app_role
    LOOP
        EXECUTE format('ALTER SEQUENCE public.%I OWNER TO %I', obj.sequencename, app_role);
    END LOOP;

    FOR obj IN
        SELECT viewname FROM pg_views
        WHERE schemaname = 'public' AND viewowner != app_role
    LOOP
        EXECUTE format('ALTER VIEW public.%I OWNER TO %I', obj.viewname, app_role);
    END LOOP;
END $$;
