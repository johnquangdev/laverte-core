package home

import (
	"context"

	"github.com/johnquangdev/laverte-core/model"
)

type IRepository interface {
	Create(ctx context.Context, h *model.Home) error
	Update(ctx context.Context, h *model.Home) error
	GetByID(ctx context.Context, id uint) (*model.Home, error)
	List(ctx context.Context) ([]*model.Home, error)
}
