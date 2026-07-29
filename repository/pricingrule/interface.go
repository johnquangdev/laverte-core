package pricingrule

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, r *model.PricingRule) error
	Update(ctx context.Context, r *model.PricingRule) error
	GetByID(ctx context.Context, id uint) (*model.PricingRule, error)
	// ListActiveByCategory returns is_active rules for category whose
	// [EffectiveFrom, EffectiveTo) window contains at.
	ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error)
}
