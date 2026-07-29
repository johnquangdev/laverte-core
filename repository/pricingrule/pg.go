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

func (r *pgRepository) Update(ctx context.Context, rule *model.PricingRule) error {
	return r.getDB(ctx).Save(rule).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.PricingRule, error) {
	var rule model.PricingRule
	err := r.getDB(ctx).First(&rule, id).Error
	return &rule, err
}

func (r *pgRepository) ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error) {
	var rules []*model.PricingRule
	err := r.getDB(ctx).
		Where("category = ? AND is_active = true AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", category, at, at).
		Find(&rules).Error
	return rules, err
}
