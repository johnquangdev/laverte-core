package bookingadmin

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]presenter.AdminBookingResponse, error)
	CreateWalkIn(ctx context.Context, req payload.CreateWalkInBookingRequest, adminID uint) (*presenter.AdminBookingResponse, error)
	Cancel(ctx context.Context, id uint) error
	Complete(ctx context.Context, id uint) error
	NoShow(ctx context.Context, id uint) error
	SetLockCode(ctx context.Context, id uint, code string) error
	SendLockCode(ctx context.Context, id uint) error
}
