package booking

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
)

// IUseCase grows across later tasks (guest lookup, admin walk-in, cancel,
// lock-code) — add methods here instead of introducing a parallel interface.
type IUseCase interface {
	Create(ctx context.Context, req payload.CreateBookingRequest) (*presenter.BookingResponse, error)
	// Availability reports the windows in [from, to) that are already taken, so a
	// guest can pick a slot before hitting the exclusion constraint.
	Availability(ctx context.Context, homeID uint, from, to time.Time) (*presenter.AvailabilityResponse, error)
}
