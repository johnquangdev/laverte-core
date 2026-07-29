package blockedslot

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, s *model.BlockedSlot) error
	Delete(ctx context.Context, id uint) error
	ListByHome(ctx context.Context, homeID uint) ([]*model.BlockedSlot, error)
	// HasOverlap treats both ranges as half-open [start, end), so a block that
	// ends exactly when a booking starts is not a conflict.
	HasOverlap(ctx context.Context, homeID uint, start, end time.Time) (bool, error)
}
