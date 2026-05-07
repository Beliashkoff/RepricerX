DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'import_log'
          AND column_name = 'started_at'
          AND data_type = 'timestamp without time zone'
    ) THEN
        ALTER TABLE import_log
            ALTER COLUMN started_at TYPE TIMESTAMPTZ USING started_at AT TIME ZONE 'UTC';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'import_log'
          AND column_name = 'finished_at'
          AND data_type = 'timestamp without time zone'
    ) THEN
        ALTER TABLE import_log
            ALTER COLUMN finished_at TYPE TIMESTAMPTZ USING finished_at AT TIME ZONE 'UTC';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'import_log'
          AND column_name = 'requested_at'
          AND data_type = 'timestamp without time zone'
    ) THEN
        ALTER TABLE import_log
            ALTER COLUMN requested_at TYPE TIMESTAMPTZ USING requested_at AT TIME ZONE 'UTC';
    END IF;
END $$;
