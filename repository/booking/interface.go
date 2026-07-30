package booking

import (
	"context"
	"errors"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

// ErrSlotConflict is returned by Create when the DB exclusion constraint
// rejects an overlapping [start_time, end_time) for the same home_id.
var ErrSlotConflict = errors.New("booking: slot conflict")

type IRepository interface {
	Create(ctx context.Context, b *model.Booking) error
	GetByID(ctx context.Context, id uint) (*model.Booking, error)
	Update(ctx context.Context, b *model.Booking) error
	GetPendingByPhone(ctx context.Context, phone string) (*model.Booking, error)
	ListExpiredPending(ctx context.Context, now time.Time) ([]*model.Booking, error)
	ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]*model.Booking, error)
	// ListUpcomingMissingLockCode returns confirmed bookings whose start_time
	// is within [now, now+leadTime) and that have no door_lock_code yet and
	// haven't been alerted on.
	ListUpcomingMissingLockCode(ctx context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error)
	// ListReadyToSendLockCode returns confirmed bookings whose start_time has
	// already passed, that have a door_lock_code set, and haven't been sent yet.
	ListReadyToSendLockCode(ctx context.Context, now time.Time) ([]*model.Booking, error)
	// These write one column each, unlike Update's Save() which rewrites every column
	// from an in-memory snapshot. The lock-code sweeps run on a timer while an admin
	// may be acting on the same booking; a read-modify-Save from either side would
	// silently discard the other's change.
	MarkLockCodeAlertSent(ctx context.Context, id uint, at time.Time) error
	SetDoorLockCode(ctx context.Context, id uint, code string) error

	// ExpireIfPending flips one booking to 'expired' only if it is still
	// 'pending_payment', reporting whether it won. The expiry sweep reads a whole
	// batch up front and writes each row a moment later; a full-row Save from that
	// snapshot would clobber a booking the SePay webhook confirmed in between —
	// reverting Status and PaymentID, and since 'expired' sits outside the overlap
	// exclusion constraint, reopening a paid guest's slot to a stranger.
	ExpireIfPending(ctx context.Context, id uint) (bool, error)

	// ClaimLockCodeSend stamps lock_code_sent_at only if it is still NULL, reporting
	// whether this caller won. A door code is a physical-access credential, so
	// read-check-send-then-mark is not enough: two in-flight sends (a double-clicked
	// button, or two cron instances) would both see NULL and both deliver it. The
	// claim also requires status 'confirmed', so a booking cancelled after the caller
	// read it cannot still receive its code. The caller sends only when this returns
	// true, and calls ReleaseLockCodeSend if the send then fails, so a retry can
	// still deliver.
	ClaimLockCodeSend(ctx context.Context, id uint, at time.Time) (bool, error)
	ReleaseLockCodeSend(ctx context.Context, id uint) error

	// CountConfirmedBetween counts bookings that actually occupy the property —
	// confirmed plus completed — whose start_time falls in [from, to).
	CountConfirmedBetween(ctx context.Context, from, to time.Time) (int64, error)
}
