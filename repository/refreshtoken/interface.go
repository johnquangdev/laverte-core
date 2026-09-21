package refreshtoken

import (
	"context"

	"github.com/johnquangdev/laverte-core/model"
)

type IRepository interface {
	Create(ctx context.Context, t *model.RefreshToken) error
	GetByTokenID(ctx context.Context, tokenID string) (*model.RefreshToken, error)
	Revoke(ctx context.Context, tokenID string) error
	RevokeFamily(ctx context.Context, familyID string) error
}
