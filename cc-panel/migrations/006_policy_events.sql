CREATE TABLE IF NOT EXISTS auto_policy_events (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES auto_policies(id) ON DELETE CASCADE,
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    metric VARCHAR(50) NOT NULL,
    observed_value DOUBLE PRECISION NOT NULL,
    threshold INT NOT NULL,
    action VARCHAR(50) NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auto_policy_events_created ON auto_policy_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_auto_policy_events_policy ON auto_policy_events(policy_id);
