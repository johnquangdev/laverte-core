package booking

import (
	"context"
	"testing"
	"time"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	pricinguc "github.com/johnquangdev/laverte-core/usecase/pricing"
)

// seedBooking puts a row straight into the fake's id map, bypassing Create: these
// tests care about which rows come back out, not about how they got in.
func (f *fakeBookingRepo) seedBooking(b *model.Booking) {
	if f.byID == nil {
		f.byID = map[uint]*model.Booking{}
	}
	f.nextID++
	b.ID = f.nextID
	f.byID[b.ID] = b
}

func (f *fakeBlockedSlotRepo) seedSlot(homeID uint, start, end time.Time) {
	if f.byHome == nil {
		f.byHome = map[uint][]*model.BlockedSlot{}
	}
	f.byHome[homeID] = append(f.byHome[homeID], &model.BlockedSlot{HomeID: homeID, StartTime: start, EndTime: end})
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("error = %v, want an apperr with code %s", err, code)
	}
	if e.Code != code {
		t.Errorf("error code = %s, want %s", e.Code, code)
	}
}

func TestAvailabilityRejectsEmptyOrReversedRange(t *testing.T) {
	h := newHarness()
	from := time.Now()

	for name, to := range map[string]time.Time{
		"equal":    from,
		"reversed": from.Add(-time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.uc.Availability(context.Background(), 1, from, to)
			wantCode(t, err, apperr.CodeValidation)
		})
	}
}

// The cap is what keeps an unauthenticated caller from asking for a decade of
// history in one query, so it has to be enforced before any repository call.
func TestAvailabilityRejectsRangeOverTheBookingCap(t *testing.T) {
	h := newHarness()
	from := time.Now()

	_, err := h.uc.Availability(context.Background(), 1, from, from.Add(pricinguc.MaxBookingDuration+time.Hour))

	wantCode(t, err, apperr.CodeValidation)
	if len(h.bookings.byID) != 0 || h.slots.byHome != nil {
		t.Error("repositories were touched for a range that should have been refused up front")
	}
}

func TestAvailabilityUnknownHomeIsNotFound(t *testing.T) {
	h := newHarness()
	from := time.Now()

	_, err := h.uc.Availability(context.Background(), 999, from, from.Add(time.Hour))

	wantCode(t, err, apperr.CodeNotFound)
}

func TestAvailabilityInactiveHomeIsRefused(t *testing.T) {
	h := newHarness()
	h.homes.home.IsActive = false
	from := time.Now()

	_, err := h.uc.Availability(context.Background(), 1, from, from.Add(time.Hour))

	wantCode(t, err, apperr.CodeValidation)
}

// A hold that has not been paid for still occupies the slot as far as the
// exclusion constraint is concerned, so reporting it free would hand the guest a
// window their booking is then refused for.
func TestAvailabilityReportsHoldsAndConfirmedOnly(t *testing.T) {
	h := newHarness()
	from := time.Now().Truncate(time.Second)
	to := from.Add(48 * time.Hour)

	h.bookings.seedBooking(&model.Booking{
		HomeID: 1, StartTime: from.Add(time.Hour), EndTime: from.Add(2 * time.Hour),
		Status: model.BookingStatusPendingPayment,
	})
	h.bookings.seedBooking(&model.Booking{
		HomeID: 1, StartTime: from.Add(3 * time.Hour), EndTime: from.Add(4 * time.Hour),
		Status: model.BookingStatusConfirmed,
	})
	h.bookings.seedBooking(&model.Booking{
		HomeID: 1, StartTime: from.Add(5 * time.Hour), EndTime: from.Add(6 * time.Hour),
		Status: model.BookingStatusCancelled,
	})

	resp, err := h.uc.Availability(context.Background(), 1, from, to)
	if err != nil {
		t.Fatalf("Availability() error = %v", err)
	}
	if len(resp.Busy) != 2 {
		t.Fatalf("Busy = %v, want the hold and the confirmed booking only", resp.Busy)
	}
}

func TestAvailabilityFiltersBlockedSlotsHalfOpen(t *testing.T) {
	h := newHarness()
	from := time.Now().Truncate(time.Second)
	to := from.Add(24 * time.Hour)

	h.slots.seedSlot(1, from.Add(-2*time.Hour), from) // ends exactly at from
	h.slots.seedSlot(1, to, to.Add(2*time.Hour))      // starts exactly at to
	h.slots.seedSlot(1, from.Add(-time.Hour), from.Add(time.Hour))
	h.slots.seedSlot(2, from, to) // another home

	resp, err := h.uc.Availability(context.Background(), 1, from, to)
	if err != nil {
		t.Fatalf("Availability() error = %v", err)
	}
	if len(resp.Busy) != 1 {
		t.Fatalf("Busy = %v, want only the slot that truly overlaps [from, to)", resp.Busy)
	}
	if !resp.Busy[0].StartTime.Equal(from.Add(-time.Hour)) {
		t.Errorf("Busy[0].StartTime = %v, want the straddling slot", resp.Busy[0].StartTime)
	}
}

// Bookings and blocked slots arrive as two separate reads; a caller drawing a
// calendar needs one ordered list, not two interleaved ones.
func TestAvailabilitySortsBookingsAndBlockedSlotsTogether(t *testing.T) {
	h := newHarness()
	from := time.Now().Truncate(time.Second)
	to := from.Add(24 * time.Hour)

	h.bookings.seedBooking(&model.Booking{
		HomeID: 1, StartTime: from.Add(6 * time.Hour), EndTime: from.Add(7 * time.Hour),
		Status: model.BookingStatusConfirmed,
	})
	h.bookings.seedBooking(&model.Booking{
		HomeID: 1, StartTime: from.Add(2 * time.Hour), EndTime: from.Add(3 * time.Hour),
		Status: model.BookingStatusConfirmed,
	})
	h.slots.seedSlot(1, from.Add(4*time.Hour), from.Add(5*time.Hour))

	resp, err := h.uc.Availability(context.Background(), 1, from, to)
	if err != nil {
		t.Fatalf("Availability() error = %v", err)
	}
	if len(resp.Busy) != 3 {
		t.Fatalf("len(Busy) = %d, want 3", len(resp.Busy))
	}
	for i := 1; i < len(resp.Busy); i++ {
		if resp.Busy[i].StartTime.Before(resp.Busy[i-1].StartTime) {
			t.Fatalf("Busy is not sorted by start: %v", resp.Busy)
		}
	}
}
