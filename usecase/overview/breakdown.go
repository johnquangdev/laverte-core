package overview

import (
	"context"
	"math"
	"sort"
	"time"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/presenter"
)

// breakdownMonths is two years: the selected month's year plus the one before
// it, which is what a same-month-last-year comparison needs.
const breakdownMonths = 24

const monthKeyLayout = "2006-01"

func (uc *UseCase) Breakdown(ctx context.Context, month time.Time) (*presenter.OverviewBreakdownResponse, error) {
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, uc.loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	seriesStart := monthStart.AddDate(0, -(breakdownMonths - 1), 0)

	months, err := uc.monthlySeries(ctx, seriesStart, monthEnd)
	if err != nil {
		return nil, err
	}

	homes, err := uc.homeRepo.List(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	bookings, err := uc.reportRepo.OccupyingBookings(ctx, monthStart, monthEnd)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	revenueRows, err := uc.reportRepo.RevenueByHome(ctx, monthStart, monthEnd)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	revenue := make(map[uint]int64, len(revenueRows))
	for _, r := range revenueRows {
		revenue[r.HomeID] = r.Amount
	}

	occ := computeOccupancy(bookings, monthStart, monthEnd, uc.loc)
	monthHours := monthEnd.Sub(monthStart).Hours()

	out := make([]presenter.OverviewHome, 0, len(homes))
	var sellable, occupied time.Duration
	for _, h := range homes {
		used := occ.occupied[h.ID]
		// A home switched off with nothing in the month was never for sale; leaving
		// it in the denominator would drag occupancy down for a room nobody offered.
		if !h.IsActive && used == 0 && revenue[h.ID] == 0 {
			continue
		}
		sellable += monthEnd.Sub(monthStart)
		occupied += used
		out = append(out, presenter.OverviewHome{
			HomeID: h.ID, Name: h.Name, Category: h.Category, IsActive: h.IsActive,
			BookingCount:     occ.started[h.ID],
			OccupiedHours:    roundTenth(used.Hours()),
			OccupancyPercent: roundTenth(100 * used.Hours() / monthHours),
			RevenueVND:       revenue[h.ID],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].OccupancyPercent > out[j].OccupancyPercent })

	var percent float64
	if sellable > 0 {
		percent = roundTenth(100 * occupied.Hours() / sellable.Hours())
	}

	return &presenter.OverviewBreakdownResponse{
		Month:            monthStart.Format(monthKeyLayout),
		Months:           months,
		OccupancyPercent: percent,
		BookedDays:       occ.bookedDays,
		Homes:            out,
	}, nil
}

func (uc *UseCase) monthlySeries(ctx context.Context, from, to time.Time) ([]presenter.OverviewMonth, error) {
	tz := uc.loc.String()
	revenue, err := uc.reportRepo.MonthlyRevenue(ctx, from, to, tz)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	counts, err := uc.reportRepo.MonthlyBookingsByType(ctx, from, to, tz)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	months := make([]presenter.OverviewMonth, 0, breakdownMonths)
	index := make(map[string]int, breakdownMonths)
	for m := from; m.Before(to); m = m.AddDate(0, 1, 0) {
		key := m.Format(monthKeyLayout)
		index[key] = len(months)
		months = append(months, presenter.OverviewMonth{Month: key})
	}
	for _, r := range revenue {
		if i, ok := index[r.Month]; ok {
			months[i].RevenueVND = r.Amount
		}
	}
	for _, c := range counts {
		i, ok := index[c.Month]
		if !ok {
			continue
		}
		months[i].BookingCount += c.Count
		switch c.BookingType {
		case model.BookingTypeHourly:
			months[i].HourlyCount += c.Count
		case model.BookingTypeOvernight:
			months[i].OvernightCount += c.Count
		case model.BookingTypeDay:
			months[i].DayCount += c.Count
		}
	}
	return months, nil
}

type occupancy struct {
	// occupied is each home's stay time clipped to the window. Stays on one home
	// never overlap — the bookings exclusion constraint forbids it — so a plain
	// sum cannot double count.
	occupied map[uint]time.Duration
	// started counts stays that begin inside the window, the same rule the
	// summary's booking count uses; a stay carried over from last month is
	// occupancy here but a booking there.
	started    map[uint]int64
	bookedDays []int
}

func computeOccupancy(bookings []*model.Booking, from, to time.Time, loc *time.Location) occupancy {
	occ := occupancy{occupied: map[uint]time.Duration{}, started: map[uint]int64{}}
	days := map[int]bool{}
	for _, b := range bookings {
		if !b.StartTime.Before(from) && b.StartTime.Before(to) {
			occ.started[b.HomeID]++
		}
		start, end := b.StartTime, b.EndTime
		if start.Before(from) {
			start = from
		}
		if end.After(to) {
			end = to
		}
		if !end.After(start) {
			continue
		}
		occ.occupied[b.HomeID] += end.Sub(start)

		local := start.In(loc)
		for d := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc); d.Before(end); d = d.AddDate(0, 0, 1) {
			days[d.Day()] = true
		}
	}
	for d := range days {
		occ.bookedDays = append(occ.bookedDays, d)
	}
	sort.Ints(occ.bookedDays)
	return occ
}

func roundTenth(v float64) float64 { return math.Round(v*10) / 10 }
