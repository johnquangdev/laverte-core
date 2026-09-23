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
	// AdminUnmatchedTransfer tells the admin that money reached the account but
	// settled no booking. Called once per transfer, on first record.
	AdminUnmatchedTransfer(ctx context.Context, t *model.UnmatchedTransfer) error
}
