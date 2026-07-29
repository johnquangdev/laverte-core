package user

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

func (r *pgRepository) GetByOAuth(ctx context.Context, provider, oauthID string) (*model.User, error) {
	var u model.User
	err := r.getDB(ctx).Where("oauth_provider = ? AND oauth_id = ?", provider, oauthID).First(&u).Error
	return &u, err
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	err := r.getDB(ctx).First(&u, id).Error
	return &u, err
}

func (r *pgRepository) Create(ctx context.Context, u *model.User) error {
	return r.getDB(ctx).Create(u).Error
}

func (r *pgRepository) ListByRole(ctx context.Context, role string) ([]*model.User, error) {
	var users []*model.User
	err := r.getDB(ctx).Where("role = ?", role).Order("admin_granted_at ASC NULLS FIRST, id ASC").Find(&users).Error
	return users, err
}

func (r *pgRepository) SetRole(ctx context.Context, userID uint, role string, grantedBy *uint, grantedAt *time.Time) error {
	return r.getDB(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"role": role, "admin_granted_by": grantedBy, "admin_granted_at": grantedAt,
	}).Error
}
