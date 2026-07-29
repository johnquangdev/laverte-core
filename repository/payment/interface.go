package payment

import (
	"context"
	"errors"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

// ErrDuplicateExternalRef means the provider's transaction id is already recorded
// against a different payment. The partial unique index on sepay_transaction_ref
// enforces that; without a sentinel the violation reaches the webhook handler as a
// raw driver error, it answers 500, and SePay retries a settlement that can never
// succeed.
var ErrDuplicateExternalRef = errors.New("payment: external ref already claimed by another payment")

type IRepository interface {
	Create(ctx context.Context, p *model.Payment) error
	GetByID(ctx context.Context, id uint) (*model.Payment, error)
	GetByBookingID(ctx context.Context, bookingID uint) (*model.Payment, error)
	// MarkPaidIfPending settles a payment exactly once: it reports (false, nil) when
	// the row was no longer pending — the normal outcome for a redelivered webhook,
	// not a failure — and (false, ErrDuplicateExternalRef) when externalRef is
	// already recorded on a different payment, which the caller must acknowledge
	// and never retry.
	MarkPaidIfPending(ctx context.Context, paymentID uint, externalRef string, paidAt time.Time) (bool, error)
	// SumPaidBetween totals paid amounts over [from, to).
	SumPaidBetween(ctx context.Context, from, to time.Time) (int64, error)
}
