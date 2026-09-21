package admin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
	userrepo "github.com/johnquangdev/laverte-core/repository/user"
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
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.AdminListItemResponse, 0, len(users))
	for _, u := range users {
		out = append(out, presenter.AdminListItemResponse{ID: u.ID, Email: u.Email, Role: u.Role})
	}
	return out, nil
}

func (uc *UseCase) GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error {
	now := time.Now()
	return uc.setRole(ctx, req.UserID, model.RoleAdmin, &grantedBy, &now)
}

func (uc *UseCase) RevokeAdmin(ctx context.Context, userID uint) error {
	return uc.setRole(ctx, userID, model.RoleUser, nil, nil)
}

// setRole turns the repository's "no such user" into a 404. Both callers are
// changes to who can administer the properties, so a mistyped id must come back as
// a failure the superadmin can see rather than as {"ok":true}.
func (uc *UseCase) setRole(ctx context.Context, userID uint, role string, grantedBy *uint, grantedAt *time.Time) error {
	if err := uc.userRepo.SetRole(ctx, userID, role, grantedBy, grantedAt); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.NotFound(err)
		}
		return apperr.Internal(err)
	}
	return nil
}
