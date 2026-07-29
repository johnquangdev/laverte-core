-- +migrate Up
CREATE TABLE homes (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    -- Pricing resolves by category (see usecase/pricing), so a value outside these
    -- two would produce a home that cannot be priced. The usecase checks it too;
    -- this is the backstop for any future writer that bypasses the usecase.
    category TEXT NOT NULL CHECK (category IN ('home', 'nest')),
    address TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    google_calendar_id TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_homes_category ON homes(category);

-- +migrate Down
DROP TABLE homes;
