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
    EXCLUDE USING gist (
        home_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status IN ('pending_payment', 'confirmed'))
);
CREATE INDEX idx_bookings_home_id ON bookings(home_id);
CREATE INDEX idx_bookings_customer_phone ON bookings(customer_phone);
CREATE INDEX idx_bookings_status ON bookings(status);

-- +migrate Down
DROP TABLE bookings;
