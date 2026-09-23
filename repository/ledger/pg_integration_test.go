package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/internal/testdb"
	"github.com/johnquangdev/laverte-core/model"
)

type fixture struct {
	db      *gorm.DB
	repo    IRepository
	adminID uint
	homeID  uint
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testdb.New(t, "ledger")
	admin := &model.User{Email: "admin@test.local", OAuthProvider: "test", OAuthID: "1", Role: model.RoleAdmin}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	home := &model.Home{Name: "Ledger Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	return fixture{db: db, repo: NewPG(func(context.Context) *gorm.DB { return db }), adminID: admin.ID, homeID: home.ID}
}

// seedPaid inserts a booking in bookingStatus with a paid payment against it.
// Each call takes its own hour so the bookings exclusion constraint never trips.
func (f fixture) seedPaid(t *testing.T, offset time.Duration, bookingStatus string) (*model.Booking, *model.Payment) {
	t.Helper()
	start := time.Now().Add(24*time.Hour + offset).Truncate(time.Second)
	b := &model.Booking{
		HomeID: f.homeID, CustomerName: "Khach Ledger", CustomerPhone: "84900000009",
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 450000, Status: bookingStatus,
	}
	if err := f.db.Create(b).Error; err != nil {
		t.Fatalf("create booking: %v", err)
	}
	paidAt := time.Now().Truncate(time.Second)
	p := &model.Payment{
		BookingID: b.ID, Provider: model.PaymentProviderSePay, Amount: 450000,
		Status: model.PaymentStatusPaid, PaidAt: &paidAt,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("create payment: %v", err)
	}
	return b, p
}

func TestListPaymentsJoinsBookingFieldsWithinRange(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	b, p := f.seedPaid(t, 0, model.BookingStatusConfirmed)

	rows, err := f.repo.ListPayments(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("ListPayments() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.ID != p.ID || got.BookingID != b.ID || got.Status != model.PaymentStatusPaid {
		t.Errorf("payment fields = id %d booking %d status %q, want %d / %d / paid", got.ID, got.BookingID, got.Status, p.ID, b.ID)
	}
	if got.CustomerName != "Khach Ledger" || got.HomeID != f.homeID || got.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("booking fields = %q / home %d / %q, want the joined booking's", got.CustomerName, got.HomeID, got.BookingStatus)
	}

	rows, err = f.repo.ListPayments(ctx, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour), 10)
	if err != nil {
		t.Fatalf("ListPayments() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) outside range = %d, want 0", len(rows))
	}
}

func TestGetPaymentMissingIsRecordNotFound(t *testing.T) {
	f := setup(t)
	if _, err := f.repo.GetPayment(context.Background(), 999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetPayment() error = %v, want gorm.ErrRecordNotFound", err)
	}
}

// The guard lives in the UPDATE: a stay that is still going ahead keeps its
// money, and a refund lands exactly once.
func TestRefundIfStayEndedOnlyForEndedStayAndOnlyOnce(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	b, p := f.seedPaid(t, 0, model.BookingStatusConfirmed)

	refunded, err := f.repo.RefundIfStayEnded(ctx, p.ID, f.adminID, "hoan", time.Now())
	if err != nil {
		t.Fatalf("RefundIfStayEnded() on a confirmed stay error = %v", err)
	}
	if refunded {
		t.Fatal("RefundIfStayEnded() on a confirmed stay = true, want false")
	}

	if err = f.db.Model(&model.Booking{}).Where("id = ?", b.ID).Update("status", model.BookingStatusCancelled).Error; err != nil {
		t.Fatalf("cancel booking: %v", err)
	}
	refunded, err = f.repo.RefundIfStayEnded(ctx, p.ID, f.adminID, "CK hoan FT1", time.Now())
	if err != nil || !refunded {
		t.Fatalf("RefundIfStayEnded() after cancel = %v, %v; want true, nil", refunded, err)
	}
	row, err := f.repo.GetPayment(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if row.Status != model.PaymentStatusRefunded || row.RefundNote != "CK hoan FT1" || row.RefundedAt == nil ||
		row.RefundedByAdminID == nil || *row.RefundedByAdminID != f.adminID {
		t.Errorf("after refund = %+v, want refunded with note, time and admin", row.Payment)
	}

	refunded, err = f.repo.RefundIfStayEnded(ctx, p.ID, f.adminID, "again", time.Now())
	if err != nil || refunded {
		t.Fatalf("second RefundIfStayEnded() = %v, %v; want false, nil", refunded, err)
	}
}
