CREATE TABLE IF NOT EXISTS geo_default_whitelist_targets (
    server_id BIGINT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS geo_cidr_sync_state (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_pull_at TIMESTAMPTZ,
    last_deploy_at TIMESTAMPTZ,
    last_changed BOOLEAN NOT NULL DEFAULT FALSE,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO geo_cidr_sync_state (id) VALUES (1) ON CONFLICT DO NOTHING;
