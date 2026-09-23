-- +migrate Up
-- A refund is recorded, not executed: the money goes back by hand through the
-- bank, and this row is the ledger's memory that it did. Revenue sums only
-- status 'paid', so flipping to 'refunded' is what takes it out of the overview.
ALTER TABLE payments
    ADD COLUMN refunded_at TIMESTAMPTZ,
    ADD COLUMN refunded_by_admin_id BIGINT REFERENCES users(id),
    ADD COLUMN refund_note TEXT NOT NULL DEFAULT '';

-- Money that reached the bank account but could not settle any booking. The
-- webhook has nowhere else to put it: payments.booking_id is NOT NULL, and a
-- transfer with an unreadable memo or a wrong amount has no booking to attach to.
CREATE TABLE unmatched_transfers (
    id BIGSERIAL PRIMARY KEY,
    -- The provider redelivers the same transfer until it gets a 2xx; keying on its
    -- stable transaction id is what keeps one transfer from becoming many rows
    -- and many admin alerts.
    sepay_transaction_ref TEXT NOT NULL UNIQUE,
    amount BIGINT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL CHECK (reason IN
        ('no_memo', 'booking_not_found', 'booking_not_pending', 'amount_mismatch')),
    -- Deliberately not a foreign key: for 'booking_not_found' this is an id the
    -- memo named that does not exist.
    booking_ref BIGINT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolved_by_admin_id BIGINT REFERENCES users(id),
    resolution_note TEXT NOT NULL DEFAULT '',
    CONSTRAINT unmatched_resolution_complete CHECK (
        (resolved_at IS NULL) = (resolved_by_admin_id IS NULL))
);
CREATE INDEX idx_unmatched_transfers_open ON unmatched_transfers(received_at DESC)
    WHERE resolved_at IS NULL;

-- +migrate Down
DROP TABLE unmatched_transfers;
ALTER TABLE payments
    DROP COLUMN refund_note,
    DROP COLUMN refunded_by_admin_id,
    DROP COLUMN refunded_at;
