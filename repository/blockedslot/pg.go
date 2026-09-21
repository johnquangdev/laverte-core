package blockedslot

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, s *model.BlockedSlot) error {
	return r.getDB(ctx).Create(s).Error
}

func (r *pgRepository) Delete(ctx context.Context, id uint) error {
	res := r.getDB(ctx).Delete(&model.BlockedSlot{}, id)
	if res.Error != nil {
		return res.Error
	}
	// Reporting success for an id that was never there would tell the admin the
	// home is bookable again while HasOverlap — which has no DB constraint behind
	// it — goes on refusing bookings for the window they meant to clear.
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *pgRepository) ListByHome(ctx context.Context, homeID uint) ([]*model.BlockedSlot, error) {
	var out []*model.BlockedSlot
	err := r.getDB(ctx).Where("home_id = ?", homeID).Order("start_time ASC").Find(&out).Error
	return out, err
}

func (r *pgRepository) HasOverlap(ctx context.Context, homeID uint, start, end time.Time) (bool, error) {
	var count int64
	err := r.getDB(ctx).Model(&model.BlockedSlot{}).
		Where("home_id = ? AND start_time < ? AND end_time > ?", homeID, end, start).
		Count(&count).Error
	return count > 0, err
}
