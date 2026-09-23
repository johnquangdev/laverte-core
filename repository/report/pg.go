package report

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

var occupyingStatuses = []string{model.BookingStatusConfirmed, model.BookingStatusCompleted}

func (r *pgRepository) MonthlyRevenue(ctx context.Context, from, to time.Time, tz string) ([]MonthAmount, error) {
	var out []MonthAmount
	err := r.getDB(ctx).Raw(`
		SELECT to_char(paid_at AT TIME ZONE ?, 'YYYY-MM') AS month, SUM(amount) AS amount
		FROM payments
		WHERE status = ? AND paid_at >= ? AND paid_at < ?
		GROUP BY 1`,
		tz, model.PaymentStatusPaid, from, to).Scan(&out).Error
	return out, err
}

func (r *pgRepository) MonthlyBookingsByType(ctx context.Context, from, to time.Time, tz string) ([]MonthTypeCount, error) {
	var out []MonthTypeCount
	err := r.getDB(ctx).Raw(`
		SELECT to_char(start_time AT TIME ZONE ?, 'YYYY-MM') AS month, booking_type, COUNT(*) AS count
		FROM bookings
		WHERE status IN ? AND start_time >= ? AND start_time < ?
		GROUP BY 1, 2`,
		tz, occupyingStatuses, from, to).Scan(&out).Error
	return out, err
}

func (r *pgRepository) OccupyingBookings(ctx context.Context, from, to time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("status IN ? AND start_time < ? AND end_time > ?", occupyingStatuses, to, from).
		Order("start_time ASC").
		Find(&out).Error
	return out, err
}

func (r *pgRepository) RevenueByHome(ctx context.Context, from, to time.Time) ([]HomeAmount, error) {
	var out []HomeAmount
	err := r.getDB(ctx).Raw(`
		SELECT bookings.home_id, SUM(payments.amount) AS amount
		FROM payments JOIN bookings ON bookings.id = payments.booking_id
		WHERE payments.status = ? AND payments.paid_at >= ? AND payments.paid_at < ?
		GROUP BY 1`,
		model.PaymentStatusPaid, from, to).Scan(&out).Error
	return out, err
}
