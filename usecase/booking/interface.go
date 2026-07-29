package booking

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

// IUseCase grows across later tasks (guest lookup, admin walk-in, cancel,
// lock-code) — add methods here instead of introducing a parallel interface.
type IUseCase interface {
	Create(ctx context.Context, req payload.CreateBookingRequest) (*presenter.BookingResponse, error)
}
