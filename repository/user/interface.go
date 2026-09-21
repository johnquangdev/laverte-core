package user

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

type IRepository interface {
	GetByOAuth(ctx context.Context, provider, oauthID string) (*model.User, error)
	GetByID(ctx context.Context, id uint) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	ListByRole(ctx context.Context, role string) ([]*model.User, error)
	SetRole(ctx context.Context, userID uint, role string, grantedBy *uint, grantedAt *time.Time) error
}
