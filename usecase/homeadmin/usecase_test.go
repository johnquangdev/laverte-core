package homeadmin

import (
	"context"
	"testing"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
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
