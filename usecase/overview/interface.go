package overview

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/presenter"
)

type IUseCase interface {
	Summary(ctx context.Context, from, to time.Time) (*presenter.OverviewResponse, error)
}
