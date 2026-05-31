CREATE TABLE IF NOT EXISTS auto_policies (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    metric VARCHAR(50) NOT NULL,
    threshold INT NOT NULL CHECK (threshold > 0),
    window_seconds INT NOT NULL CHECK (window_seconds > 0),
    block_seconds INT NOT NULL CHECK (block_seconds > 0),
    target_set VARCHAR(100) NOT NULL DEFAULT 'cc_rate_block',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auto_policies_enabled ON auto_policies(enabled);
