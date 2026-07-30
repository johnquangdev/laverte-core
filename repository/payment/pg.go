package payment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

// postgresUniqueViolation is the SQLSTATE Postgres raises when a unique index
// rejects a row — here, uq_payments_sepay_transaction_ref.
const postgresUniqueViolation = "23505"

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, p *model.Payment) error {
	return r.getDB(ctx).Create(p).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Payment, error) {
	var p model.Payment
	err := r.getDB(ctx).First(&p, id).Error
	return &p, err
}

func (r *pgRepository) GetByBookingID(ctx context.Context, bookingID uint) (*model.Payment, error) {
	var p model.Payment
	err := r.getDB(ctx).Where("booking_id = ?", bookingID).Order("id DESC").First(&p).Error
	return &p, err
}

// MarkPaidIfPending puts the pending-check inside the UPDATE's WHERE clause so
// two concurrent webhook deliveries cannot both observe "pending" and settle;
// Postgres serializes them on the row lock and the loser matches zero rows.
func (r *pgRepository) MarkPaidIfPending(ctx context.Context, paymentID uint, externalRef string, paidAt time.Time) (bool, error) {
	result := r.getDB(ctx).Model(&model.Payment{}).
		Where("id = ? AND status = ?", paymentID, model.PaymentStatusPending).
		Updates(map[string]any{
			"status":                model.PaymentStatusPaid,
			"sepay_transaction_ref": externalRef,
			"paid_at":               paidAt,
		})
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return false, ErrDuplicateExternalRef
		}
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *pgRepository) SumPaidBetween(ctx context.Context, from, to time.Time) (int64, error) {
	var total int64
	err := r.getDB(ctx).Model(&model.Payment{}).
		Where("status = ? AND paid_at >= ? AND paid_at < ?", model.PaymentStatusPaid, from, to).
		Select("COALESCE(SUM(amount), 0)").Scan(&total).Error
	return total, err
}

func (r *pgRepository) MarkExpiredIfPending(ctx context.Context, paymentID uint) error {
	return r.getDB(ctx).Model(&model.Payment{}).
		Where("id = ? AND status = ?", paymentID, model.PaymentStatusPending).
		Update("status", model.PaymentStatusExpired).Error
}
