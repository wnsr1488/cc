CREATE TABLE IF NOT EXISTS firewall_snapshots (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    iptables_rules TEXT NOT NULL,
    ipset_rules TEXT NOT NULL,
    reason VARCHAR(100) NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_firewall_snapshots_server_created ON firewall_snapshots(server_id, created_at DESC);
