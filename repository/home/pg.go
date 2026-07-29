package home

import (
	"context"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, h *model.Home) error {
	return r.getDB(ctx).Create(h).Error
}

func (r *pgRepository) Update(ctx context.Context, h *model.Home) error {
	return r.getDB(ctx).Save(h).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Home, error) {
	var h model.Home
	err := r.getDB(ctx).First(&h, id).Error
	return &h, err
}

func (r *pgRepository) List(ctx context.Context) ([]*model.Home, error) {
	var homes []*model.Home
	err := r.getDB(ctx).Order("id ASC").Find(&homes).Error
	return homes, err
}
