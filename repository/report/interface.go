// Package report holds the read-only aggregates behind the admin overview. None
// of it takes part in a write path, so it is kept out of the booking and payment
// repositories whose interfaces every settlement fake has to implement.
package report

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

type MonthAmount struct {
	Month  string // YYYY-MM in the zone the caller asked for
	Amount int64
}

type MonthTypeCount struct {
	Month       string
	BookingType string
	Count       int64
}

type HomeAmount struct {
	HomeID uint
	Amount int64
}

type IRepository interface {
	// MonthlyRevenue totals paid payments by the month their paid_at falls in,
	// bucketed in tz (an IANA zone name). Months with no revenue are absent.
	MonthlyRevenue(ctx context.Context, from, to time.Time, tz string) ([]MonthAmount, error)
	// MonthlyBookingsByType counts confirmed and completed bookings by the month
	// their start_time falls in — the same statuses and clock CountConfirmedBetween
	// uses, so the breakdown adds up to the summary's booking count.
	MonthlyBookingsByType(ctx context.Context, from, to time.Time, tz string) ([]MonthTypeCount, error)
	// OccupyingBookings returns confirmed and completed bookings overlapping
	// [from, to), across every home.
	OccupyingBookings(ctx context.Context, from, to time.Time) ([]*model.Booking, error)
	// RevenueByHome totals paid payments with paid_at in [from, to) per home.
	RevenueByHome(ctx context.Context, from, to time.Time) ([]HomeAmount, error)
}
