package pricingrule

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, rule *model.PricingRule) error {
	return r.getDB(ctx).Create(rule).Error
}

func (r *pgRepository) Supersede(ctx context.Context, oldID uint, replacement *model.PricingRule, at time.Time) error {
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.PricingRule{}).
			Where("id = ? AND is_active = true", oldID).
			Updates(map[string]any{"effective_to": at, "is_active": false})
		if res.Error != nil {
			return res.Error
		}
		// Zero rows means the id is unknown or already superseded; inserting the
		// replacement anyway would leave two active rules for the category.
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(replacement).Error
	})
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.PricingRule, error) {
	var rule model.PricingRule
	err := r.getDB(ctx).First(&rule, id).Error
	return &rule, err
}

func (r *pgRepository) ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error) {
	var rules []*model.PricingRule
	// Ordered newest-first and deterministic: without it Postgres may return two
	// overlapping active rules in either order, and Compute takes the first match
	// — so the same booking could be quoted a different price on each request.
	err := r.getDB(ctx).
		Where("category = ? AND is_active = true AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", category, at, at).
		Order("effective_from DESC, id DESC").
		Find(&rules).Error
	return rules, err
}
