package billing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/config"
	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	paymentrepo "github.com/johnquangdev/laverte-core/repository/payment"
	"github.com/johnquangdev/laverte-core/util/checkout"
)

type fakeBookingRepo struct {
	booking *model.Booking
	// confirmAttempts counts calls, confirmWins only the ones that took the row.
	// Distinguishing them is the point: a redelivery that should have short-circuited
	// on the re-read still loses the guard, so counting wins alone cannot show it
	// reached the database at all.
	confirmAttempts  int
	confirmWins      int
	setCalendarCalls int
	getCalls         int
	// getErr, if set, makes every GetByID fail with it instead of the normal
	// lookup — used to prove a DB outage surfaces as 500, not 404.
	getErr error
	// confirmOnGetCall, if non-zero, flips booking.Status to confirmed on the
	// matching 1-indexed GetByID call, simulating a concurrent delivery's
	// settle landing between this delivery's first read and its recovery re-read.
	confirmOnGetCall int
	// beforeConfirm runs inside ConfirmIfPending, standing in for an admin cancel or
	// the expiry sweep landing between this delivery's status check and its write.
	beforeConfirm func()
}

func (f *fakeBookingRepo) Create(context.Context, *model.Booking) error { return nil }

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.booking == nil || f.booking.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	if f.confirmOnGetCall != 0 && f.getCalls == f.confirmOnGetCall {
		f.booking.Status = model.BookingStatusConfirmed
	}
	return f.booking, nil
}

// ConfirmIfPending mirrors the guarded UPDATE the settle path depends on. A fake
// that reported success unconditionally would hide the whole point of the guard:
// this delivery must lose to whatever moved the booking out of pending_payment
// while the provider call was in flight.
func (f *fakeBookingRepo) ConfirmIfPending(_ context.Context, id uint, paymentID uint) (bool, error) {
	if f.beforeConfirm != nil {
		f.beforeConfirm()
	}
	f.confirmAttempts++
	if f.booking == nil || f.booking.ID != id || f.booking.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	f.confirmWins++
	f.booking.Status = model.BookingStatusConfirmed
	f.booking.PaymentID = &paymentID
	return true, nil
}

func (f *fakeBookingRepo) SetCalendarEventID(_ context.Context, id uint, eventID string) error {
	f.setCalendarCalls++
	if f.booking != nil && f.booking.ID == id {
		f.booking.GoogleCalendarEventID = eventID
	}
	return nil
}

// SetPaymentID and the three admin status guards are not reached from the webhook;
// the guards still mirror their predicates so a future test cannot pass here while
// the real query declines.
func (f *fakeBookingRepo) SetPaymentID(_ context.Context, id uint, paymentID uint) error {
	if f.booking != nil && f.booking.ID == id {
		f.booking.PaymentID = &paymentID
	}
	return nil
}

func (f *fakeBookingRepo) CancelIfNotTerminal(_ context.Context, id uint) (bool, error) {
	if f.booking == nil || f.booking.ID != id {
		return false, nil
	}
	switch f.booking.Status {
	case model.BookingStatusCancelled, model.BookingStatusExpired,
		model.BookingStatusCompleted, model.BookingStatusNoShow:
		return false, nil
	}
	f.booking.Status = model.BookingStatusCancelled
	return true, nil
}

func (f *fakeBookingRepo) CompleteIfConfirmed(_ context.Context, id uint) (bool, error) {
	return f.setStatusIfConfirmed(id, model.BookingStatusCompleted)
}

func (f *fakeBookingRepo) NoShowIfConfirmed(_ context.Context, id uint) (bool, error) {
	return f.setStatusIfConfirmed(id, model.BookingStatusNoShow)
}

func (f *fakeBookingRepo) setStatusIfConfirmed(id uint, status string) (bool, error) {
	if f.booking == nil || f.booking.ID != id || f.booking.Status != model.BookingStatusConfirmed {
		return false, nil
	}
	f.booking.Status = status
	return true, nil
}

func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, gorm.ErrRecordNotFound
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

func (f *fakeBookingRepo) MarkLockCodeAlertSent(_ context.Context, id uint, at time.Time) error {
	if f.booking != nil && f.booking.ID == id {
		f.booking.LockCodeAlertSentAt = &at
	}
	return nil
}

// SetDoorLockCode, ClaimLockCodeSend and ReleaseLockCodeSend satisfy the interface
// only; this package's tests exercise the SePay webhook, not the lock-code flow.
func (f *fakeBookingRepo) SetDoorLockCode(context.Context, uint, string) error { return nil }

// ExpireIfPending mirrors the guarded UPDATE rather than stubbing it: the webhook this
// package tests is the other half of that race, so a fake that always claimed the row
// would hide a settle path that expires a booking it just confirmed.
func (f *fakeBookingRepo) ExpireIfPending(_ context.Context, id uint) (bool, error) {
	if f.booking == nil || f.booking.ID != id || f.booking.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	f.booking.Status = model.BookingStatusExpired
	return true, nil
}

func (f *fakeBookingRepo) ReleaseHoldIfPending(ctx context.Context, id uint) (bool, error) {
	return f.ExpireIfPending(ctx, id)
}

// ClaimLockCodeSend mirrors the SQL predicate even though this package never reaches
// it: a constant win is the direction that would let a future test here believe a door
// code was claimed when the real query declined.
func (f *fakeBookingRepo) ClaimLockCodeSend(_ context.Context, id uint, at time.Time) (bool, error) {
	if f.booking == nil || f.booking.ID != id ||
		f.booking.Status != model.BookingStatusConfirmed || f.booking.LockCodeSentAt != nil {
		return false, nil
	}
	f.booking.LockCodeSentAt = &at
	return true, nil
}

func (f *fakeBookingRepo) ReleaseLockCodeSend(context.Context, uint) error { return nil }

// CountConfirmedBetween satisfies the interface only; this package's tests
// exercise the SePay webhook, not the revenue overview.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakePaymentRepo struct {
	payment   *model.Payment
	markOK    bool
	markErr   error
	markCalls int
	// getErr, if set, makes GetByBookingID fail with it instead of the normal
	// lookup — used to prove a DB outage surfaces as 500, not 404.
	getErr error
}

func (f *fakePaymentRepo) Create(context.Context, *model.Payment) error { return nil }

func (f *fakePaymentRepo) GetByID(context.Context, uint) (*model.Payment, error) {
	if f.payment == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) GetByBookingID(context.Context, uint) (*model.Payment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.payment == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	f.markCalls++
	return f.markOK, f.markErr
}

func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakeHomeRepo struct{ home *model.Home }

func (f *fakeHomeRepo) Create(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) Update(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	if f.home == nil || f.home.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return f.home, nil
}
func (f *fakeHomeRepo) List(context.Context) ([]*model.Home, error) { return nil, nil }

type fakeProvider struct {
	event     *checkout.WebhookEvent
	verifyErr error
}

func (f *fakeProvider) CreateQR(context.Context, checkout.CreateQRRequest) (*checkout.QRResult, error) {
	return nil, nil
}

func (f *fakeProvider) VerifyWebhook(context.Context, []byte, http.Header) (*checkout.WebhookEvent, error) {
	return f.event, f.verifyErr
}

type fakeNotifier struct {
	confirmedCalls int
	confirmErr     error
}

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error {
	f.confirmedCalls++
	return f.confirmErr
}
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error     { return nil }
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }

type fakeCalendar struct {
	eventID     string
	createErr   error
	createCalls int
}

func (f *fakeCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	f.createCalls++
	return f.eventID, f.createErr
}

func (f *fakeCalendar) DeleteEvent(context.Context, string, string) error { return nil }

type harness struct {
	uc       IUseCase
	bookings *fakeBookingRepo
	payments *fakePaymentRepo
	homes    *fakeHomeRepo
	provider *fakeProvider
	notifier *fakeNotifier
	calendar *fakeCalendar
}

func newHarness(booking *model.Booking, event *checkout.WebhookEvent, verifyErr error) *harness {
	h := &harness{
		bookings: &fakeBookingRepo{booking: booking},
		payments: &fakePaymentRepo{
			payment: &model.Payment{
				ID: 77, BookingID: booking.ID, Provider: model.PaymentProviderSePay,
				Amount: booking.ComputedPrice, Status: model.PaymentStatusPending, QRContent: "QR",
			},
			markOK: true,
		},
		homes:    &fakeHomeRepo{home: &model.Home{ID: booking.HomeID, Name: "Nest 1", Category: model.HomeCategoryNest, GoogleCalendarID: "cal-1", IsActive: true}},
		provider: &fakeProvider{event: event, verifyErr: verifyErr},
		notifier: &fakeNotifier{},
		calendar: &fakeCalendar{eventID: "gcal-evt-1"},
	}
	h.uc = New(h.bookings, h.payments, h.provider, h.notifier, h.calendar, h.homes,
		zap.NewNop(), config.Config{SePayTransferPrefix: "LAVERTE"})
	return h
}

func pendingBooking() *model.Booking {
	return &model.Booking{
		ID: 42, HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status:    model.BookingStatusPendingPayment,
		ExpiresAt: expiresIn(15 * time.Minute),
	}
}

func expiresIn(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}

func paidEvent() *checkout.WebhookEvent {
	return &checkout.WebhookEvent{
		ProviderRef: "CT DEN:LAVERTE42", ExternalRef: "TXN-9001",
		Success: true, AmountVND: 300000, Raw: "{}",
	}
}

func TestHandleSePayWebhookConfirmsBooking(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v", err)
	}
	if h.payments.markCalls != 1 {
		t.Errorf("MarkPaidIfPending calls = %d, want 1", h.payments.markCalls)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", h.bookings.booking.Status, model.BookingStatusConfirmed)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1", h.notifier.confirmedCalls)
	}
	if h.bookings.booking.GoogleCalendarEventID != "gcal-evt-1" {
		t.Errorf("GoogleCalendarEventID = %q, want gcal-evt-1", h.bookings.booking.GoogleCalendarEventID)
	}
	// Status and payment_id are written by one statement. A confirmed booking whose
	// payment link is missing is money SumPaidBetween counts with nothing to
	// reconcile it against.
	if h.bookings.booking.PaymentID == nil || *h.bookings.booking.PaymentID != h.payments.payment.ID {
		t.Errorf("PaymentID = %v, want %d", h.bookings.booking.PaymentID, h.payments.payment.ID)
	}
}

// TestHandleSePayWebhookDoesNotResurrectCancelledBooking covers the race the
// reviewer reproduced: the guest phones to cancel while the transfer is in flight,
// the admin cancels and the Calendar event is deleted, and then this delivery
// arrives. Confirming anyway would put the booking back inside the overlap
// exclusion constraint — re-holding the slot for a stay everyone believes is off,
// with no Calendar event left to show it.
func TestHandleSePayWebhookDoesNotResurrectCancelledBooking(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.bookings.beforeConfirm = func() { h.bookings.booking.Status = model.BookingStatusCancelled }

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v, want nil (a retry cannot help)", err)
	}
	if h.bookings.booking.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q left standing", h.bookings.booking.Status, model.BookingStatusCancelled)
	}
	if h.bookings.confirmWins != 0 {
		t.Errorf("ConfirmIfPending wins = %d, want 0", h.bookings.confirmWins)
	}
	if h.calendar.createCalls != 0 {
		t.Errorf("CreateEvent calls = %d, want 0 — the cancelled stay must not get a fresh event", h.calendar.createCalls)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookAcknowledgesPing(t *testing.T) {
	h := newHarness(pendingBooking(), nil, checkout.ErrWebhookPing)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte(""), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on ping error = %v, want nil", err)
	}
	if h.payments.markCalls != 0 || h.bookings.confirmWins != 0 {
		t.Errorf("ping mutated state: markCalls=%d confirmWins=%d", h.payments.markCalls, h.bookings.confirmWins)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want unchanged %q", h.bookings.booking.Status, model.BookingStatusPendingPayment)
	}
}

func TestHandleSePayWebhookRejectsAmountMismatch(t *testing.T) {
	event := paidEvent()
	event.AmountVND = 100000
	h := newHarness(pendingBooking(), event, nil)

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	if err == nil {
		t.Fatal("HandleSePayWebhook() with wrong amount = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeAmountMismatch {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeAmountMismatch)
	}
	if e.HTTPCode != http.StatusBadRequest {
		t.Errorf("HTTPCode = %d, want %d", e.HTTPCode, http.StatusBadRequest)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want it left pending", h.bookings.booking.Status)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0", h.payments.markCalls)
	}
}

func TestHandleSePayWebhookIsIdempotentForConfirmedBooking(t *testing.T) {
	booking := pendingBooking()
	booking.Status = model.BookingStatusConfirmed
	h := newHarness(booking, paidEvent(), nil)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on redelivery error = %v, want nil", err)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0 on redelivery", h.payments.markCalls)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0 on redelivery", h.notifier.confirmedCalls)
	}
}

// TestHandleSePayWebhookRecoversStrandedSettlement covers the case where a
// prior delivery's MarkPaidIfPending succeeded but its bookingRepo.Update
// failed before the process could confirm the booking: the payment is paid,
// the booking is still pending_payment, and nothing else will ever retry
// this except SePay's own redelivery. This delivery must finish the job —
// confirm the booking and notify — rather than repeating the old "someone
// else handled it" no-op, which left the booking stranded forever.
func TestHandleSePayWebhookRecoversStrandedSettlement(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.markOK = false // this delivery's own MarkPaidIfPending call also reports "not pending"

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want confirmed (recovered)", h.bookings.booking.Status)
	}
	if h.bookings.confirmWins != 1 {
		t.Errorf("ConfirmIfPending wins = %d, want 1", h.bookings.confirmWins)
	}
	if h.bookings.setCalendarCalls != 1 {
		t.Errorf("SetCalendarEventID calls = %d, want 1 — recovery must still push the event", h.bookings.setCalendarCalls)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1", h.notifier.confirmedCalls)
	}
}

// TestHandleSePayWebhookConcurrentRaceLoserIsANoOp covers the genuine
// concurrent race the recovery path in the test above must not disturb: a
// second delivery's MarkPaidIfPending reports "not pending" because a
// different delivery is settling the same payment right now, and by the
// time this delivery re-reads the booking, that other delivery has already
// confirmed it. The loser must do nothing — no second Update, no second
// notification.
func TestHandleSePayWebhookConcurrentRaceLoserIsANoOp(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.markOK = false
	// The 2nd GetByID call is the recovery re-read; flip to confirmed there to
	// simulate the winning delivery's Update landing in between.
	h.bookings.confirmOnGetCall = 2

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want confirmed (by the winner)", h.bookings.booking.Status)
	}
	if h.bookings.confirmAttempts != 0 {
		t.Errorf("ConfirmIfPending calls = %d, want 0 — the loser must short-circuit on the re-read, not fall through and lose the guard",
			h.bookings.confirmAttempts)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0 — the loser must not notify again", h.notifier.confirmedCalls)
	}
}

// TestHandleSePayWebhookRejectsBadSignature proves the authentication
// boundary: a VerifyWebhook failure that is not the connectivity-ping
// sentinel must be rejected as unauthorized, not silently acknowledged.
func TestHandleSePayWebhookRejectsBadSignature(t *testing.T) {
	h := newHarness(pendingBooking(), nil, errors.New("checkout: signature mismatch"))

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeUnauthorized {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeUnauthorized)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0", h.payments.markCalls)
	}
}

// TestHandleSePayWebhookBookingLookupOutageIsInternal proves a database
// outage on the booking lookup surfaces as 500, not as the 404 a guest
// would get for a booking id that genuinely doesn't exist.
func TestHandleSePayWebhookBookingLookupOutageIsInternal(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.bookings.getErr = errors.New("connection refused")

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeInternal {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeInternal)
	}
	if e != nil && e.HTTPCode != http.StatusInternalServerError {
		t.Errorf("HTTPCode = %d, want %d", e.HTTPCode, http.StatusInternalServerError)
	}
}

// TestHandleSePayWebhookPaymentLookupOutageIsInternal is the same proof as
// above for the payment lookup.
func TestHandleSePayWebhookPaymentLookupOutageIsInternal(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.getErr = errors.New("connection refused")

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeInternal {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeInternal)
	}
	if e != nil && e.HTTPCode != http.StatusInternalServerError {
		t.Errorf("HTTPCode = %d, want %d", e.HTTPCode, http.StatusInternalServerError)
	}
}

// TestHandleSePayWebhookAcknowledgesDuplicateExternalRef proves a provider
// transaction id already claimed by another payment is answered as success:
// retrying it can never settle anything, so it must not come back as a 5xx
// the provider will keep resending.
func TestHandleSePayWebhookAcknowledgesDuplicateExternalRef(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.markOK = false
	h.payments.markErr = paymentrepo.ErrDuplicateExternalRef

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on duplicate external ref error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want it left pending", h.bookings.booking.Status)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookConfirmsDespiteCalendarFailure(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.calendar.createErr = errors.New("calendar 503")

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() with a failing calendar error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", h.bookings.booking.Status, model.BookingStatusConfirmed)
	}
	if h.calendar.createCalls != 1 {
		t.Errorf("CreateEvent calls = %d, want 1", h.calendar.createCalls)
	}
	if h.bookings.booking.GoogleCalendarEventID != "" {
		t.Errorf("GoogleCalendarEventID = %q, want empty after a failed push", h.bookings.booking.GoogleCalendarEventID)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1 — notification must still run", h.notifier.confirmedCalls)
	}
}

// TestHandleSePayWebhookConfirmsDespiteNotifierFailure mirrors the calendar
// case for the other best-effort side effect: the payment already settled,
// so a notification outage must not turn into a 5xx the provider retries.
func TestHandleSePayWebhookConfirmsDespiteNotifierFailure(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.notifier.confirmErr = errors.New("zns 503")

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() with a failing notifier error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", h.bookings.booking.Status, model.BookingStatusConfirmed)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookRejectsExpiredBooking(t *testing.T) {
	booking := pendingBooking()
	booking.Status = model.BookingStatusExpired
	h := newHarness(booking, paidEvent(), nil)

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	if err == nil {
		t.Fatal("HandleSePayWebhook() on an expired booking = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeBookingExpired {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeBookingExpired)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0", h.payments.markCalls)
	}
}

func TestBookingIDFromMemo(t *testing.T) {
	cases := []struct {
		memo    string
		want    uint
		wantErr bool
	}{
		{memo: "LAVERTE42", want: 42},
		{memo: "ct dEn:laverte7 ND", want: 7},
		{memo: "LAVERTE", wantErr: true},
		{memo: "OTHER99", wantErr: true},
	}
	for _, tc := range cases {
		got, err := bookingIDFromMemo("LAVERTE", tc.memo)
		if tc.wantErr {
			if err == nil {
				t.Errorf("bookingIDFromMemo(%q) = %d, want error", tc.memo, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("bookingIDFromMemo(%q) error = %v", tc.memo, err)
			continue
		}
		if got != tc.want {
			t.Errorf("bookingIDFromMemo(%q) = %d, want %d", tc.memo, got, tc.want)
		}
	}
}

// ListOccupyingBetween serves the public availability lookup, which this package
// never calls.
func (f *fakeBookingRepo) ListOccupyingBetween(context.Context, uint, time.Time, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
