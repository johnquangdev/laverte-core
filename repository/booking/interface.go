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
	// There is deliberately no full-row write on this interface. Every writer below
	// races at least one other — the SePay webhook, the expiry sweep, the lock-code
	// cron, an admin acting on the same booking — and several of them hold their
	// in-memory copy across a multi-second Google Calendar or SePay call. A
	// read-modify-write from such a snapshot rewrites every column, so it silently
	// reverts whatever the other writer changed. Because 'pending_payment' and
	// 'confirmed' are the only two statuses the overlap exclusion constraint covers,
	// a reverted status either re-holds a slot for a stay everyone believes is
	// cancelled or releases one a guest has already paid for.
	MarkLockCodeAlertSent(ctx context.Context, id uint, at time.Time) error
	SetDoorLockCode(ctx context.Context, id uint, code string) error

	// SetCalendarEventID and SetPaymentID need no precondition: each is written once
	// per booking, by whoever created the thing being linked, and neither takes part
	// in the status transitions the guarded writes below protect.
	SetCalendarEventID(ctx context.Context, id uint, eventID string) error
	SetPaymentID(ctx context.Context, id uint, paymentID uint) error

	// ConfirmIfPending stamps status and payment_id in one statement, reporting
	// whether the row was still 'pending_payment'. Both columns move together
	// because a confirmed booking with no payment link is money nobody can
	// reconcile, and the status guard is what stops a webhook delivery from
	// confirming a hold that an admin cancelled or the sweep expired while the
	// provider call was in flight.
	ConfirmIfPending(ctx context.Context, id uint, paymentID uint) (bool, error)

	// CancelIfNotTerminal, CompleteIfConfirmed and NoShowIfConfirmed re-assert in SQL
	// the precondition the usecase checked against a row it read moments earlier.
	// False means another writer got there first; the caller must turn that into a
	// refusal, because an admin told the action succeeded while the row went the
	// other way has no way to discover it.
	CancelIfNotTerminal(ctx context.Context, id uint) (bool, error)
	CompleteIfConfirmed(ctx context.Context, id uint) (bool, error)
	NoShowIfConfirmed(ctx context.Context, id uint) (bool, error)

	// ReleaseHoldIfPending is the same transition as ExpireIfPending, named for its
	// caller: a create that failed after its booking row committed and is handing
	// the slot back. The guard matters because the payment it gave up on may have
	// landed in the meantime.
	ReleaseHoldIfPending(ctx context.Context, id uint) (bool, error)

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
