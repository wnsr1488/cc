CREATE TABLE IF NOT EXISTS geo_cidrs (
    id BIGSERIAL PRIMARY KEY,
    country VARCHAR(100) NOT NULL,
    province VARCHAR(100),
    city VARCHAR(100),
    cidr VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geo_cidrs_country ON geo_cidrs(country);
CREATE INDEX IF NOT EXISTS idx_geo_cidrs_province ON geo_cidrs(province);
CREATE INDEX IF NOT EXISTS idx_geo_cidrs_city ON geo_cidrs(city);

CREATE TABLE IF NOT EXISTS geo_block_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    country VARCHAR(100) NOT NULL,
    province VARCHAR(100),
    city VARCHAR(100),
    action VARCHAR(20) NOT NULL DEFAULT 'DROP',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geo_block_rules_enabled ON geo_block_rules(enabled);
