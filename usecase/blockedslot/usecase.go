package blockedslot

import (
	"context"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
)

type UseCase struct{ repo blockedslotrepo.IRepository }

func New(repo blockedslotrepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.CreateBlockedSlotRequest, adminID uint) (*presenter.BlockedSlotResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	s := &model.BlockedSlot{
		HomeID: req.HomeID, StartTime: req.StartTime, EndTime: req.EndTime,
		Reason: req.Reason, CreatedByAdminID: &adminID,
	}
	if err := uc.repo.Create(ctx, s); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToBlockedSlotResponse(s)
	return &resp, nil
}

func (uc *UseCase) Delete(ctx context.Context, id uint) error {
	if err := uc.repo.Delete(ctx, id); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (uc *UseCase) ListByHome(ctx context.Context, homeID uint) ([]presenter.BlockedSlotResponse, error) {
	slots, err := uc.repo.ListByHome(ctx, homeID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.BlockedSlotResponse, 0, len(slots))
	for _, s := range slots {
		out = append(out, presenter.ToBlockedSlotResponse(s))
	}
	return out, nil
}
