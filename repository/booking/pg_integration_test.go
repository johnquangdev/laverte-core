package booking

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/migrations"
	"github.com/johnquangdev/laverte-home/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	if _, err = migrate.Exec(sqlDB, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() {
		// Truncate rather than migrate-down: these tests share one database, so
		// each needs a clean slate without tearing the schema out from under a
		// sibling test.
		if _, err = sqlDB.Exec("TRUNCATE bookings, homes RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate: %v", err)
		}
		if err = sqlDB.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gormDB
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
