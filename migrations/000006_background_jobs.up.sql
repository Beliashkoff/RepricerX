DO $$
BEGIN
    CREATE TYPE background_job_status AS ENUM (
        'pending',
        'running',
        'succeeded',
        'failed',
        'canceled',
        'retrying'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS background_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL,
    status background_job_status NOT NULL DEFAULT 'pending',
    queue TEXT NOT NULL DEFAULT 'default',
    priority INT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ NULL,
    locked_by TEXT NULL,
    lock_expires_at TIMESTAMPTZ NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    canceled_at TIMESTAMPTZ NULL,
    CONSTRAINT background_jobs_attempts_check CHECK (
        attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts
    ),
    CONSTRAINT background_jobs_finished_check CHECK (
        (
            status IN ('succeeded', 'failed', 'canceled')
            AND finished_at IS NOT NULL
        )
        OR
        (
            status IN ('pending', 'running', 'retrying')
            AND finished_at IS NULL
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_background_jobs_pick
    ON background_jobs (queue, status, run_at, priority DESC, created_at)
    WHERE status IN ('pending', 'retrying');

CREATE INDEX IF NOT EXISTS idx_background_jobs_expired_locks
    ON background_jobs (lock_expires_at)
    WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_background_jobs_type_status_created
    ON background_jobs (job_type, status, created_at DESC);

ALTER TABLE import_log DROP CONSTRAINT IF EXISTS import_log_status_check;

ALTER TABLE import_log
    ADD COLUMN IF NOT EXISTS job_id UUID NULL REFERENCES background_jobs(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE import_log il
SET user_id = s.user_id
FROM shops s
WHERE il.shop_id = s.id
  AND il.user_id IS NULL;

UPDATE import_log
SET status = 'succeeded'
WHERE status = 'completed';

UPDATE import_log
SET status = 'failed',
    finished_at = COALESCE(finished_at, NOW()),
    errors = errors || '[{"code":"worker_migration_interrupted","message":"Import was interrupted before durable workers were enabled"}]'::jsonb
WHERE status = 'running'
  AND job_id IS NULL;

DO $$
BEGIN
    ALTER TABLE import_log
        ADD CONSTRAINT import_log_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'partial', 'failed', 'canceled'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS ux_import_log_job_id
    ON import_log(job_id)
    WHERE job_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_import_log_one_active_per_shop
    ON import_log(shop_id)
    WHERE status IN ('pending', 'running');

CREATE INDEX IF NOT EXISTS idx_import_log_user_started
    ON import_log(user_id, started_at DESC);
