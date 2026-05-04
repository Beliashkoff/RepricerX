DROP INDEX IF EXISTS idx_import_log_user_started;
DROP INDEX IF EXISTS ux_import_log_one_active_per_shop;
DROP INDEX IF EXISTS ux_import_log_job_id;

ALTER TABLE import_log DROP CONSTRAINT IF EXISTS import_log_status_check;

UPDATE import_log
SET status = 'completed'
WHERE status = 'succeeded';

UPDATE import_log
SET status = 'failed'
WHERE status IN ('pending', 'canceled');

DO $$
BEGIN
    ALTER TABLE import_log
        ADD CONSTRAINT import_log_status_check
        CHECK (status IN ('running', 'completed', 'failed', 'partial'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE import_log
    DROP COLUMN IF EXISTS requested_at,
    DROP COLUMN IF EXISTS user_id,
    DROP COLUMN IF EXISTS job_id;

DROP INDEX IF EXISTS idx_background_jobs_type_status_created;
DROP INDEX IF EXISTS idx_background_jobs_expired_locks;
DROP INDEX IF EXISTS idx_background_jobs_pick;

DROP TABLE IF EXISTS background_jobs;
DROP TYPE IF EXISTS background_job_status;
