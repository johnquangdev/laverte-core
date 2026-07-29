package pricingadmin

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
)

type fakeRepo struct {
	byID    map[uint]*model.PricingRule
	created []*model.PricingRule
	nextID  uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uint]*model.PricingRule{}}
}

func (f *fakeRepo) Create(_ context.Context, r *model.PricingRule) error {
	f.nextID++
	r.ID = f.nextID
	f.byID[r.ID] = r
	f.created = append(f.created, r)
	return nil
}

func (f *fakeRepo) Update(_ context.Context, r *model.PricingRule) error {
	f.byID[r.ID] = r
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uint) (*model.PricingRule, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return r, nil
}

func (f *fakeRepo) ListActiveByCategory(_ context.Context, category string, _ time.Time) ([]*model.PricingRule, error) {
	var out []*model.PricingRule
	for _, r := range f.byID {
		if r.Category == category && r.IsActive {
			out = append(out, r)
		}
	}
	return out, nil
}

func int64p(v int64) *int64 { return &v }
func intp(v int) *int       { return &v }

func hourlyReq(base int64) payload.UpsertPricingRuleRequest {
	return payload.UpsertPricingRuleRequest{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(base), ExtraHourPrice: int64p(50000),
	}
}

func TestSupersedeClosesOldRuleAndCreatesReplacement(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	original, err := uc.Create(ctx, hourlyReq(200000))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	replacement, err := uc.Supersede(ctx, original.ID, hourlyReq(250000))
	if err != nil {
		t.Fatalf("Supersede() error = %v", err)
	}

	if replacement.ID == original.ID {
		t.Error("Supersede() reused the old row; a replacement must be a new rule")
	}
	if *replacement.BasePrice != 250000 {
		t.Errorf("replacement BasePrice = %d, want 250000", *replacement.BasePrice)
	}

	// The old rule must survive as history, closed rather than rewritten — the
	// price a past booking was charged under has to stay reconstructible.
	old := repo.byID[original.ID]
	if old.IsActive {
		t.Error("old rule still active after Supersede")
	}
	if old.EffectiveTo == nil {
		t.Error("old rule EffectiveTo not set after Supersede")
	}
	if *old.BasePrice != 200000 {
		t.Errorf("old rule BasePrice = %d, want it left at 200000", *old.BasePrice)
	}
}

func TestSupersedeUnknownRuleRejected(t *testing.T) {
	uc := New(newFakeRepo())
	if _, err := uc.Supersede(context.Background(), 999, hourlyReq(250000)); err == nil {
		t.Fatal("Supersede() on a missing rule = nil error, want error")
	}
}

func TestSupersedeRejectsInvalidCategory(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	original, err := uc.Create(ctx, hourlyReq(200000))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	bad := hourlyReq(250000)
	bad.Category = "villa"
	if _, err := uc.Supersede(ctx, original.ID, bad); err == nil {
		t.Fatal("Supersede() with invalid category = nil error, want error")
	}
	if !repo.byID[original.ID].IsActive {
		t.Error("a rejected Supersede must not have closed the existing rule")
	}
}
