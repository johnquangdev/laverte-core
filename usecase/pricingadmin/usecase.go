package pricingadmin

import (
	"context"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
)

type UseCase struct{ repo pricingrulerepo.IRepository }

func New(repo pricingrulerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category khong hop le")
	}
	r := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: time.Now(), IsActive: true,
	}
	if err := uc.repo.Create(ctx, r); err != nil {
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
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category khong hop le")
	}
	old, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperr.NotFound(err)
	}

	now := time.Now()
	old.EffectiveTo = &now
	old.IsActive = false
	if err := uc.repo.Update(ctx, old); err != nil {
		return nil, apperr.Internal(err)
	}

	replacement := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: now, IsActive: true,
	}
	if err := uc.repo.Create(ctx, replacement); err != nil {
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
