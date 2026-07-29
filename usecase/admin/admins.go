package admin

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
)

type UseCase struct {
	userRepo userrepo.IRepository
}

func New(userRepo userrepo.IRepository) IUseCase {
	return &UseCase{userRepo: userRepo}
}

func (uc *UseCase) ListAdmins(ctx context.Context) ([]presenter.AdminListItemResponse, error) {
	users, err := uc.userRepo.ListByRole(ctx, model.RoleAdmin)
	if err != nil {
		return nil, err
	}
	out := make([]presenter.AdminListItemResponse, 0, len(users))
	for _, u := range users {
		out = append(out, presenter.AdminListItemResponse{ID: u.ID, Email: u.Email, Role: u.Role})
	}
	return out, nil
}

func (uc *UseCase) GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error {
	now := time.Now()
	return uc.userRepo.SetRole(ctx, req.UserID, model.RoleAdmin, &grantedBy, &now)
}

func (uc *UseCase) RevokeAdmin(ctx context.Context, userID uint) error {
	return uc.userRepo.SetRole(ctx, userID, model.RoleUser, nil, nil)
}
