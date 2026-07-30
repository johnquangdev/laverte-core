package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

// postgresExclusionViolation is the SQLSTATE Postgres raises when an EXCLUDE
// constraint rejects an insert/update — see migrations/0004_bookings.sql.
const postgresExclusionViolation = "23P01"

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, b *model.Booking) error {
	err := r.getDB(ctx).Create(b).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresExclusionViolation {
		return ErrSlotConflict
	}
	return err
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Booking, error) {
	var b model.Booking
	err := r.getDB(ctx).First(&b, id).Error
	return &b, err
}

func (r *pgRepository) Update(ctx context.Context, b *model.Booking) error {
	err := r.getDB(ctx).Save(b).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresExclusionViolation {
		return ErrSlotConflict
	}
	return err
}

func (r *pgRepository) GetPendingByPhone(ctx context.Context, phone string) (*model.Booking, error) {
	var b model.Booking
	err := r.getDB(ctx).
		Where("customer_phone = ? AND status = ? AND expires_at > ?", phone, model.BookingStatusPendingPayment, time.Now()).
		Order("created_at DESC").First(&b).Error
	return &b, err
}

func (r *pgRepository) ListExpiredPending(ctx context.Context, now time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).Where("status = ? AND expires_at < ?", model.BookingStatusPendingPayment, now).Find(&out).Error
	return out, err
}

func (r *pgRepository) ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]*model.Booking, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("home_id = ? AND start_time < ? AND end_time > ?", homeID, end, start).
		Order("start_time ASC").Find(&out).Error
	return out, err
}

func (r *pgRepository) ListUpcomingMissingLockCode(ctx context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("status = ? AND door_lock_code IS NULL AND lock_code_alert_sent_at IS NULL AND start_time <= ? AND start_time > ?",
			model.BookingStatusConfirmed, now.Add(leadTime), now).
		Find(&out).Error
	return out, err
}

func (r *pgRepository) ListReadyToSendLockCode(ctx context.Context, now time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("status = ? AND door_lock_code IS NOT NULL AND lock_code_sent_at IS NULL AND start_time <= ?",
			model.BookingStatusConfirmed, now).
		Find(&out).Error
	return out, err
}

func (r *pgRepository) MarkLockCodeAlertSent(ctx context.Context, id uint, at time.Time) error {
	return r.getDB(ctx).Model(&model.Booking{}).
		Where("id = ?", id).
		Update("lock_code_alert_sent_at", at).Error
}

func (r *pgRepository) SetDoorLockCode(ctx context.Context, id uint, code string) error {
	return r.getDB(ctx).Model(&model.Booking{}).
		Where("id = ?", id).
		Update("door_lock_code", code).Error
}

func (r *pgRepository) ExpireIfPending(ctx context.Context, id uint) (bool, error) {
	res := r.getDB(ctx).Model(&model.Booking{}).
		Where("id = ? AND status = ?", id, model.BookingStatusPendingPayment).
		Update("status", model.BookingStatusExpired)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// The status predicate matters as much as the NULL one: a booking cancelled between
// the sweep's list query and this claim must not have its door code delivered, and
// the admin's own send races the same way between its status read and its claim.
func (r *pgRepository) ClaimLockCodeSend(ctx context.Context, id uint, at time.Time) (bool, error) {
	res := r.getDB(ctx).Model(&model.Booking{}).
		Where("id = ? AND status = ? AND lock_code_sent_at IS NULL", id, model.BookingStatusConfirmed).
		Update("lock_code_sent_at", at)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *pgRepository) ReleaseLockCodeSend(ctx context.Context, id uint) error {
	return r.getDB(ctx).Model(&model.Booking{}).
		Where("id = ?", id).
		Update("lock_code_sent_at", nil).Error
}

func (r *pgRepository) CountConfirmedBetween(ctx context.Context, from, to time.Time) (int64, error) {
	var n int64
	err := r.getDB(ctx).Model(&model.Booking{}).
		Where("status IN ? AND start_time >= ? AND start_time < ?",
			[]string{model.BookingStatusConfirmed, model.BookingStatusCompleted}, from, to).
		Count(&n).Error
	return n, err
}
