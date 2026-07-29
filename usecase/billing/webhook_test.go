package billing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type fakeBookingRepo struct {
	booking     *model.Booking
	updateCalls int
}

func (f *fakeBookingRepo) Create(context.Context, *model.Booking) error { return nil }

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	if f.booking == nil || f.booking.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return f.booking, nil
}

func (f *fakeBookingRepo) Update(_ context.Context, b *model.Booking) error {
	f.updateCalls++
	f.booking = b
	return nil
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

func (f *fakeBookingRepo) MarkLockCodeSent(_ context.Context, id uint, at time.Time) error {
	if f.booking != nil && f.booking.ID == id {
		f.booking.LockCodeSentAt = &at
	}
	return nil
}

type fakePaymentRepo struct {
	payment   *model.Payment
	markOK    bool
	markErr   error
	markCalls int
}

func (f *fakePaymentRepo) Create(context.Context, *model.Payment) error { return nil }

func (f *fakePaymentRepo) GetByID(context.Context, uint) (*model.Payment, error) {
	if f.payment == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) GetByBookingID(context.Context, uint) (*model.Payment, error) {
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
}

func TestHandleSePayWebhookAcknowledgesPing(t *testing.T) {
	h := newHarness(pendingBooking(), nil, checkout.ErrWebhookPing)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte(""), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on ping error = %v, want nil", err)
	}
	if h.payments.markCalls != 0 || h.bookings.updateCalls != 0 {
		t.Errorf("ping mutated state: markCalls=%d updateCalls=%d", h.payments.markCalls, h.bookings.updateCalls)
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

func TestHandleSePayWebhookStopsWhenPaymentAlreadyPaid(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.markOK = false

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want it untouched when the payment was already settled", h.bookings.booking.Status)
	}
	if h.bookings.updateCalls != 0 {
		t.Errorf("bookingRepo.Update calls = %d, want 0", h.bookings.updateCalls)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0", h.notifier.confirmedCalls)
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
