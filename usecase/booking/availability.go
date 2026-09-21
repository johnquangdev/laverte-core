package booking

import (
	"context"
	"sort"
	"time"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/presenter"
	pricinguc "github.com/johnquangdev/laverte-core/usecase/pricing"
)

// DefaultAvailabilityWindow is how far ahead an availability request without an
// explicit `to` looks.
const DefaultAvailabilityWindow = 7 * 24 * time.Hour

func (uc *UseCase) Availability(ctx context.Context, homeID uint, from, to time.Time) (*presenter.AvailabilityResponse, error) {
	if !to.After(from) {
		return nil, apperr.Validation("to phai sau from")
	}
	// The same cap Create enforces on a booking range: this endpoint is public and
	// unauthenticated, and a multi-year window is a full-table scan per request.
	if err := pricinguc.ValidateDuration(from, to); err != nil {
		return nil, err
	}

	home, err := uc.homeRepo.GetByID(ctx, homeID)
	if err != nil {
		return nil, apperr.NotFound(err)
	}
	if !home.IsActive {
		return nil, apperr.Validation("home dang tam ngung nhan khach")
	}

	bookings, err := uc.bookingRepo.ListOccupyingBetween(ctx, homeID, from, to)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	slots, err := uc.blockedSlotRepo.ListByHome(ctx, homeID)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	busy := make([]presenter.BusyRange, 0, len(bookings)+len(slots))
	for _, b := range bookings {
		busy = append(busy, presenter.BusyRange{StartTime: b.StartTime, EndTime: b.EndTime})
	}
	for _, s := range slots {
		// Half-open, matching blockedslot.HasOverlap: a block ending exactly at from
		// leaves the window free, and answering otherwise would hide a slot Create
		// then accepts.
		if s.StartTime.Before(to) && s.EndTime.After(from) {
			busy = append(busy, presenter.BusyRange{StartTime: s.StartTime, EndTime: s.EndTime})
		}
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].StartTime.Before(busy[j].StartTime) })

	return &presenter.AvailabilityResponse{HomeID: homeID, From: from, To: to, Busy: busy}, nil
}
