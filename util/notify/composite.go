package notify

import (
	"context"

	"github.com/johnquangdev/laverte-core/model"
)

type composite struct {
	customer INotifier
	admin    INotifier
}

// NewComposite hides the two-channel split behind one INotifier so the billing
// usecase never learns that customers go over ZNS and admins over email.
func NewComposite(customer, admin INotifier) INotifier {
	return &composite{customer: customer, admin: admin}
}

func (c *composite) BookingConfirmed(ctx context.Context, b *model.Booking) error {
	return c.customer.BookingConfirmed(ctx, b)
}

func (c *composite) LockCode(ctx context.Context, b *model.Booking, code string) error {
	return c.customer.LockCode(ctx, b, code)
}

func (c *composite) AdminLockCodeMissing(ctx context.Context, b *model.Booking) error {
	return c.admin.AdminLockCodeMissing(ctx, b)
}
