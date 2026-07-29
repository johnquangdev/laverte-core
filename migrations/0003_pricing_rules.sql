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
-- Compute takes the first matching active rule, so two active rules for one
-- category+type would make a customer's quote depend on row order. Superseded
-- rows have is_active=false and stay as price history, so they are exempt.
CREATE UNIQUE INDEX idx_pricing_rules_one_active
    ON pricing_rules(category, rule_type) WHERE is_active;

-- +migrate Down
DROP TABLE pricing_rules;
