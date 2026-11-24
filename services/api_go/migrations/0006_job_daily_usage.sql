CREATE TABLE job_daily_usage (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    usage_date TEXT NOT NULL,
    created_count INTEGER NOT NULL CHECK (created_count > 0),
    PRIMARY KEY (user_id, usage_date)
);

INSERT INTO job_daily_usage (user_id, usage_date, created_count)
SELECT user_id, date(created_at / 1000.0, 'unixepoch', '+8 hours'), count(*)
FROM jobs
GROUP BY user_id, date(created_at / 1000.0, 'unixepoch', '+8 hours');
