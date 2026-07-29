-- +migrate Up
CREATE TABLE bookings (
    id BIGSERIAL PRIMARY KEY,
    home_id BIGINT NOT NULL REFERENCES homes(id),
    customer_name TEXT NOT NULL,
    customer_phone TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    booking_type TEXT NOT NULL,
    computed_price BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_payment',
    payment_id BIGINT,
    google_calendar_event_id TEXT NOT NULL DEFAULT '',
    door_lock_code TEXT,
    lock_code_alert_sent_at TIMESTAMPTZ,
    lock_code_sent_at TIMESTAMPTZ,
    created_by_admin_id BIGINT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- tstzrange() itself only rejects end < start, and it rejects it with a raw
    -- driver error the usecase cannot map. start = end is worse: it builds an
    -- EMPTY range, which overlaps nothing, so the exclusion constraint below
    -- would give a zero-duration booking no protection at all.
    CONSTRAINT bookings_time_order CHECK (end_time > start_time),
    -- Same defence-in-depth stance as homes.category: the usecase validates
    -- these, this is the backstop for any writer that bypasses it.
    CONSTRAINT bookings_status_valid CHECK (status IN
        ('pending_payment', 'confirmed', 'cancelled', 'expired', 'completed', 'no_show')),
    CONSTRAINT bookings_type_valid CHECK (booking_type IN ('hourly', 'overnight', 'day')),
    -- GetPendingByPhone is the anti-spam gate and filters on expires_at > now().
    -- SQL treats NULL > now() as unknown, so a pending row with no expiry would be
    -- invisible to that check and silently defeat it.
    CONSTRAINT bookings_pending_has_expiry CHECK (
        status <> 'pending_payment' OR expires_at IS NOT NULL),
    EXCLUDE USING gist (
        home_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status IN ('pending_payment', 'confirmed'))
);
CREATE INDEX idx_bookings_home_id ON bookings(home_id);
CREATE INDEX idx_bookings_customer_phone ON bookings(customer_phone);
CREATE INDEX idx_bookings_status ON bookings(status);
-- The expiry sweep runs every minute over pending_payment rows; status alone
-- leaves expires_at as a residual filter.
CREATE INDEX idx_bookings_status_expires_at ON bookings(status, expires_at);

-- +migrate Down
DROP TABLE bookings;
