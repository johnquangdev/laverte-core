package refreshtoken

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, t *model.RefreshToken) error {
	return r.getDB(ctx).Create(t).Error
}

func (r *pgRepository) GetByTokenID(ctx context.Context, tokenID string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.getDB(ctx).Where("token_id = ?", tokenID).First(&t).Error
	return &t, err
}

func (r *pgRepository) Revoke(ctx context.Context, tokenID string) error {
	return r.getDB(ctx).Model(&model.RefreshToken{}).Where("token_id = ?", tokenID).
		Update("revoked_at", time.Now()).Error
}

func (r *pgRepository) RevokeFamily(ctx context.Context, familyID string) error {
	return r.getDB(ctx).Model(&model.RefreshToken{}).Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", time.Now()).Error
}
