package admin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	ListAdmins(ctx context.Context) ([]presenter.AdminListItemResponse, error)
	GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error
	RevokeAdmin(ctx context.Context, userID uint) error
}
