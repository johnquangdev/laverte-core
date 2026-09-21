package homeadmin

import (
	"context"
	"net/http"
	"testing"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
)

type fakeRepo struct {
	homes  map[uint]*model.Home
	nextID uint
}

func newFakeRepo() *fakeRepo { return &fakeRepo{homes: map[uint]*model.Home{}} }

func (f *fakeRepo) Create(_ context.Context, h *model.Home) error {
	f.nextID++
	h.ID = f.nextID
	f.homes[h.ID] = h
	return nil
}
func (f *fakeRepo) Update(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	h, ok := f.homes[id]
	if !ok {
		return nil, apperr.NotFound(nil)
	}
	return h, nil
}
func (f *fakeRepo) List(_ context.Context) ([]*model.Home, error) {
	out := make([]*model.Home, 0, len(f.homes))
	for _, h := range f.homes {
		out = append(out, h)
	}
	return out, nil
}

func TestCreateRejectsInvalidCategory(t *testing.T) {
	uc := New(newFakeRepo())
	_, err := uc.Create(context.Background(), payload.CreateHomeRequest{Name: "A", Category: "villa"})
	if err == nil {
		t.Fatal("Create() with invalid category = nil error, want error")
	}
}

func TestCreateAndListRoundTrip(t *testing.T) {
	uc := New(newFakeRepo())
	created, err := uc.Create(context.Background(), payload.CreateHomeRequest{Name: "Nest 1", Category: model.HomeCategoryNest})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	list, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("List() = %+v, want one item with ID %d", list, created.ID)
	}
}

func boolp(v bool) *bool    { return &v }
func strp(v string) *string { return &v }

// The trap this guards: an admin editing one text field sends no is_active and no
// google_calendar_id, and a plain-value payload would read those omissions as
// "false" and "" — silently pulling the home out of service and dropping its
// calendar link. Omitted must mean unchanged.
func TestUpdateLeavesOmittedFlagsUntouched(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	created, err := uc.Create(ctx, payload.CreateHomeRequest{Name: "Home 1", Category: model.HomeCategoryHome})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	repo.homes[created.ID].GoogleCalendarID = "cal-abc"
	repo.homes[created.ID].IsActive = true

	got, err := uc.Update(ctx, created.ID, payload.UpdateHomeRequest{
		Name: "Home 1", Category: model.HomeCategoryHome, Address: "12 Nguyen Trai",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !got.IsActive {
		t.Error("Update() deactivated the home even though is_active was omitted")
	}
	if got.GoogleCalendarID != "cal-abc" {
		t.Errorf("GoogleCalendarID = %q, want it left at cal-abc", got.GoogleCalendarID)
	}
	if got.Address != "12 Nguyen Trai" {
		t.Errorf("Address = %q, want the update applied", got.Address)
	}
}

// A present-but-false is_active is a real instruction and must be applied.
func TestUpdateAppliesExplicitFlags(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	created, err := uc.Create(ctx, payload.CreateHomeRequest{Name: "Nest 2", Category: model.HomeCategoryNest})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := uc.Update(ctx, created.ID, payload.UpdateHomeRequest{
		Name: "Nest 2", Category: model.HomeCategoryNest,
		IsActive: boolp(false), GoogleCalendarID: strp("cal-xyz"),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.IsActive {
		t.Error("Update() ignored an explicit is_active=false")
	}
	if got.GoogleCalendarID != "cal-xyz" {
		t.Errorf("GoogleCalendarID = %q, want cal-xyz", got.GoogleCalendarID)
	}
}

func TestUpdateUnknownHomeIsNotFound(t *testing.T) {
	uc := New(newFakeRepo())
	_, err := uc.Update(context.Background(), 999, payload.UpdateHomeRequest{
		Name: "X", Category: model.HomeCategoryHome,
	})
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("error = %v, want an *apperr.Error", err)
	}
	if e.HTTPCode != http.StatusNotFound {
		t.Errorf("HTTPCode = %d, want 404", e.HTTPCode)
	}
}

func TestUpdateRejectsInvalidCategory(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	created, err := uc.Create(ctx, payload.CreateHomeRequest{Name: "Home 3", Category: model.HomeCategoryHome})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := uc.Update(ctx, created.ID, payload.UpdateHomeRequest{Name: "Home 3", Category: "villa"}); err == nil {
		t.Fatal("Update() with an invalid category = nil error, want error")
	}
	if repo.homes[created.ID].Name != "Home 3" {
		t.Error("a rejected Update must not have written anything")
	}
}
