package pricingadmin

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
)

type fakeRepo struct {
	byID    map[uint]*model.PricingRule
	created []*model.PricingRule
	nextID  uint
	// createErr, when set, is what Create returns instead of succeeding — used to
	// simulate the partial unique index rejecting a duplicate active rule.
	createErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uint]*model.PricingRule{}}
}

func (f *fakeRepo) Create(_ context.Context, r *model.PricingRule) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.nextID++
	r.ID = f.nextID
	f.byID[r.ID] = r
	f.created = append(f.created, r)
	return nil
}

// Supersede mirrors the pg repository's transaction: it refuses an unknown or
// already-superseded id instead of inserting a replacement that would leave
// two active rules for the category.
func (f *fakeRepo) Supersede(_ context.Context, oldID uint, replacement *model.PricingRule, at time.Time) error {
	old, ok := f.byID[oldID]
	if !ok || !old.IsActive {
		return gorm.ErrRecordNotFound
	}
	old.EffectiveTo = &at
	old.IsActive = false

	f.nextID++
	replacement.ID = f.nextID
	f.byID[replacement.ID] = replacement
	f.created = append(f.created, replacement)
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

// A negative extra_hour_price would make a long stay cheaper than a short one
// and eventually price a booking below zero; it must never reach the table.
func TestCreateRejectsNegativeExtraHourPrice(t *testing.T) {
	req := hourlyReq(200000)
	neg := int64(-1000)
	req.ExtraHourPrice = &neg
	if _, err := New(newFakeRepo()).Create(context.Background(), req); err == nil {
		t.Fatal("Create() with negative extra_hour_price = nil error, want error")
	}
}

// Without both window bounds, computeFlat skips the window check entirely and
// the rule would apply at any hour like a flat day rate.
func TestCreateRejectsOvernightWithoutWindow(t *testing.T) {
	req := payload.UpsertPricingRuleRequest{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000),
	}
	if _, err := New(newFakeRepo()).Create(context.Background(), req); err == nil {
		t.Fatal("Create() with overnight rule missing its window = nil error, want error")
	}
}

// A duplicate Create is a caller mistake the admin can fix, so it must answer 409
// with an actionable message — not the 500 a raw unique-violation would produce.
func TestCreateDuplicateActiveRuleIsConflict(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = pricingrulerepo.ErrActiveRuleExists
	uc := New(repo)

	_, err := uc.Create(context.Background(), hourlyReq(200000))
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("error = %v, want an *apperr.Error", err)
	}
	if e.HTTPCode != http.StatusConflict {
		t.Errorf("HTTPCode = %d, want 409", e.HTTPCode)
	}
}
