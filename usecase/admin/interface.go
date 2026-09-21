package admin

import (
	"context"

	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
)

type IUseCase interface {
	ListAdmins(ctx context.Context) ([]presenter.AdminListItemResponse, error)
	GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error
	RevokeAdmin(ctx context.Context, userID uint) error
}
