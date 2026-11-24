CREATE TABLE asr_budget_usage (
    usage_date TEXT PRIMARY KEY NOT NULL,
    estimated_cost_cny REAL NOT NULL
);

INSERT INTO asr_budget_usage (usage_date, estimated_cost_cny)
SELECT date(started_at / 1000.0, 'unixepoch'), sum(estimated_cost_cny)
FROM asr_calls
GROUP BY date(started_at / 1000.0, 'unixepoch');

-- Record costs in the transaction that writes each call. Deleting the call or
-- its task leaves the aggregate in place for budget enforcement.
CREATE TRIGGER asr_budget_record_call AFTER INSERT ON asr_calls
BEGIN
    INSERT INTO asr_budget_usage (usage_date, estimated_cost_cny)
    VALUES (date(NEW.started_at / 1000.0, 'unixepoch'), NEW.estimated_cost_cny)
    ON CONFLICT (usage_date) DO UPDATE SET
        estimated_cost_cny = asr_budget_usage.estimated_cost_cny + excluded.estimated_cost_cny;
END;

CREATE TRIGGER asr_budget_adjust_call AFTER UPDATE OF estimated_cost_cny ON asr_calls
WHEN NEW.estimated_cost_cny <> OLD.estimated_cost_cny
BEGIN
    UPDATE asr_budget_usage
    SET estimated_cost_cny = estimated_cost_cny + (NEW.estimated_cost_cny - OLD.estimated_cost_cny)
    WHERE usage_date = date(OLD.started_at / 1000.0, 'unixepoch');
END;
