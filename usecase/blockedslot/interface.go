package blockedslot

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.CreateBlockedSlotRequest, adminID uint) (*presenter.BlockedSlotResponse, error)
	Delete(ctx context.Context, id uint) error
	ListByHome(ctx context.Context, homeID uint) ([]presenter.BlockedSlotResponse, error)
}
