ALTER TABLE users ADD COLUMN is_cdk INTEGER NOT NULL DEFAULT 0 CHECK (is_cdk IN (0, 1));

CREATE TABLE login_cdks (
    id TEXT PRIMARY KEY,
    code_hash TEXT NOT NULL UNIQUE,
    prefix TEXT NOT NULL,
    task_limit INTEGER NOT NULL CHECK (task_limit BETWEEN 1 AND 100000),
    task_count INTEGER NOT NULL DEFAULT 0 CHECK (task_count >= 0),
    created_at INTEGER NOT NULL,
    redeemed_at INTEGER,
    revoked_at INTEGER,
    user_id TEXT UNIQUE REFERENCES users(id) DEFERRABLE INITIALLY DEFERRED,
    CHECK ((redeemed_at IS NULL) = (user_id IS NULL))
);

CREATE INDEX login_cdks_created_idx ON login_cdks(created_at DESC, id DESC);
