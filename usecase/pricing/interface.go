package pricing

import (
	"context"
	"time"
)

type IUseCase interface {
	// Compute returns the price in VND for a booking of bookingType in
	// category, spanning [start, end), evaluated against rules effective at
	// `at` (normally start).
	Compute(ctx context.Context, category, bookingType string, start, end, at time.Time) (int64, error)
}
