package unmatchedtransfer

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/johnquangdev/laverte-core/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) RecordIfNew(ctx context.Context, t *model.UnmatchedTransfer) (bool, error) {
	res := r.getDB(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "sepay_transaction_ref"}}, DoNothing: true}).
		Create(t)
	return res.RowsAffected > 0, res.Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.UnmatchedTransfer, error) {
	var t model.UnmatchedTransfer
	err := r.getDB(ctx).First(&t, id).Error
	return &t, err
}

func (r *pgRepository) List(ctx context.Context, openOnly bool, limit int) ([]*model.UnmatchedTransfer, error) {
	q := r.getDB(ctx).Order("received_at DESC, id DESC").Limit(limit)
	if openOnly {
		q = q.Where("resolved_at IS NULL")
	}
	var out []*model.UnmatchedTransfer
	err := q.Find(&out).Error
	return out, err
}

func (r *pgRepository) ResolveIfOpen(ctx context.Context, id, adminID uint, note string, at time.Time) (bool, error) {
	res := r.getDB(ctx).Model(&model.UnmatchedTransfer{}).
		Where("id = ? AND resolved_at IS NULL", id).
		Updates(map[string]any{
			"resolved_at":          at,
			"resolved_by_admin_id": adminID,
			"resolution_note":      note,
		})
	return res.RowsAffected > 0, res.Error
}
