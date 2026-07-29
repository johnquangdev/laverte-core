-- +migrate Up
CREATE TABLE blocked_slots (
    id BIGSERIAL PRIMARY KEY,
    home_id BIGINT NOT NULL REFERENCES homes(id),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by_admin_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Same defence-in-depth stance as bookings_time_order: the usecase already
    -- rejects end <= start, this is the backstop for any writer that bypasses it.
    CONSTRAINT blocked_slots_time_order CHECK (end_time > start_time)
);
-- Deliberately no EXCLUDE constraint, unlike bookings: admins legitimately
-- stack overlapping blocks (a maintenance window nested inside a longer
-- seasonal close), and readers only ever ask whether ANY block overlaps.
CREATE INDEX idx_blocked_slots_home_id ON blocked_slots(home_id);

-- +migrate Down
DROP TABLE blocked_slots;
