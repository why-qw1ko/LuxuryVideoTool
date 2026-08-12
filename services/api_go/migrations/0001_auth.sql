CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username_normalized TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_login_at INTEGER
);

CREATE TABLE refresh_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    device_id TEXT NOT NULL,
    device_name TEXT NOT NULL,
    platform TEXT NOT NULL CHECK (platform IN ('android', 'windows')),
    app_version TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER,
    replaced_by TEXT REFERENCES refresh_tokens(id),
    created_at INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL
);

CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT,
    request_id TEXT NOT NULL,
    ip_hash TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);

CREATE INDEX refresh_tokens_user_active_idx
    ON refresh_tokens(user_id, revoked_at, expires_at);
CREATE INDEX audit_logs_created_idx ON audit_logs(created_at);

