package bookingjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/model"
)

type fakeBookingRepo struct {
	rows       map[uint]*model.Booking
	nextID     uint
	expireErrs map[uint]error

	setCodeCalls int
	claimCalls   int
	releaseCalls int
	expireCalls  int

	// beforeExpire runs inside ExpireIfPending, standing in for the SePay webhook
	// confirming this booking between the sweep's list query and its write.
	beforeExpire func()
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{
		rows:       map[uint]*model.Booking{},
		expireErrs: map[uint]error{},
	}
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
func (f *fakeBookingRepo) SetPaymentID(_ context.Context, id uint, paymentID uint) error {
	if b, ok := f.rows[id]; ok {
		b.PaymentID = &paymentID
	}
	return nil
}

func (f *fakeBookingRepo) SetCalendarEventID(_ context.Context, id uint, eventID string) error {
	if b, ok := f.rows[id]; ok {
		b.GoogleCalendarEventID = eventID
	}
	return nil
}

// None of the four guards below is reached from a cron job, but each mirrors its
// SQL predicate: the sweeps are the other side of every race these guards exist to
// lose, so a fake that reported a constant win here would be actively misleading.
func (f *fakeBookingRepo) ConfirmIfPending(_ context.Context, id uint, paymentID uint) (bool, error) {
	b, ok := f.rows[id]
	if !ok || b.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	b.Status = model.BookingStatusConfirmed
	b.PaymentID = &paymentID
	return true, nil
}

func (f *fakeBookingRepo) CancelIfNotTerminal(_ context.Context, id uint) (bool, error) {
	b, ok := f.rows[id]
	if !ok {
		return false, nil
	}
	switch b.Status {
	case model.BookingStatusCancelled, model.BookingStatusExpired,
		model.BookingStatusCompleted, model.BookingStatusNoShow:
		return false, nil
	}
	b.Status = model.BookingStatusCancelled
	return true, nil
}

func (f *fakeBookingRepo) CompleteIfConfirmed(_ context.Context, id uint) (bool, error) {
	return f.setStatusIfConfirmed(id, model.BookingStatusCompleted)
}

func (f *fakeBookingRepo) NoShowIfConfirmed(_ context.Context, id uint) (bool, error) {
	return f.setStatusIfConfirmed(id, model.BookingStatusNoShow)
}

func (f *fakeBookingRepo) setStatusIfConfirmed(id uint, status string) (bool, error) {
	b, ok := f.rows[id]
	if !ok || b.Status != model.BookingStatusConfirmed {
		return false, nil
	}
	b.Status = status
	return true, nil
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

// ExpireIfPending mirrors the guarded UPDATE: the row moves only while it is still
// pending, and expireErrs lets a test fail one row without failing the batch.
func (f *fakeBookingRepo) ExpireIfPending(_ context.Context, id uint) (bool, error) {
	if f.beforeExpire != nil {
		f.beforeExpire()
	}
	f.expireCalls++
	if err := f.expireErrs[id]; err != nil {
		return false, err
	}
	b, ok := f.rows[id]
	if !ok || b.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	b.Status = model.BookingStatusExpired
	return true, nil
}

func (f *fakeBookingRepo) ReleaseHoldIfPending(ctx context.Context, id uint) (bool, error) {
	return f.ExpireIfPending(ctx, id)
}

// ClaimLockCodeSend mirrors the SQL: stamp only when lock_code_sent_at is still
// NULL, and report whether this caller won the claim.
func (f *fakeBookingRepo) ClaimLockCodeSend(_ context.Context, id uint, at time.Time) (bool, error) {
	b, ok := f.rows[id]
	if !ok || b.Status != model.BookingStatusConfirmed || b.LockCodeSentAt != nil {
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
	// The sweep's contract is that losing the booking write stops it touching the
	// money side at all. Neither the payment's final status nor markExpiredCalls can
	// show that on its own: expirePaymentOf bails out for an already-paid payment
	// before it writes, so both look identical whether the guard held or the sweep
	// walked in and found nothing to do. Counting the lookup is what separates them.
	getByBookingIDCalls int
	markExpiredCalls    int
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
	f.getByBookingIDCalls++
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
	f.markExpiredCalls++
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
	alertErr      error
}

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error {
	f.lockCodeCalls++
	return f.lockCodeErr
}
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	f.alertCalls++
	return f.alertErr
}
func (f *fakeNotifier) AdminUnmatchedTransfer(context.Context, *model.UnmatchedTransfer) error {
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
	bookings.expireErrs[broken.ID] = errors.New("write conflict")

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v, want nil (per-row failures are logged)", err)
	}
	if good.Status != model.BookingStatusExpired {
		t.Errorf("second booking Status = %q, want %q — a failed row must not abort the sweep", good.Status, model.BookingStatusExpired)
	}
}

// The sweep lists a batch, then writes each row a moment later. A booking the SePay
// webhook confirmed in that gap must survive: 'expired' sits outside the overlap
// exclusion constraint, so reverting a paid stay would reopen its slot to a stranger.
func TestExpirePendingBookingsLeavesBookingConfirmedMidSweepAlone(t *testing.T) {
	uc, bookings, payments, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	b := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	paidAt := time.Now()
	p := payments.seed(&model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Status: model.PaymentStatusPending, Amount: 300000})
	b.PaymentID = &p.ID

	// Stand in for the webhook landing between the list query and this row's write.
	bookings.beforeExpire = func() {
		b.Status = model.BookingStatusConfirmed
		p.Status = model.PaymentStatusPaid
		p.PaidAt = &paidAt
	}

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v", err)
	}
	if b.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q — the guarded update must not overwrite a booking the webhook confirmed",
			b.Status, model.BookingStatusConfirmed)
	}
	if p.Status != model.PaymentStatusPaid {
		t.Errorf("payment Status = %q, want %q", p.Status, model.PaymentStatusPaid)
	}
	if payments.getByBookingIDCalls != 0 || payments.markExpiredCalls != 0 {
		t.Errorf("payment side reached: GetByBookingID=%d MarkExpiredIfPending=%d, want 0/0 — a declined booking write must stop the sweep before the money",
			payments.getByBookingIDCalls, payments.markExpiredCalls)
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

// LockCodeAlertSentAt is what stops the admin being re-alerted every tick, so it must
// not be written when the alert never went out — otherwise a single email outage means
// nobody is ever told that a stay starting in ten minutes has no door code.
func TestAlertMissingLockCodesLeavesTimestampNilWhenAlertFails(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	b := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(10 * time.Minute),
	})
	notifier.alertErr = errors.New("smtp down")

	if err := uc.AlertMissingLockCodes(context.Background()); err != nil {
		t.Fatalf("AlertMissingLockCodes() error = %v, want nil (per-row failures are logged)", err)
	}
	if b.LockCodeAlertSentAt != nil {
		t.Errorf("LockCodeAlertSentAt = %v, want nil so the next tick alerts again", b.LockCodeAlertSentAt)
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
	if bookings.claimCalls != 1 {
		t.Errorf("claimCalls = %d, want 1: the send must be gated by the DB claim", bookings.claimCalls)
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

// ListOccupyingBetween serves the public availability lookup, which this package
// never calls.
func (f *fakeBookingRepo) ListOccupyingBetween(context.Context, uint, time.Time, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
