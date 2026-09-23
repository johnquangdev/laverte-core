package overview

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/presenter"
)

type IUseCase interface {
	Summary(ctx context.Context, from, to time.Time) (*presenter.OverviewResponse, error)
	// Breakdown details the calendar month containing month, in the business zone.
	Breakdown(ctx context.Context, month time.Time) (*presenter.OverviewBreakdownResponse, error)
}
