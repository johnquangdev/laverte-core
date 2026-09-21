package notify

import (
	"context"

	"github.com/johnquangdev/laverte-core/model"
)

// INotifier sends guest- and admin-facing booking notifications.
type INotifier interface {
	BookingConfirmed(ctx context.Context, b *model.Booking) error
	LockCode(ctx context.Context, b *model.Booking, code string) error
	AdminLockCodeMissing(ctx context.Context, b *model.Booking) error
}
