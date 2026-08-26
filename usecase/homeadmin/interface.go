package homeadmin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.CreateHomeRequest) (*presenter.HomeResponse, error)
	Update(ctx context.Context, id uint, req payload.UpdateHomeRequest) (*presenter.HomeResponse, error)
	List(ctx context.Context) ([]presenter.HomeResponse, error)
	ListActive(ctx context.Context) ([]presenter.PublicHomeResponse, error)
}
