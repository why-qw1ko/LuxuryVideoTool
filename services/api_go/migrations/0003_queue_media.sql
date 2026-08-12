CREATE TABLE job_steps (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    step_name TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    details_json TEXT NOT NULL DEFAULT '{}',
    error_code TEXT,
    error_message TEXT
);

CREATE TABLE files (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    relative_path TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256 TEXT NOT NULL,
    expires_at INTEGER,
    created_at INTEGER NOT NULL,
    deleted_at INTEGER
);

ALTER TABLE jobs ADD COLUMN retry_at INTEGER;
ALTER TABLE jobs ADD COLUMN cancel_requested_at INTEGER;

CREATE INDEX job_steps_job_idx ON job_steps(job_id, started_at);
CREATE INDEX files_job_kind_idx ON files(job_id, kind);
CREATE INDEX files_expiry_idx ON files(expires_at, deleted_at);
