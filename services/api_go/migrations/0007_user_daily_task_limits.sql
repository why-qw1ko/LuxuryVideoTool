CREATE TABLE user_daily_task_limits (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    daily_task_limit INTEGER CHECK (daily_task_limit BETWEEN 0 AND 100000),
    updated_at INTEGER NOT NULL,
    updated_by TEXT REFERENCES users(id) ON DELETE SET NULL
);
