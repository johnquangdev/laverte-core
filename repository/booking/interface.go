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
}
