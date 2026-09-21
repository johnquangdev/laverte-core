package blockedslot

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/internal/testdb"
	"github.com/johnquangdev/laverte-core/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Own database per package: go test runs packages in parallel, and a shared
	// database means one package's cleanup truncates another's fixtures mid-run.
	return testdb.New(t, "blockedslot")
}

// TestHasOverlapBoundaries pins the half-open [start, end) convention
// HasOverlap's WHERE clause implements. It must agree with bookings' EXCLUDE
// constraint, which uses tstzrange(start_time, end_time) — Postgres's range
// constructor defaults to the same '[)' bound flag — because blocked_slots has
// no DB constraint of its own backing this check: HasOverlap is a plain SELECT
// COUNT, so a slot the two disagreed on would let a guest book a home mid-block
// with nothing in the database to catch it.
func TestHasOverlapBoundaries(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	homeA := &model.Home{Name: "Boundary Home A", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(homeA).Error; err != nil {
		t.Fatalf("create home A: %v", err)
	}
	homeB := &model.Home{Name: "Boundary Home B", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(homeB).Error; err != nil {
		t.Fatalf("create home B: %v", err)
	}

	blockStart := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	blockEnd := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	block := &model.BlockedSlot{HomeID: homeA.ID, StartTime: blockStart, EndTime: blockEnd, Reason: "maintenance"}
	if err := repo.Create(ctx, block); err != nil {
		t.Fatalf("create blocked slot: %v", err)
	}

	hour := time.Hour
	cases := []struct {
		name       string
		homeID     uint
		start, end time.Time
		want       bool
	}{
		{"ends exactly when block starts", homeA.ID, blockStart.Add(-2 * hour), blockStart, false},
		{"straddles the start", homeA.ID, blockStart.Add(-hour), blockStart.Add(hour), true},
		{"fully inside", homeA.ID, blockStart.Add(30 * time.Minute), blockStart.Add(hour), true},
		{"fully contains the block", homeA.ID, blockStart.Add(-hour), blockEnd.Add(hour), true},
		{"starts exactly when block ends", homeA.ID, blockEnd, blockEnd.Add(2 * hour), false},
		{"entirely after", homeA.ID, blockEnd.Add(hour), blockEnd.Add(3 * hour), false},
		{"same window on a different home", homeB.ID, blockStart, blockEnd, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.HasOverlap(ctx, tc.homeID, tc.start, tc.end)
			if err != nil {
				t.Fatalf("HasOverlap() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("HasOverlap() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDeleteUnknownIDReportsNotFound proves Delete distinguishes "removed" from
// "was never there": this endpoint's whole purpose is to make a home bookable
// again, so reporting success for an id that matched no row would tell the
// admin the block is cleared while HasOverlap keeps refusing bookings for it.
func TestDeleteUnknownIDReportsNotFound(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)

	if err := repo.Delete(context.Background(), 999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Delete(unknown id) error = %v, want gorm.ErrRecordNotFound", err)
	}
}
