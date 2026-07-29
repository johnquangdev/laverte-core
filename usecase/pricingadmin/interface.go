package pricingadmin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	// Create validates the whole rule before storing it. A rule that Compute
	// cannot price — or that would price to a negative amount — must never
	// reach the table, because the next thing to read it is a customer's quote.
	Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	// Supersede closes rule `id` and inserts req as its replacement, so a price
	// change never rewrites the rule a past booking was charged under.
	Supersede(ctx context.Context, id uint, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	ListByCategory(ctx context.Context, category string) ([]presenter.PricingRuleResponse, error)
}
