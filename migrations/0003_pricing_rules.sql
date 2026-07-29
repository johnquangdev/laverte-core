-- +migrate Up
CREATE TABLE pricing_rules (
    id BIGSERIAL PRIMARY KEY,
    category TEXT NOT NULL,
    rule_type TEXT NOT NULL,
    base_hours INT,
    base_price BIGINT,
    extra_hour_price BIGINT,
    window_start TEXT,
    window_end TEXT,
    flat_price BIGINT,
    effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_to TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pricing_rules_category_type ON pricing_rules(category, rule_type);

-- +migrate Down
DROP TABLE pricing_rules;
