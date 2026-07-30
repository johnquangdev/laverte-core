package booking

import (
	"context"
	"database/sql"
	"errors"
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
	return testdb.New(t, "booking")
}

// expiresIn builds the *time.Time the pending-expiry CHECK requires.
func expiresIn(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}

func TestCreateRejectsOverlappingBookingForSameHome(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Hour)

	first := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: end, BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	overlapping := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start.Add(30 * time.Minute), EndTime: end.Add(30 * time.Minute),
		BookingType: model.BookingTypeHourly, ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
		ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, overlapping); !errors.Is(err, ErrSlotConflict) {
		t.Fatalf("second Create() error = %v, want ErrSlotConflict", err)
	}
}

func TestCreateAllowsNonOverlappingBookingForSameHome(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 2", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	first := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	after := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start.Add(time.Hour), EndTime: start.Add(2 * time.Hour),
		BookingType: model.BookingTypeHourly, ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
		ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, after); err != nil {
		t.Fatalf("adjacent (non-overlapping) Create() error = %v, want nil", err)
	}
}

func TestCreateAllowsOverlapWhenFirstIsCancelled(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 3", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	cancelled := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusCancelled,
	}
	if err := repo.Create(ctx, cancelled); err != nil {
		t.Fatalf("create cancelled booking: %v", err)
	}

	overlapping := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, overlapping); err != nil {
		t.Fatalf("Create() overlapping a cancelled booking error = %v, want nil", err)
	}
}

// TestCreateAllowsOverlapWhenFirstIsExpired proves the partial WHERE clause in
// the exclusion constraint covers 'expired' the same way it covers
// 'cancelled': an expired hold must release its slot for a new booking to
// take it, since only 'pending_payment' and 'confirmed' rows are guarded.
func TestCreateAllowsOverlapWhenFirstIsExpired(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 4", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	expired := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusExpired,
	}
	if err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("create expired booking: %v", err)
	}

	overlapping := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, overlapping); err != nil {
		t.Fatalf("Create() overlapping an expired booking error = %v, want nil", err)
	}
}

// TestCreateRejectsZeroAndNegativeDuration proves the bookings_time_order CHECK:
// tstzrange(start, start) is an EMPTY range and overlaps nothing, so without this
// CHECK the exclusion constraint would give a zero-duration booking no protection
// at all, and Postgres itself would raise an unmappable driver error for end <
// start rather than a constraint the repository can translate.
func TestCreateRejectsZeroAndNegativeDuration(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 5", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)

	zeroDuration := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start, BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, zeroDuration); err == nil {
		t.Fatal("Create() with start == end error = nil, want a rejection")
	}

	negativeDuration := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start, EndTime: start.Add(-time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := repo.Create(ctx, negativeDuration); err == nil {
		t.Fatal("Create() with end before start error = nil, want a rejection")
	}
}

// TestCreateRejectsPendingWithoutExpiry proves the bookings_pending_has_expiry
// CHECK: GetPendingByPhone filters on expires_at > now(), and SQL treats
// NULL > now() as unknown rather than true, so a pending_payment row with no
// expiry would be invisible to that anti-spam gate and defeat it silently.
func TestCreateRejectsPendingWithoutExpiry(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 6", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	noExpiry := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: nil,
	}
	if err := repo.Create(ctx, noExpiry); err == nil {
		t.Fatal("Create() pending_payment with nil ExpiresAt error = nil, want a rejection")
	}
}

// seedConfirmedBooking inserts a confirmed booking with no door code and no
// lock-code timestamps — the starting state for the lock-code sweeps.
func seedConfirmedBooking(t *testing.T, db *gorm.DB, homeID uint, phone string) *model.Booking {
	t.Helper()
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	b := &model.Booking{
		HomeID: homeID, CustomerName: "A", CustomerPhone: phone,
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusConfirmed,
	}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("seed confirmed booking: %v", err)
	}
	return b
}

// TestClaimLockCodeSendFirstClaimWinsSecondFails proves the guard a door code's
// physical-access risk requires: two concurrent claims against the same row
// must not both succeed, or the guest's lock code goes out twice.
func TestClaimLockCodeSendFirstClaimWinsSecondFails(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Claim Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedConfirmedBooking(t, db, home.ID, "0900000010")

	claimed, err := repo.ClaimLockCodeSend(ctx, b.ID, time.Now())
	if err != nil {
		t.Fatalf("first ClaimLockCodeSend() error = %v", err)
	}
	if !claimed {
		t.Fatal("first ClaimLockCodeSend() claimed = false, want true")
	}

	claimedAgain, err := repo.ClaimLockCodeSend(ctx, b.ID, time.Now())
	if err != nil {
		t.Fatalf("second ClaimLockCodeSend() error = %v", err)
	}
	if claimedAgain {
		t.Fatal("second ClaimLockCodeSend() claimed = true, want false — the row is already claimed")
	}
}

// TestReleaseLockCodeSendAllowsReclaim proves ReleaseLockCodeSend writes actual
// SQL NULL rather than a Go zero value the driver might serialize differently,
// since a claim's guard is a plain IS NULL check.
func TestReleaseLockCodeSendAllowsReclaim(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Release Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedConfirmedBooking(t, db, home.ID, "0900000011")

	if _, err := repo.ClaimLockCodeSend(ctx, b.ID, time.Now()); err != nil {
		t.Fatalf("ClaimLockCodeSend() error = %v", err)
	}

	if err := repo.ReleaseLockCodeSend(ctx, b.ID); err != nil {
		t.Fatalf("ReleaseLockCodeSend() error = %v", err)
	}

	var sentAt sql.NullTime
	if err := db.Model(&model.Booking{}).Where("id = ?", b.ID).Pluck("lock_code_sent_at", &sentAt).Error; err != nil {
		t.Fatalf("read back lock_code_sent_at: %v", err)
	}
	if sentAt.Valid {
		t.Fatalf("lock_code_sent_at = %v after release, want SQL NULL", sentAt.Time)
	}

	claimed, err := repo.ClaimLockCodeSend(ctx, b.ID, time.Now())
	if err != nil {
		t.Fatalf("re-claim ClaimLockCodeSend() error = %v", err)
	}
	if !claimed {
		t.Fatal("re-claim ClaimLockCodeSend() claimed = false, want true — release must have written a real NULL")
	}
}

// TestSetDoorLockCodeChangesOnlyThatColumn proves SetDoorLockCode is the
// single-column write its comment in the interface promises: a full-row Save
// racing an admin editing the same booking would silently discard their change.
func TestSetDoorLockCodeChangesOnlyThatColumn(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "SetCode Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedConfirmedBooking(t, db, home.ID, "0900000012")

	// Simulate a concurrent admin edit to a different column that SetDoorLockCode
	// must not clobber if it were a full-row Save from a stale in-memory copy.
	if err := db.Model(&model.Booking{}).Where("id = ?", b.ID).Update("customer_name", "Changed Concurrently").Error; err != nil {
		t.Fatalf("simulate concurrent edit: %v", err)
	}

	if err := repo.SetDoorLockCode(ctx, b.ID, "9876"); err != nil {
		t.Fatalf("SetDoorLockCode() error = %v", err)
	}

	got, err := repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.DoorLockCode == nil || *got.DoorLockCode != "9876" {
		t.Errorf("DoorLockCode = %v, want \"9876\"", got.DoorLockCode)
	}
	if got.CustomerName != "Changed Concurrently" {
		t.Errorf("CustomerName = %q, want the concurrently-written value to survive untouched", got.CustomerName)
	}
}
