package homeadmin

import (
	"context"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
)

type UseCase struct{ repo homerepo.IRepository }

func New(repo homerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.CreateHomeRequest) (*presenter.HomeResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category phai la 'home' hoac 'nest'")
	}
	h := &model.Home{Name: req.Name, Category: req.Category, Address: req.Address, Description: req.Description, IsActive: true}
	if err := uc.repo.Create(ctx, h); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToHomeResponse(h)
	return &resp, nil
}

func (uc *UseCase) Update(ctx context.Context, id uint, req payload.UpdateHomeRequest) (*presenter.HomeResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category phai la 'home' hoac 'nest'")
	}
	h, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperr.NotFound(err)
	}
	h.Name, h.Category, h.Address, h.Description = req.Name, req.Category, req.Address, req.Description
	h.GoogleCalendarID, h.IsActive = req.GoogleCalendarID, req.IsActive
	if err := uc.repo.Update(ctx, h); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToHomeResponse(h)
	return &resp, nil
}

func (uc *UseCase) List(ctx context.Context) ([]presenter.HomeResponse, error) {
	homes, err := uc.repo.List(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.HomeResponse, 0, len(homes))
	for _, h := range homes {
		out = append(out, presenter.ToHomeResponse(h))
	}
	return out, nil
}
