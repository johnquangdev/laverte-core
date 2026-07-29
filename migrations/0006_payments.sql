-- +migrate Up
CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES bookings(id),
    provider TEXT NOT NULL,
    amount BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    qr_content TEXT NOT NULL DEFAULT '',
    sepay_transaction_ref TEXT NOT NULL DEFAULT '',
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_booking_id ON payments(booking_id);
CREATE INDEX idx_payments_status ON payments(status);
-- A redelivered webhook carries the same SePay transaction id, so this makes a
-- second settlement of the same bank transfer fail at the DB even if it lands
-- on a different payment row. Rows no webhook has touched keep '' and are
-- excluded, since many of them coexist legitimately.
CREATE UNIQUE INDEX uq_payments_sepay_transaction_ref
    ON payments(sepay_transaction_ref) WHERE sepay_transaction_ref <> '';

-- +migrate Down
DROP TABLE payments;
