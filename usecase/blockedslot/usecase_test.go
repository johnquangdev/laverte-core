package blockedslot

import (
	"context"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
)

type fakeRepo struct {
	slots  map[uint]*model.BlockedSlot
	nextID uint
}

func newFakeRepo() *fakeRepo { return &fakeRepo{slots: map[uint]*model.BlockedSlot{}} }

func (f *fakeRepo) Create(_ context.Context, s *model.BlockedSlot) error {
	f.nextID++
	s.ID = f.nextID
	f.slots[s.ID] = s
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, id uint) error {
	delete(f.slots, id)
	return nil
}

func (f *fakeRepo) ListByHome(_ context.Context, homeID uint) ([]*model.BlockedSlot, error) {
	out := make([]*model.BlockedSlot, 0, len(f.slots))
	for _, s := range f.slots {
		if s.HomeID == homeID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) HasOverlap(_ context.Context, homeID uint, start, end time.Time) (bool, error) {
	for _, s := range f.slots {
		if s.HomeID == homeID && s.StartTime.Before(end) && s.EndTime.After(start) {
			return true, nil
		}
	}
	return false, nil
}

func TestCreateRejectsEndBeforeStart(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 1, StartTime: start, EndTime: start.Add(-time.Hour)}

	if _, err := uc.Create(context.Background(), req, 7); err == nil {
		t.Fatal("Create() with end before start = nil error, want error")
	}
}

func TestCreateRejectsZeroLengthSlot(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 1, StartTime: start, EndTime: start}

	if _, err := uc.Create(context.Background(), req, 7); err == nil {
		t.Fatal("Create() with end == start = nil error, want error")
	}
}

func TestCreateThenListByHomeRoundTrip(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 3, StartTime: start, EndTime: start.Add(4 * time.Hour), Reason: "bao tri dieu hoa"}

	created, err := uc.Create(context.Background(), req, 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.CreatedByAdminID == nil || *created.CreatedByAdminID != 7 {
		t.Errorf("CreatedByAdminID = %v, want 7", created.CreatedByAdminID)
	}

	list, err := uc.ListByHome(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListByHome() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID || list[0].Reason != "bao tri dieu hoa" {
		t.Errorf("ListByHome() = %+v, want one item with ID %d", list, created.ID)
	}

	other, err := uc.ListByHome(context.Background(), 99)
	if err != nil {
		t.Fatalf("ListByHome(99) error = %v", err)
	}
	if len(other) != 0 {
		t.Errorf("ListByHome(99) = %+v, want empty", other)
	}
}

func TestDeleteRemovesSlot(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	created, err := uc.Create(context.Background(), payload.CreateBlockedSlotRequest{
		HomeID: 3, StartTime: start, EndTime: start.Add(time.Hour),
	}, 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err = uc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	list, err := uc.ListByHome(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListByHome() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListByHome() after Delete = %+v, want empty", list)
	}
}
