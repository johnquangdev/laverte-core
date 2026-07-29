package booking

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type fakeBookingRepo struct {
	pending     *model.Booking
	createErr   error
	created     []*model.Booking
	updateCalls int
	nextID      uint
	byID        map[uint]*model.Booking
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.nextID++
	b.ID = f.nextID
	f.created = append(f.created, b)
	if f.byID == nil {
		f.byID = map[uint]*model.Booking{}
	}
	f.byID[b.ID] = b
	return nil
}

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	for _, b := range f.created {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBookingRepo) Update(_ context.Context, _ *model.Booking) error {
	f.updateCalls++
	return nil
}

func (f *fakeBookingRepo) GetPendingByPhone(_ context.Context, phone string) (*model.Booking, error) {
	if f.pending != nil && f.pending.CustomerPhone == phone {
		return f.pending, nil
	}
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

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18. Stubbed here
// already so that task never has to retro-edit this file.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeBookingRepo) MarkLockCodeAlertSent(_ context.Context, id uint, at time.Time) error {
	if b, ok := f.byID[id]; ok {
		b.LockCodeAlertSentAt = &at
	}
	return nil
}

func (f *fakeBookingRepo) MarkLockCodeSent(_ context.Context, id uint, at time.Time) error {
	if b, ok := f.byID[id]; ok {
		b.LockCodeSentAt = &at
	}
	return nil
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

type fakeBlockedSlotRepo struct{ overlap bool }

func (f *fakeBlockedSlotRepo) Create(context.Context, *model.BlockedSlot) error { return nil }
func (f *fakeBlockedSlotRepo) Delete(context.Context, uint) error               { return nil }
func (f *fakeBlockedSlotRepo) ListByHome(context.Context, uint) ([]*model.BlockedSlot, error) {
	return nil, nil
}
func (f *fakeBlockedSlotRepo) HasOverlap(context.Context, uint, time.Time, time.Time) (bool, error) {
	return f.overlap, nil
}

type fakePaymentRepo struct {
	byBookingID map[uint]*model.Payment
	nextID      uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{byBookingID: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error {
	f.nextID++
	p.ID = f.nextID
	f.byBookingID[p.BookingID] = p
	return nil
}

func (f *fakePaymentRepo) GetByID(_ context.Context, id uint) (*model.Payment, error) {
	for _, p := range f.byBookingID {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakePaymentRepo) GetByBookingID(_ context.Context, bookingID uint) (*model.Payment, error) {
	p, ok := f.byBookingID[bookingID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}

func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return true, nil
}

func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

// MarkExpiredIfPending joins paymentrepo.IRepository in Task 17, stubbed here
// for the same reason as fakeBookingRepo.CountConfirmedBetween.
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakePricingUC struct {
	price int64
	calls int
}

func (f *fakePricingUC) Compute(context.Context, string, string, time.Time, time.Time, time.Time) (int64, error) {
	f.calls++
	return f.price, nil
}

type fakePaymentProvider struct {
	qrContent string
	calls     int
}

func (f *fakePaymentProvider) CreateQR(_ context.Context, req checkout.CreateQRRequest) (*checkout.QRResult, error) {
	f.calls++
	return &checkout.QRResult{
		QRContent:   f.qrContent,
		ProviderRef: fmt.Sprintf("LAVERTE%d", req.BookingID),
	}, nil
}

func (f *fakePaymentProvider) VerifyWebhook(context.Context, []byte, http.Header) (*checkout.WebhookEvent, error) {
	return nil, nil
}

type harness struct {
	uc       IUseCase
	bookings *fakeBookingRepo
	homes    *fakeHomeRepo
	slots    *fakeBlockedSlotRepo
	payments *fakePaymentRepo
	pricing  *fakePricingUC
	provider *fakePaymentProvider
}

func newHarness() *harness {
	h := &harness{
		bookings: &fakeBookingRepo{},
		homes:    &fakeHomeRepo{home: &model.Home{ID: 1, Name: "Nest 1", Category: model.HomeCategoryNest, IsActive: true}},
		slots:    &fakeBlockedSlotRepo{},
		payments: newFakePaymentRepo(),
		pricing:  &fakePricingUC{price: 300000},
		provider: &fakePaymentProvider{qrContent: "00020101021238..."},
	}
	h.uc = New(h.bookings, h.homes, h.slots, h.payments, h.pricing, h.provider, config.Config{BookingPendingTTLMinutes: 15})
	return h
}

func validRequest() payload.CreateBookingRequest {
	start := time.Now().Add(2 * time.Hour)
	return payload.CreateBookingRequest{
		HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(3 * time.Hour), BookingType: model.BookingTypeHourly,
	}
}

func TestCreateHappyPathReturnsPendingPaymentWithQR(t *testing.T) {
	h := newHarness()

	resp, err := h.uc.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want %q", resp.Status, model.BookingStatusPendingPayment)
	}
	if resp.ComputedPrice != 300000 {
		t.Errorf("ComputedPrice = %d, want 300000", resp.ComputedPrice)
	}
	if resp.QRContent != "00020101021238..." {
		t.Errorf("QRContent = %q, want the provider's QR", resp.QRContent)
	}
	if resp.ExpiresAt == nil {
		t.Error("ExpiresAt = nil, want the pending TTL deadline")
	}
	pay, err := h.payments.GetByBookingID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("payment for booking %d not created: %v", resp.ID, err)
	}
	if pay.Status != model.PaymentStatusPending || pay.Amount != 300000 {
		t.Errorf("payment = %+v, want pending/300000", pay)
	}
	if h.bookings.updateCalls != 1 {
		t.Errorf("bookingRepo.Update calls = %d, want 1 (link payment_id)", h.bookings.updateCalls)
	}
}

func TestCreateReturnsExistingPendingBookingForSamePhone(t *testing.T) {
	h := newHarness()
	expires := time.Now().Add(10 * time.Minute)
	h.bookings.pending = &model.Booking{
		ID: 42, HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status: model.BookingStatusPendingPayment, ExpiresAt: &expires,
	}
	h.payments.byBookingID[42] = &model.Payment{
		ID: 7, BookingID: 42, Provider: model.PaymentProviderSePay, Amount: 300000,
		Status: model.PaymentStatusPending, QRContent: "QR-EXISTING",
	}

	resp, err := h.uc.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("ID = %d, want the existing pending booking 42", resp.ID)
	}
	if resp.QRContent != "QR-EXISTING" {
		t.Errorf("QRContent = %q, want QR-EXISTING", resp.QRContent)
	}
	if h.pricing.calls != 0 {
		t.Errorf("pricing Compute calls = %d, want 0 — re-use must short-circuit before any work", h.pricing.calls)
	}
	if h.provider.calls != 0 {
		t.Errorf("CreateQR calls = %d, want 0 — no second QR for the same phone", h.provider.calls)
	}
	if len(h.bookings.created) != 0 {
		t.Errorf("created %d bookings, want 0", len(h.bookings.created))
	}
}

func TestCreateRejectsBlockedSlot(t *testing.T) {
	h := newHarness()
	h.slots.overlap = true

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() over a blocked slot = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeSlotConflict)
	}
	if h.provider.calls != 0 {
		t.Errorf("CreateQR calls = %d, want 0", h.provider.calls)
	}
}

func TestCreateMapsRepoSlotConflictToApperr(t *testing.T) {
	h := newHarness()
	h.bookings.createErr = bookingrepo.ErrSlotConflict

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() with ErrSlotConflict = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("apperr.As(%v) = false, want true", err)
	}
	if e.Code != apperr.CodeSlotConflict {
		t.Errorf("Code = %q, want %q", e.Code, apperr.CodeSlotConflict)
	}
}

func TestCreateRejectsInactiveHome(t *testing.T) {
	h := newHarness()
	h.homes.home.IsActive = false

	if _, err := h.uc.Create(context.Background(), validRequest()); err == nil {
		t.Fatal("Create() for an inactive home = nil error, want error")
	}
	if len(h.bookings.created) != 0 {
		t.Errorf("created %d bookings, want 0", len(h.bookings.created))
	}
}
