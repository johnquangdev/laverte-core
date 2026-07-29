package payment

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/internal/testdb"
	"github.com/johnquangdev/laverte-home/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Own database per package: go test runs packages in parallel, and a shared
	// database means one package's cleanup truncates another's fixtures mid-run.
	return testdb.New(t, "payment")
}

// expiresIn builds the *time.Time the pending-expiry CHECK requires.
func expiresIn(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}

// seedPendingPayment inserts the home+booking rows the payments FK requires,
// then a pending sepay payment for that booking.
func seedPendingPayment(t *testing.T, db *gorm.DB, repo IRepository, phone string, amount int64) *model.Payment {
	t.Helper()
	ctx := context.Background()

	home := &model.Home{Name: "Payment Test Home " + phone, Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	booking := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: phone,
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: amount, Status: model.BookingStatusPendingPayment,
		ExpiresAt: expiresIn(15 * time.Minute),
	}
	if err := db.Create(booking).Error; err != nil {
		t.Fatalf("create booking: %v", err)
	}

	p := &model.Payment{
		BookingID: booking.ID, Provider: model.PaymentProviderSePay, Amount: amount,
		Status: model.PaymentStatusPending, QRContent: "https://vietqr.app/img?acc=1&bank=MSB",
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create payment: %v", err)
	}
	return p
}

func TestMarkPaidIfPendingSettlesOnceThenReportsFalse(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()

	p := seedPendingPayment(t, db, repo, "0900000001", 350000)
	paidAt := time.Now().Truncate(time.Second)

	settled, err := repo.MarkPaidIfPending(ctx, p.ID, "998877", paidAt)
	if err != nil {
		t.Fatalf("first MarkPaidIfPending() error = %v", err)
	}
	if !settled {
		t.Fatal("first MarkPaidIfPending() = false, want true")
	}

	// A redelivered webhook must be a no-op, not an error.
	settled, err = repo.MarkPaidIfPending(ctx, p.ID, "998877", paidAt)
	if err != nil {
		t.Fatalf("second MarkPaidIfPending() error = %v", err)
	}
	if settled {
		t.Fatal("second MarkPaidIfPending() = true, want false")
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.PaymentStatusPaid {
		t.Errorf("Status = %q, want %q", got.Status, model.PaymentStatusPaid)
	}
	if got.SePayTransactionRef != "998877" {
		t.Errorf("SePayTransactionRef = %q, want 998877", got.SePayTransactionRef)
	}
	if got.PaidAt == nil {
		t.Error("PaidAt = nil, want a timestamp")
	}
}

func TestGetByBookingIDAndSumPaidBetween(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()

	paid := seedPendingPayment(t, db, repo, "0900000002", 500000)
	pending := seedPendingPayment(t, db, repo, "0900000003", 250000)

	paidAt := time.Now().Truncate(time.Second)
	if _, err := repo.MarkPaidIfPending(ctx, paid.ID, "112233", paidAt); err != nil {
		t.Fatalf("MarkPaidIfPending() error = %v", err)
	}

	got, err := repo.GetByBookingID(ctx, paid.BookingID)
	if err != nil {
		t.Fatalf("GetByBookingID() error = %v", err)
	}
	if got.ID != paid.ID {
		t.Errorf("GetByBookingID().ID = %d, want %d", got.ID, paid.ID)
	}

	total, err := repo.SumPaidBetween(ctx, paidAt.Add(-time.Hour), paidAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("SumPaidBetween() error = %v", err)
	}
	if total != 500000 {
		t.Errorf("SumPaidBetween() = %d, want 500000 (the still-pending %d must not count)", total, pending.Amount)
	}

	empty, err := repo.SumPaidBetween(ctx, paidAt.Add(24*time.Hour), paidAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("SumPaidBetween() over an empty window error = %v", err)
	}
	if empty != 0 {
		t.Errorf("SumPaidBetween() over an empty window = %d, want 0", empty)
	}
}
