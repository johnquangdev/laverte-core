package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// TestClaimLockCodeSendDeclinesCancelledBooking covers the other half of the claim's
// guard: the sweep lists confirmed bookings, but an admin can cancel one before its
// turn comes, and a cancelled stay must not receive the door code.
func TestClaimLockCodeSendDeclinesCancelledBooking(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Cancelled Claim Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedConfirmedBooking(t, db, home.ID, "0900000013")
	if err := db.Model(&model.Booking{}).Where("id = ?", b.ID).
		Update("status", model.BookingStatusCancelled).Error; err != nil {
		t.Fatalf("cancel booking: %v", err)
	}

	claimed, err := repo.ClaimLockCodeSend(ctx, b.ID, time.Now())
	if err != nil {
		t.Fatalf("ClaimLockCodeSend() error = %v", err)
	}
	if claimed {
		t.Fatal("ClaimLockCodeSend() claimed = true for a cancelled booking, want false")
	}

	var sentAt sql.NullTime
	if err := db.Model(&model.Booking{}).Where("id = ?", b.ID).Pluck("lock_code_sent_at", &sentAt).Error; err != nil {
		t.Fatalf("read back lock_code_sent_at: %v", err)
	}
	if sentAt.Valid {
		t.Fatalf("lock_code_sent_at = %v, want SQL NULL — a declined claim must not stamp the row", sentAt.Time)
	}
}

// TestExpireIfPendingOnlyMovesPendingRows is the guard that keeps the expiry sweep from
// reverting a booking the webhook confirmed after the sweep's list query. Expired sits
// outside the overlap exclusion constraint, so a lost race there reopens a paid guest's
// slot to a stranger.
func TestExpireIfPendingOnlyMovesPendingRows(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Expire Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	expiresAt := time.Now().Add(-time.Minute)
	pending := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000014",
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: &expiresAt,
	}
	if err := db.Create(pending).Error; err != nil {
		t.Fatalf("seed pending booking: %v", err)
	}

	expired, err := repo.ExpireIfPending(ctx, pending.ID)
	if err != nil {
		t.Fatalf("ExpireIfPending() error = %v", err)
	}
	if !expired {
		t.Fatal("ExpireIfPending() expired = false for a pending row, want true")
	}
	got, err := repo.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusExpired {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusExpired)
	}

	// A second sweep tick — or the same tick after the webhook confirmed the row —
	// must decline rather than write.
	confirmed := seedConfirmedBooking(t, db, home.ID, "0900000015")
	expired, err = repo.ExpireIfPending(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("ExpireIfPending() on a confirmed row error = %v", err)
	}
	if expired {
		t.Fatal("ExpireIfPending() expired = true for a confirmed booking, want false")
	}
	got, err = repo.GetByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusConfirmed)
	}
}

// seedPendingBooking inserts a hold with an expiry the pending CHECK requires —
// the starting state for every confirm/release guard below. startIn selects the
// slot: two rows a test keeps inside the guarded statuses at the same time need
// different windows, or the exclusion constraint rejects the second insert.
func seedPendingBooking(t *testing.T, db *gorm.DB, homeID uint, phone string, startIn time.Duration) *model.Booking {
	t.Helper()
	start := time.Now().Add(startIn).Truncate(time.Second)
	b := &model.Booking{
		HomeID: homeID, CustomerName: "A", CustomerPhone: phone,
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment, ExpiresAt: expiresIn(time.Hour),
	}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("seed pending booking: %v", err)
	}
	return b
}

// setStatus moves a seeded row directly, standing in for whatever concurrent
// writer the guard under test has to lose to.
func setStatus(t *testing.T, db *gorm.DB, id uint, status string) {
	t.Helper()
	if err := db.Model(&model.Booking{}).Where("id = ?", id).Update("status", status).Error; err != nil {
		t.Fatalf("set status %s: %v", status, err)
	}
}

// TestSetCalendarEventIDChangesOnlyThatColumn covers the write that reproduced a
// cancelled booking coming back to life: the event id is persisted after a
// multi-second Calendar call, so an admin can cancel inside that window. Writing
// the one column leaves their cancellation standing; a full-row write from the
// pre-call snapshot would restore 'confirmed', and confirmed re-holds the slot.
func TestSetCalendarEventIDChangesOnlyThatColumn(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Calendar Event Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedConfirmedBooking(t, db, home.ID, "0900000030")
	setStatus(t, db, b.ID, model.BookingStatusCancelled)

	if err := repo.SetCalendarEventID(ctx, b.ID, "gcal-evt-9"); err != nil {
		t.Fatalf("SetCalendarEventID() error = %v", err)
	}

	got, err := repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.GoogleCalendarEventID != "gcal-evt-9" {
		t.Errorf("GoogleCalendarEventID = %q, want gcal-evt-9", got.GoogleCalendarEventID)
	}
	if got.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q — the concurrent cancel must survive", got.Status, model.BookingStatusCancelled)
	}
}

// TestSetPaymentIDChangesOnlyThatColumn is the same proof for the payment link:
// the walk-in and guest create paths both write it after inserting the payment
// row, by which time another writer may have moved the status.
func TestSetPaymentIDChangesOnlyThatColumn(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Payment Link Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}
	b := seedPendingBooking(t, db, home.ID, "0900000031", time.Hour)
	setStatus(t, db, b.ID, model.BookingStatusConfirmed)

	if err := repo.SetPaymentID(ctx, b.ID, 55); err != nil {
		t.Fatalf("SetPaymentID() error = %v", err)
	}

	got, err := repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.PaymentID == nil || *got.PaymentID != 55 {
		t.Errorf("PaymentID = %v, want 55", got.PaymentID)
	}
	if got.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusConfirmed)
	}
}

// TestConfirmIfPendingWritesStatusAndPaymentTogether pins both halves of the
// webhook's settle write. Met: status and payment_id move in one statement, so
// there is never a confirmed booking with no payment link. Unmet: a hold the
// admin cancelled while the transfer was in flight must not be resurrected —
// 'confirmed' is inside the exclusion constraint, so it would re-hold the slot.
func TestConfirmIfPendingWritesStatusAndPaymentTogether(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Confirm Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	pending := seedPendingBooking(t, db, home.ID, "0900000032", time.Hour)
	confirmed, err := repo.ConfirmIfPending(ctx, pending.ID, 91)
	if err != nil {
		t.Fatalf("ConfirmIfPending() error = %v", err)
	}
	if !confirmed {
		t.Fatal("ConfirmIfPending() = false for a pending row, want true")
	}
	got, err := repo.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusConfirmed)
	}
	if got.PaymentID == nil || *got.PaymentID != 91 {
		t.Errorf("PaymentID = %v, want 91", got.PaymentID)
	}

	cancelled := seedPendingBooking(t, db, home.ID, "0900000033", 5*time.Hour)
	setStatus(t, db, cancelled.ID, model.BookingStatusCancelled)
	confirmed, err = repo.ConfirmIfPending(ctx, cancelled.ID, 92)
	if err != nil {
		t.Fatalf("ConfirmIfPending() on a cancelled row error = %v", err)
	}
	if confirmed {
		t.Fatal("ConfirmIfPending() = true for a cancelled booking, want false")
	}
	got, err = repo.GetByID(ctx, cancelled.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusCancelled)
	}
	if got.PaymentID != nil {
		t.Errorf("PaymentID = %v, want nil — a declined confirm must write nothing", got.PaymentID)
	}
}

// TestCancelIfNotTerminalDeclinesEveryTerminalStatus pins the other proven
// failure: a cancel that lands in the instant a hold's transfer settles must not
// win. Writing status='cancelled' over a paid booking puts the slot back on sale
// while payment.paid_at and the SePay ref stay stamped and irreversible — the
// guest paid, has no room, and nothing links their money to any booking.
func TestCancelIfNotTerminalDeclinesEveryTerminalStatus(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Cancel Guard Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	confirmed := seedConfirmedBooking(t, db, home.ID, "0900000034")
	cancelled, err := repo.CancelIfNotTerminal(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("CancelIfNotTerminal() error = %v", err)
	}
	if !cancelled {
		t.Fatal("CancelIfNotTerminal() = false for a confirmed row, want true")
	}
	got, err := repo.GetByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusCancelled)
	}

	phone := 40
	for _, terminal := range []string{
		model.BookingStatusCancelled, model.BookingStatusExpired,
		model.BookingStatusCompleted, model.BookingStatusNoShow,
	} {
		phone++
		b := seedConfirmedBooking(t, db, home.ID, fmt.Sprintf("09000000%d", phone))
		setStatus(t, db, b.ID, terminal)

		cancelled, err = repo.CancelIfNotTerminal(ctx, b.ID)
		if err != nil {
			t.Fatalf("CancelIfNotTerminal() on %s error = %v", terminal, err)
		}
		if cancelled {
			t.Errorf("CancelIfNotTerminal() = true for %s, want false", terminal)
		}
		got, err = repo.GetByID(ctx, b.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.Status != terminal {
			t.Errorf("Status = %q, want %q left untouched", got.Status, terminal)
		}
	}
}

// TestCompleteIfConfirmedOnlyMovesConfirmedRows and its no-show twin below guard
// a release rather than a hold: both statuses sit outside the exclusion
// constraint, so winning against a booking that is no longer confirmed would
// free a slot that is still occupied.
func TestCompleteIfConfirmedOnlyMovesConfirmedRows(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Complete Guard Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	confirmed := seedConfirmedBooking(t, db, home.ID, "0900000050")
	completed, err := repo.CompleteIfConfirmed(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("CompleteIfConfirmed() error = %v", err)
	}
	if !completed {
		t.Fatal("CompleteIfConfirmed() = false for a confirmed row, want true")
	}
	got, err := repo.GetByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusCompleted)
	}

	pending := seedPendingBooking(t, db, home.ID, "0900000051", 5*time.Hour)
	completed, err = repo.CompleteIfConfirmed(ctx, pending.ID)
	if err != nil {
		t.Fatalf("CompleteIfConfirmed() on a pending row error = %v", err)
	}
	if completed {
		t.Fatal("CompleteIfConfirmed() = true for a pending booking, want false")
	}
	got, err = repo.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusPendingPayment)
	}
}

func TestNoShowIfConfirmedOnlyMovesConfirmedRows(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "NoShow Guard Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	confirmed := seedConfirmedBooking(t, db, home.ID, "0900000052")
	marked, err := repo.NoShowIfConfirmed(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("NoShowIfConfirmed() error = %v", err)
	}
	if !marked {
		t.Fatal("NoShowIfConfirmed() = false for a confirmed row, want true")
	}
	got, err := repo.GetByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusNoShow {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusNoShow)
	}

	pending := seedPendingBooking(t, db, home.ID, "0900000053", 5*time.Hour)
	marked, err = repo.NoShowIfConfirmed(ctx, pending.ID)
	if err != nil {
		t.Fatalf("NoShowIfConfirmed() on a pending row error = %v", err)
	}
	if marked {
		t.Fatal("NoShowIfConfirmed() = true for a pending booking, want false")
	}
	got, err = repo.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusPendingPayment)
	}
}

// TestReleaseHoldIfPendingOnlyMovesPendingRows covers the unwind of a create
// that failed after its booking row committed. Losing the guard here would let a
// provider timeout expire a booking the transfer had meanwhile paid for.
func TestReleaseHoldIfPendingOnlyMovesPendingRows(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Release Hold Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	pending := seedPendingBooking(t, db, home.ID, "0900000054", time.Hour)
	released, err := repo.ReleaseHoldIfPending(ctx, pending.ID)
	if err != nil {
		t.Fatalf("ReleaseHoldIfPending() error = %v", err)
	}
	if !released {
		t.Fatal("ReleaseHoldIfPending() = false for a pending row, want true")
	}
	got, err := repo.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusExpired {
		t.Errorf("Status = %q, want %q", got.Status, model.BookingStatusExpired)
	}

	confirmed := seedConfirmedBooking(t, db, home.ID, "0900000055")
	released, err = repo.ReleaseHoldIfPending(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("ReleaseHoldIfPending() on a confirmed row error = %v", err)
	}
	if released {
		t.Fatal("ReleaseHoldIfPending() = true for a confirmed booking, want false")
	}
	got, err = repo.GetByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q left untouched", got.Status, model.BookingStatusConfirmed)
	}
}

// TestCountConfirmedBetweenFiltersStatusAndRange proves the revenue overview's
// booking count only tallies stays that actually occupy the property: it must
// count confirmed and completed rows in range, and it must exclude a pending,
// cancelled, and out-of-range confirmed row that a status- or range-less query
// would wrongly include.
func TestCountConfirmedBetweenFiltersStatusAndRange(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Overview Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	seed := func(phone string, status string, start time.Time) {
		b := &model.Booking{
			HomeID: home.ID, CustomerName: "A", CustomerPhone: phone,
			StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
			ComputedPrice: 100000, Status: status,
		}
		if status == model.BookingStatusPendingPayment {
			b.ExpiresAt = expiresIn(time.Hour)
		}
		if err := db.Create(b).Error; err != nil {
			t.Fatalf("seed booking (status %s): %v", status, err)
		}
	}

	// Each row gets its own slot: pending_payment and confirmed both sit inside
	// the exclusion constraint's guarded statuses, so overlapping start times
	// here would fail on the constraint this test isn't exercising.
	seed("0900000020", model.BookingStatusConfirmed, from.Add(24*time.Hour))
	seed("0900000021", model.BookingStatusCompleted, from.Add(48*time.Hour))
	seed("0900000022", model.BookingStatusPendingPayment, from.Add(72*time.Hour))
	seed("0900000023", model.BookingStatusCancelled, from.Add(96*time.Hour))
	seed("0900000024", model.BookingStatusConfirmed, to.Add(24*time.Hour))

	got, err := repo.CountConfirmedBetween(ctx, from, to)
	if err != nil {
		t.Fatalf("CountConfirmedBetween() error = %v", err)
	}
	if got != 2 {
		t.Errorf("CountConfirmedBetween() = %d, want 2 (confirmed + completed in range only)", got)
	}
}
