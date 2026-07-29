package payment

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, p *model.Payment) error
	GetByID(ctx context.Context, id uint) (*model.Payment, error)
	GetByBookingID(ctx context.Context, bookingID uint) (*model.Payment, error)
	// MarkPaidIfPending settles a payment exactly once: it reports false (with
	// a nil error) when the row was no longer pending, which is the normal
	// outcome for a redelivered webhook and must not be treated as a failure.
	MarkPaidIfPending(ctx context.Context, paymentID uint, externalRef string, paidAt time.Time) (bool, error)
	// SumPaidBetween totals paid amounts over [from, to).
	SumPaidBetween(ctx context.Context, from, to time.Time) (int64, error)
}
