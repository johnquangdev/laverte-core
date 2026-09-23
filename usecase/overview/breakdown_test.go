package overview

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-core/model"
	reportrepo "github.com/johnquangdev/laverte-core/repository/report"
)

var ict = time.FixedZone("ICT", 7*60*60)

func at(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, ict)
}

func TestComputeOccupancyClipsToWindowAndMarksEveryDayTouched(t *testing.T) {
	from, to := at(time.September, 1, 0), at(time.October, 1, 0)
	bookings := []*model.Booking{
		// Carried over from August: 6h of it fall in September, and it is not a
		// September booking.
		{HomeID: 1, StartTime: at(time.August, 31, 20), EndTime: at(time.September, 1, 6)},
		// Crosses midnight: marks both the 10th and the 11th.
		{HomeID: 1, StartTime: at(time.September, 10, 22), EndTime: at(time.September, 11, 2)},
		// Runs past the window's end: only the hours to midnight count.
		{HomeID: 2, StartTime: at(time.September, 30, 21), EndTime: at(time.October, 1, 9)},
	}

	occ := computeOccupancy(bookings, from, to, ict)

	if got := occ.occupied[1]; got != 10*time.Hour {
		t.Errorf("home 1 occupied = %v, want 10h (6h carried over + 4h)", got)
	}
	if got := occ.occupied[2]; got != 3*time.Hour {
		t.Errorf("home 2 occupied = %v, want 3h clipped at the window end", got)
	}
	if occ.started[1] != 1 || occ.started[2] != 1 {
		t.Errorf("started = %v, want 1 per home (the carried-over stay is August's)", occ.started)
	}
	if want := []int{1, 10, 11, 30}; !reflect.DeepEqual(occ.bookedDays, want) {
		t.Errorf("bookedDays = %v, want %v", occ.bookedDays, want)
	}
}

// Days are counted in the business zone: 18:00Z on the 5th is 01:00 on the 6th
// there, so the 5th must stay unmarked.
func TestComputeOccupancyBucketsDaysInBusinessZone(t *testing.T) {
	from, to := at(time.September, 1, 0), at(time.October, 1, 0)
	start := time.Date(2026, time.September, 5, 18, 0, 0, 0, time.UTC)
	bookings := []*model.Booking{{HomeID: 1, StartTime: start, EndTime: start.Add(30 * time.Minute)}}

	occ := computeOccupancy(bookings, from, to, ict)
	if want := []int{6}; !reflect.DeepEqual(occ.bookedDays, want) {
		t.Errorf("bookedDays = %v, want %v", occ.bookedDays, want)
	}
}

type fakeReportRepo struct {
	revenue  []reportrepo.MonthAmount
	counts   []reportrepo.MonthTypeCount
	bookings []*model.Booking
	byHome   []reportrepo.HomeAmount
}

func (f *fakeReportRepo) MonthlyRevenue(context.Context, time.Time, time.Time, string) ([]reportrepo.MonthAmount, error) {
	return f.revenue, nil
}
func (f *fakeReportRepo) MonthlyBookingsByType(context.Context, time.Time, time.Time, string) ([]reportrepo.MonthTypeCount, error) {
	return f.counts, nil
}
func (f *fakeReportRepo) OccupyingBookings(context.Context, time.Time, time.Time) ([]*model.Booking, error) {
	return f.bookings, nil
}
func (f *fakeReportRepo) RevenueByHome(context.Context, time.Time, time.Time) ([]reportrepo.HomeAmount, error) {
	return f.byHome, nil
}

type fakeHomeRepo struct{ homes []*model.Home }

func (f *fakeHomeRepo) Create(context.Context, *model.Home) error          { return nil }
func (f *fakeHomeRepo) Update(context.Context, *model.Home) error          { return nil }
func (f *fakeHomeRepo) GetByID(context.Context, uint) (*model.Home, error) { return nil, nil }
func (f *fakeHomeRepo) List(context.Context) ([]*model.Home, error)        { return f.homes, nil }

func TestBreakdownFillsEveryMonthAndSkipsIdleInactiveHomes(t *testing.T) {
	report := &fakeReportRepo{
		revenue: []reportrepo.MonthAmount{{Month: "2026-09", Amount: 1_500_000}, {Month: "2025-10", Amount: 200_000}},
		counts: []reportrepo.MonthTypeCount{
			{Month: "2026-09", BookingType: model.BookingTypeHourly, Count: 3},
			{Month: "2026-09", BookingType: model.BookingTypeOvernight, Count: 1},
		},
		// Home 1 is booked for exactly half of September (15 of 30 days).
		bookings: []*model.Booking{{HomeID: 1, StartTime: at(time.September, 1, 0), EndTime: at(time.September, 16, 0)}},
		byHome:   []reportrepo.HomeAmount{{HomeID: 1, Amount: 1_500_000}},
	}
	homes := &fakeHomeRepo{homes: []*model.Home{
		{ID: 1, Name: "Home 1", Category: model.HomeCategoryHome, IsActive: true},
		{ID: 2, Name: "Nest 1", Category: model.HomeCategoryNest, IsActive: true},
		{ID: 3, Name: "Closed", Category: model.HomeCategoryNest, IsActive: false},
	}}
	uc := New(nil, nil, report, homes, ict).(*UseCase)

	resp, err := uc.Breakdown(context.Background(), at(time.September, 17, 12))
	if err != nil {
		t.Fatalf("Breakdown() error = %v", err)
	}
	if len(resp.Months) != breakdownMonths {
		t.Fatalf("len(Months) = %d, want %d", len(resp.Months), breakdownMonths)
	}
	if first, last := resp.Months[0].Month, resp.Months[len(resp.Months)-1].Month; first != "2024-10" || last != "2026-09" {
		t.Errorf("Months span %s..%s, want 2024-10..2026-09", first, last)
	}
	sep := resp.Months[len(resp.Months)-1]
	if sep.RevenueVND != 1_500_000 || sep.BookingCount != 4 || sep.HourlyCount != 3 || sep.OvernightCount != 1 {
		t.Errorf("September = %+v, want revenue 1.5M and 4 bookings (3 hourly, 1 overnight)", sep)
	}
	if resp.Months[12].Month != "2025-10" || resp.Months[12].RevenueVND != 200_000 {
		t.Errorf("Months[12] = %+v, want 2025-10 with 200k", resp.Months[12])
	}
	if len(resp.Homes) != 2 {
		t.Fatalf("len(Homes) = %d, want 2 (the idle inactive home dropped)", len(resp.Homes))
	}
	if resp.Homes[0].HomeID != 1 || resp.Homes[0].OccupancyPercent != 50 {
		t.Errorf("Homes[0] = %+v, want home 1 first at 50%%", resp.Homes[0])
	}
	// 15 occupied days over 2 sellable homes × 30 days.
	if resp.OccupancyPercent != 25 {
		t.Errorf("OccupancyPercent = %v, want 25", resp.OccupancyPercent)
	}
}
