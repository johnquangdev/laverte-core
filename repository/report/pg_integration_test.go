package report

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/internal/testdb"
	"github.com/johnquangdev/laverte-core/model"
)

func TestReportAggregatesBucketInRequestedZone(t *testing.T) {
	db := testdb.New(t, "report")
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()
	ict := time.FixedZone("ICT", 7*60*60)

	home := &model.Home{Name: "Report Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	seed := func(start time.Time, status, bookingType string, paidAt *time.Time) {
		t.Helper()
		b := &model.Booking{
			HomeID: home.ID, CustomerName: "R", CustomerPhone: "84900000010",
			StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: bookingType,
			ComputedPrice: 300000, Status: status,
		}
		if err := db.Create(b).Error; err != nil {
			t.Fatalf("create booking: %v", err)
		}
		if paidAt == nil {
			return
		}
		p := &model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Amount: 300000,
			Status: model.PaymentStatusPaid, PaidAt: paidAt}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("create payment: %v", err)
		}
	}

	// 20:00Z on 31 Aug is 03:00 on 1 Sep in the business zone: September money.
	lateAugustUTC := time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	seed(time.Date(2026, 9, 2, 10, 0, 0, 0, ict), model.BookingStatusConfirmed, model.BookingTypeHourly, &lateAugustUTC)
	paid := time.Date(2026, 9, 10, 9, 0, 0, 0, ict)
	seed(time.Date(2026, 9, 12, 22, 0, 0, 0, ict), model.BookingStatusCompleted, model.BookingTypeOvernight, &paid)
	// A cancelled stay occupies nothing and is not counted.
	seed(time.Date(2026, 9, 15, 10, 0, 0, 0, ict), model.BookingStatusCancelled, model.BookingTypeHourly, nil)

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, ict)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, ict)

	revenue, err := repo.MonthlyRevenue(ctx, from, to, "Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("MonthlyRevenue() error = %v", err)
	}
	if len(revenue) != 1 || revenue[0].Month != "2026-09" || revenue[0].Amount != 600000 {
		t.Errorf("MonthlyRevenue() = %+v, want only 2026-09 with 600000", revenue)
	}

	counts, err := repo.MonthlyBookingsByType(ctx, from, to, "Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("MonthlyBookingsByType() error = %v", err)
	}
	byType := map[string]int64{}
	for _, c := range counts {
		if c.Month != "2026-09" {
			t.Errorf("unexpected month %q", c.Month)
		}
		byType[c.BookingType] += c.Count
	}
	if byType[model.BookingTypeHourly] != 1 || byType[model.BookingTypeOvernight] != 1 {
		t.Errorf("counts by type = %v, want 1 hourly and 1 overnight (cancelled excluded)", byType)
	}

	sepFrom := time.Date(2026, 9, 1, 0, 0, 0, 0, ict)
	occupying, err := repo.OccupyingBookings(ctx, sepFrom, to)
	if err != nil {
		t.Fatalf("OccupyingBookings() error = %v", err)
	}
	if len(occupying) != 2 {
		t.Errorf("len(OccupyingBookings()) = %d, want 2", len(occupying))
	}

	byHome, err := repo.RevenueByHome(ctx, sepFrom, to)
	if err != nil {
		t.Fatalf("RevenueByHome() error = %v", err)
	}
	if len(byHome) != 1 || byHome[0].HomeID != home.ID || byHome[0].Amount != 600000 {
		t.Errorf("RevenueByHome() = %+v, want home %d with 600000", byHome, home.ID)
	}
}
