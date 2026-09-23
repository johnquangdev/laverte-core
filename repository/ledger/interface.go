// Package ledger holds the admin-facing reads and corrections over payments.
// It is separate from repository/payment because the settlement path never
// needs any of it, and every fake of that interface would otherwise grow these
// methods too.
package ledger

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

// PaymentRow is a payment plus the booking fields an admin needs to recognise
// it. BookingStatus is what decides whether a paid row may be refunded.
type PaymentRow struct {
	model.Payment
	CustomerName  string
	CustomerPhone string
	HomeID        uint
	BookingStatus string
}

type IRepository interface {
	// ListPayments returns payments created in [from, to), newest first, at most
	// limit rows.
	ListPayments(ctx context.Context, from, to time.Time, limit int) ([]*PaymentRow, error)
	GetPayment(ctx context.Context, id uint) (*PaymentRow, error)
	// RefundIfStayEnded flips a payment to refunded only while it is 'paid' AND its
	// booking is cancelled or expired, reporting whether it won. Both conditions sit
	// in the UPDATE itself: checked in Go first, a booking completed or a refund
	// recorded by a second admin in between would still be overwritten.
	RefundIfStayEnded(ctx context.Context, paymentID, adminID uint, note string, at time.Time) (bool, error)
}
