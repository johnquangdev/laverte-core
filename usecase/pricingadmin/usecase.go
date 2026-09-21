package pricingadmin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
	pricingrulerepo "github.com/johnquangdev/laverte-core/repository/pricingrule"
)

type UseCase struct{ repo pricingrulerepo.IRepository }

func New(repo pricingrulerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

// validateRule rejects every rule shape Compute cannot handle. Each check exists
// because the alternative is discovering it in a customer's price: a nil
// BaseHours makes an hourly rule unpriceable, a negative price charges a
// negative amount, and an overnight rule missing its window silently applies at
// any hour of the day like a flat day rate.
func validateRule(req payload.UpsertPricingRuleRequest) error {
	if !model.IsValidHomeCategory(req.Category) {
		return apperr.Validation("category khong hop le")
	}
	if !model.IsValidPricingRuleType(req.RuleType) {
		return apperr.Validation("rule_type phai la 'hourly', 'overnight' hoac 'day'")
	}

	positive := func(name string, v *int64) error {
		if v == nil {
			return apperr.Validation(name + " la bat buoc")
		}
		if *v <= 0 {
			return apperr.Validation(name + " phai lon hon 0")
		}
		return nil
	}

	switch req.RuleType {
	case model.PricingRuleTypeHourly:
		if req.BaseHours == nil || *req.BaseHours <= 0 {
			return apperr.Validation("base_hours phai lon hon 0")
		}
		if err := positive("base_price", req.BasePrice); err != nil {
			return err
		}
		if err := positive("extra_hour_price", req.ExtraHourPrice); err != nil {
			return err
		}
	case model.PricingRuleTypeOvernight:
		if err := positive("flat_price", req.FlatPrice); err != nil {
			return err
		}
		// Without both bounds computeFlat skips the window check entirely, so the
		// rule would quietly apply at any hour instead of only overnight.
		if req.WindowStart == nil || req.WindowEnd == nil {
			return apperr.Validation("rule overnight phai co ca window_start va window_end")
		}
		if err := model.ValidateClockWindow(*req.WindowStart, *req.WindowEnd); err != nil {
			return apperr.Validation(err.Error())
		}
	case model.PricingRuleTypeDay:
		if err := positive("flat_price", req.FlatPrice); err != nil {
			return err
		}
	}
	return nil
}

func (uc *UseCase) Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	if err := validateRule(req); err != nil {
		return nil, err
	}
	r := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: time.Now(), IsActive: true,
	}
	if err := uc.repo.Create(ctx, r); err != nil {
		if errors.Is(err, pricingrulerepo.ErrActiveRuleExists) {
			return nil, apperr.Conflict(err, "hang nay da co bang gia dang ap dung cho loai nay — dung PUT de doi gia")
		}
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToPricingRuleResponse(r)
	return &resp, nil
}

// Supersede is how a price is changed: the old rule is closed at `now` rather
// than edited, because usecase/pricing resolves rules by the booking's own
// timestamp — mutating a live rule in place would retroactively change what
// already-created bookings were priced under.
func (uc *UseCase) Supersede(ctx context.Context, id uint, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	if err := validateRule(req); err != nil {
		return nil, err
	}

	now := time.Now()
	replacement := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: now, IsActive: true,
	}
	// One transaction, in the repository: closing the old rule and inserting its
	// replacement as two independent writes means a failure between them leaves
	// the category with no active rule of that type at all, and every booking of
	// that type is refused until someone notices.
	if err := uc.repo.Supersede(ctx, id, replacement, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NotFound(err)
		}
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToPricingRuleResponse(replacement)
	return &resp, nil
}

func (uc *UseCase) ListByCategory(ctx context.Context, category string) ([]presenter.PricingRuleResponse, error) {
	rules, err := uc.repo.ListActiveByCategory(ctx, category, time.Now())
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.PricingRuleResponse, 0, len(rules))
	for _, r := range rules {
		out = append(out, presenter.ToPricingRuleResponse(r))
	}
	return out, nil
}
