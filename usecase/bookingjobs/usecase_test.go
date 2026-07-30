package bookingjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type fakeBookingRepo struct {
	rows       map[uint]*model.Booking
	nextID     uint
	updateErrs map[uint]error

	updateCalls  int
	setCodeCalls int
	claimCalls   int
	releaseCalls int
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{rows: map[uint]*model.Booking{}, updateErrs: map[uint]error{}}
}

func (f *fakeBookingRepo) seed(b *model.Booking) *model.Booking {
	f.nextID++
	b.ID = f.nextID
	f.rows[b.ID] = b
	return b
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error { f.seed(b); return nil }
func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	b, ok := f.rows[id]
	if !ok {
		return nil, errors.New("booking not found")
	}
	return b, nil
}
func (f *fakeBookingRepo) Update(_ context.Context, b *model.Booking) error {
	f.updateCalls++
	if err := f.updateErrs[b.ID]; err != nil {
		return err
	}
	f.rows[b.ID] = b
	return nil
}
func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) ListExpiredPending(_ context.Context, now time.Time) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusPendingPayment {
			continue
		}
		if b.ExpiresAt != nil && b.ExpiresAt.Before(now) {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

// ListUpcomingMissingLockCode mirrors the real query's filters — including the
// lock_code_alert_sent_at IS NULL clause the alert sweep relies on.
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(_ context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusConfirmed {
			continue
		}
		if b.DoorLockCode != nil || b.LockCodeAlertSentAt != nil {
			continue
		}
		if b.StartTime.After(now) && !b.StartTime.After(now.Add(leadTime)) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBookingRepo) ListReadyToSendLockCode(_ context.Context, now time.Time) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusConfirmed || b.LockCodeSentAt != nil {
			continue
		}
		if !b.StartTime.After(now) {
			out = append(out, b)
		}
	}
	return out, nil
}

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18; the fake
// carries it from the start so this file needs no edit then.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeBookingRepo) MarkLockCodeAlertSent(_ context.Context, id uint, at time.Time) error {
	if b, ok := f.rows[id]; ok {
		b.LockCodeAlertSentAt = &at
	}
	return nil
}

func (f *fakeBookingRepo) SetDoorLockCode(_ context.Context, id uint, code string) error {
	if b, ok := f.rows[id]; ok {
		b.DoorLockCode = &code
	}
	f.setCodeCalls++
	return nil
}

// ClaimLockCodeSend mirrors the SQL: stamp only when lock_code_sent_at is still
// NULL, and report whether this caller won the claim.
func (f *fakeBookingRepo) ClaimLockCodeSend(_ context.Context, id uint, at time.Time) (bool, error) {
	b, ok := f.rows[id]
	if !ok || b.LockCodeSentAt != nil {
		return false, nil
	}
	b.LockCodeSentAt = &at
	f.claimCalls++
	return true, nil
}

func (f *fakeBookingRepo) ReleaseLockCodeSend(_ context.Context, id uint) error {
	if b, ok := f.rows[id]; ok {
		b.LockCodeSentAt = nil
	}
	f.releaseCalls++
	return nil
}

type fakePaymentRepo struct {
	rows   map[uint]*model.Payment
	nextID uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{rows: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) seed(p *model.Payment) *model.Payment {
	f.nextID++
	p.ID = f.nextID
	f.rows[p.ID] = p
	return p
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error { f.seed(p); return nil }
func (f *fakePaymentRepo) GetByID(_ context.Context, id uint) (*model.Payment, error) {
	p, ok := f.rows[id]
	if !ok {
		return nil, errors.New("payment not found")
	}
	return p, nil
}
func (f *fakePaymentRepo) GetByBookingID(_ context.Context, bookingID uint) (*model.Payment, error) {
	for _, p := range f.rows {
		if p.BookingID == bookingID {
			return p, nil
		}
	}
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakePaymentRepo) MarkExpiredIfPending(_ context.Context, paymentID uint) error {
	p, ok := f.rows[paymentID]
	if !ok {
		return errors.New("payment not found")
	}
	if p.Status == model.PaymentStatusPending {
		p.Status = model.PaymentStatusExpired
	}
	return nil
}

type fakeNotifier struct {
	lockCodeCalls int
	lockCodeErr   error
	alertCalls    int
}

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error {
	f.lockCodeCalls++
	return f.lockCodeErr
}
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	f.alertCalls++
	return nil
}

func newTestUseCase() (*UseCase, *fakeBookingRepo, *fakePaymentRepo, *fakeNotifier) {
	bookings := newFakeBookingRepo()
	payments := newFakePaymentRepo()
	notifier := &fakeNotifier{}
	cfg := config.Config{BookingCheckinAlertLeadMinutes: 30}
	uc := New(bookings, payments, notifier, zap.NewNop(), cfg).(*UseCase)
	return uc, bookings, payments, notifier
}

func TestExpirePendingBookingsExpiresBookingAndPayment(t *testing.T) {
	uc, bookings, payments, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	b := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	p := payments.seed(&model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Status: model.PaymentStatusPending, Amount: 300000})
	b.PaymentID = &p.ID

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v", err)
	}
	if b.Status != model.BookingStatusExpired {
		t.Errorf("booking Status = %q, want %q", b.Status, model.BookingStatusExpired)
	}
	if p.Status != model.PaymentStatusExpired {
		t.Errorf("payment Status = %q, want %q", p.Status, model.PaymentStatusExpired)
	}
}

func TestExpirePendingBookingsLeavesPaidPaymentAlone(t *testing.T) {
	uc, bookings, payments, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	paidAt := time.Now()
	b := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	p := payments.seed(&model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Status: model.PaymentStatusPaid, Amount: 300000, PaidAt: &paidAt})
	b.PaymentID = &p.ID

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v", err)
	}
	if p.Status != model.PaymentStatusPaid {
		t.Errorf("payment Status = %q, want %q", p.Status, model.PaymentStatusPaid)
	}
}

func TestExpirePendingBookingsContinuesAfterRowFailure(t *testing.T) {
	uc, bookings, _, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	broken := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	good := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	bookings.updateErrs[broken.ID] = errors.New("write conflict")

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v, want nil (per-row failures are logged)", err)
	}
	if good.Status != model.BookingStatusExpired {
		t.Errorf("second booking Status = %q, want %q — a failed row must not abort the sweep", good.Status, model.BookingStatusExpired)
	}
}

func TestAlertMissingLockCodesAlertsOnceOnly(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(10 * time.Minute),
	})

	if err := uc.AlertMissingLockCodes(context.Background()); err != nil {
		t.Fatalf("first AlertMissingLockCodes() error = %v", err)
	}
	if bookings.rows[1].LockCodeAlertSentAt == nil {
		t.Fatal("LockCodeAlertSentAt = nil after alert, want a timestamp")
	}
	if err := uc.AlertMissingLockCodes(context.Background()); err != nil {
		t.Fatalf("second AlertMissingLockCodes() error = %v", err)
	}
	if notifier.alertCalls != 1 {
		t.Errorf("AdminLockCodeMissing calls = %d, want 1", notifier.alertCalls)
	}
}

func TestSendDueLockCodesSendsAndMarks(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	code := "1234"
	due := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Minute), DoorLockCode: &code,
	})
	noCode := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Minute),
	})

	if err := uc.SendDueLockCodes(context.Background()); err != nil {
		t.Fatalf("SendDueLockCodes() error = %v", err)
	}
	if notifier.lockCodeCalls != 1 {
		t.Errorf("LockCode calls = %d, want 1", notifier.lockCodeCalls)
	}
	if due.LockCodeSentAt == nil {
		t.Error("LockCodeSentAt = nil for the due booking, want a timestamp")
	}
	if noCode.LockCodeSentAt != nil {
		t.Error("LockCodeSentAt set for a booking with no code, want nil")
	}
	if bookings.claimCalls != 1 || bookings.updateCalls != 0 {
		t.Errorf("claimCalls=%d updateCalls=%d, want 1/0: the send must be gated by the DB claim, not by a full-row Save",
			bookings.claimCalls, bookings.updateCalls)
	}
	// A second claim on the same row must still lose: this is the only thing that
	// stops a concurrent admin "send now" click and this sweep from both delivering
	// the code, so the guard has to hold even after the sweep's own send succeeded.
	claimedAgain, err := bookings.ClaimLockCodeSend(context.Background(), due.ID, time.Now())
	if err != nil {
		t.Fatalf("repeat ClaimLockCodeSend() error = %v", err)
	}
	if claimedAgain {
		t.Error("repeat ClaimLockCodeSend() on an already-sent row claimed = true, want false")
	}
}

// A send that fails must leave the row claimable, or the next tick skips it and the
// guest arrives at a locked door with no code.
func TestSendDueLockCodesReleasesClaimOnFailure(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	code := "1234"
	due := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Minute), DoorLockCode: &code,
	})
	notifier.lockCodeErr = errors.New("zns down")

	if err := uc.SendDueLockCodes(context.Background()); err != nil {
		t.Fatalf("SendDueLockCodes() error = %v, want nil (per-row failures are logged)", err)
	}
	if due.LockCodeSentAt != nil {
		t.Errorf("LockCodeSentAt = %v, want nil so the next tick retries", due.LockCodeSentAt)
	}
	if bookings.releaseCalls != 1 {
		t.Errorf("releaseCalls = %d, want 1", bookings.releaseCalls)
	}
}
