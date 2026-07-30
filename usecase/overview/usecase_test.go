package overview

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
)

type fakePaymentRepo struct {
	sum int64
	err error
}

func (f *fakePaymentRepo) Create(context.Context, *model.Payment) error { return nil }
func (f *fakePaymentRepo) GetByID(context.Context, uint) (*model.Payment, error) {
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) GetByBookingID(context.Context, uint) (*model.Payment, error) {
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }
func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return f.sum, f.err
}

type fakeBookingRepo struct {
	count int64
	err   error
}

func (f *fakeBookingRepo) Create(context.Context, *model.Booking) error { return nil }
func (f *fakeBookingRepo) GetByID(context.Context, uint) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) Update(context.Context, *model.Booking) error { return nil }
func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return f.count, f.err
}
func (f *fakeBookingRepo) MarkLockCodeAlertSent(context.Context, uint, time.Time) error {
	return nil
}
func (f *fakeBookingRepo) SetDoorLockCode(context.Context, uint, string) error { return nil }
func (f *fakeBookingRepo) ExpireIfPending(context.Context, uint) (bool, error) { return true, nil }
func (f *fakeBookingRepo) ClaimLockCodeSend(context.Context, uint, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeBookingRepo) ReleaseLockCodeSend(context.Context, uint) error { return nil }

func TestSummaryReportsRevenueAndCount(t *testing.T) {
	uc := New(&fakePaymentRepo{sum: 4500000}, &fakeBookingRepo{count: 12})
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	resp, err := uc.Summary(context.Background(), from, to)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if resp.TotalRevenueVND != 4500000 {
		t.Errorf("TotalRevenueVND = %d, want 4500000", resp.TotalRevenueVND)
	}
	if resp.BookingCount != 12 {
		t.Errorf("BookingCount = %d, want 12", resp.BookingCount)
	}
	if !resp.From.Equal(from) || !resp.To.Equal(to) {
		t.Errorf("range = %v..%v, want %v..%v", resp.From, resp.To, from, to)
	}
}

func TestSummaryRejectsNonPositiveRange(t *testing.T) {
	uc := New(&fakePaymentRepo{}, &fakeBookingRepo{})
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := uc.Summary(context.Background(), day, day)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Summary() error = %v, want CodeValidation", err)
	}
}

func TestSummarySurfacesRepoErrorAsAppErr(t *testing.T) {
	uc := New(&fakePaymentRepo{err: errors.New("db down")}, &fakeBookingRepo{})
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := uc.Summary(context.Background(), from, from.AddDate(0, 1, 0))
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeInternal {
		t.Fatalf("Summary() error = %v, want CodeInternal", err)
	}
}
