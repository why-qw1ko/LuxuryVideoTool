CREATE TABLE asr_calls (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    provider_request_id TEXT,
    provider_task_id TEXT,
    segment_index INTEGER NOT NULL,
    audio_seconds REAL NOT NULL,
    billed_seconds REAL,
    estimated_cost_cny REAL NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('submitted', 'running', 'completed', 'failed')),
    response_summary_json TEXT NOT NULL DEFAULT '{}',
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    error_code TEXT
);

ALTER TABLE jobs ADD COLUMN result_json TEXT;
ALTER TABLE jobs ADD COLUMN options_json TEXT NOT NULL DEFAULT '{}';

CREATE INDEX asr_calls_job_segment_idx ON asr_calls(job_id, segment_index);
CREATE INDEX asr_calls_provider_task_idx ON asr_calls(provider, provider_task_id);
CREATE INDEX asr_calls_started_idx ON asr_calls(started_at, status);
