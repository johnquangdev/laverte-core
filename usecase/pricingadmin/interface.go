package pricingadmin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	// Supersede closes rule `id` and inserts req as its replacement, so a price
	// change never rewrites the rule a past booking was charged under.
	Supersede(ctx context.Context, id uint, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	ListByCategory(ctx context.Context, category string) ([]presenter.PricingRuleResponse, error)
}
