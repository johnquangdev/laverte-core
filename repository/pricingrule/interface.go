package pricingrule

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, r *model.PricingRule) error
	// Supersede closes rule oldID at `at` and inserts replacement, both in one
	// transaction. Split across two calls, a failure between them leaves the
	// category with no active rule and refuses every booking of that type.
	// Returns gorm.ErrRecordNotFound if oldID does not exist.
	Supersede(ctx context.Context, oldID uint, replacement *model.PricingRule, at time.Time) error
	GetByID(ctx context.Context, id uint) (*model.PricingRule, error)
	// ListActiveByCategory returns is_active rules for category whose
	// [EffectiveFrom, EffectiveTo) window contains at.
	ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error)
}
