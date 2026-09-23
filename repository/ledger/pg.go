package ledger

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

const paymentRowColumns = "payments.*, bookings.customer_name, bookings.customer_phone, " +
	"bookings.home_id, bookings.status AS booking_status"

func (r *pgRepository) paymentRows(ctx context.Context) *gorm.DB {
	return r.getDB(ctx).Table("payments").
		Select(paymentRowColumns).
		Joins("JOIN bookings ON bookings.id = payments.booking_id")
}

func (r *pgRepository) ListPayments(ctx context.Context, from, to time.Time, limit int) ([]*PaymentRow, error) {
	var out []*PaymentRow
	err := r.paymentRows(ctx).
		Where("payments.created_at >= ? AND payments.created_at < ?", from, to).
		Order("payments.created_at DESC, payments.id DESC").
		Limit(limit).
		Scan(&out).Error
	return out, err
}

func (r *pgRepository) GetPayment(ctx context.Context, id uint) (*PaymentRow, error) {
	var row PaymentRow
	res := r.paymentRows(ctx).Where("payments.id = ?", id).Limit(1).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	// Scan, unlike First, reports a missing row as zero rows rather than an error.
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (r *pgRepository) RefundIfStayEnded(ctx context.Context, paymentID, adminID uint, note string, at time.Time) (bool, error) {
	res := r.getDB(ctx).Model(&model.Payment{}).
		Where("id = ? AND status = ?", paymentID, model.PaymentStatusPaid).
		Where("booking_id IN (?)", r.getDB(ctx).Model(&model.Booking{}).Select("id").
			Where("status IN ?", []string{model.BookingStatusCancelled, model.BookingStatusExpired})).
		Updates(map[string]any{
			"status":               model.PaymentStatusRefunded,
			"refunded_at":          at,
			"refunded_by_admin_id": adminID,
			"refund_note":          note,
		})
	return res.RowsAffected > 0, res.Error
}
