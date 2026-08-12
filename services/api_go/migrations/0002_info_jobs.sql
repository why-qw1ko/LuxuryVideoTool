CREATE TABLE works (
    id TEXT PRIMARY KEY,
    douyin_work_id TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL CHECK (content_type IN ('video', 'note')),
    canonical_url TEXT NOT NULL,
    author_id TEXT,
    author_name TEXT,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    cover_url TEXT,
    published_at INTEGER,
    metadata_json TEXT NOT NULL,
    resolver_name TEXT NOT NULL,
    resolver_version TEXT NOT NULL,
    resolved_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    work_id TEXT REFERENCES works(id),
    input_text TEXT NOT NULL,
    input_url TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('info', 'download', 'transcribe', 'full')),
    status TEXT NOT NULL CHECK (status IN ('queued', 'resolving', 'downloading', 'extracting', 'transcribing', 'postprocessing', 'retry_wait', 'completed', 'failed', 'cancelled')),
    progress INTEGER NOT NULL CHECK (progress BETWEEN 0 AND 100),
    status_message TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    force_refresh INTEGER NOT NULL DEFAULT 0 CHECK (force_refresh IN (0, 1)),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    lease_owner TEXT,
    lease_expires_at INTEGER,
    heartbeat_at INTEGER,
    error_code TEXT,
    error_message TEXT,
    started_at INTEGER,
    completed_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(user_id, idempotency_key)
);

CREATE INDEX jobs_user_created_idx ON jobs(user_id, created_at DESC);
CREATE INDEX jobs_status_created_idx ON jobs(status, created_at);
CREATE INDEX jobs_lease_idx ON jobs(lease_expires_at);
CREATE INDEX works_douyin_id_idx ON works(douyin_work_id);
