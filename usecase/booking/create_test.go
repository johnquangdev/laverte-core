package booking

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type fakeBookingRepo struct {
	pending           *model.Booking
	createErr         error
	created           []*model.Booking
	setPaymentIDCalls int
	nextID            uint
	byID              map[uint]*model.Booking
}

// setPending registers the hold in byID as well, so the guarded writes below see
// it the way the real repository would — a pending row reachable by id, not just
// by phone. Without that, releasing an orphan would silently find no row.
func (f *fakeBookingRepo) setPending(b *model.Booking) {
	f.pending = b
	if f.byID == nil {
		f.byID = map[uint]*model.Booking{}
	}
	f.byID[b.ID] = b
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

func (f *fakeBookingRepo) SetPaymentID(_ context.Context, id uint, paymentID uint) error {
	f.setPaymentIDCalls++
	if b, ok := f.byID[id]; ok {
		b.PaymentID = &paymentID
	}
	return nil
}

func (f *fakeBookingRepo) SetCalendarEventID(_ context.Context, id uint, eventID string) error {
	if b, ok := f.byID[id]; ok {
		b.GoogleCalendarEventID = eventID
	}
	return nil
}

// The status guards below are never reached from booking creation, but they mirror
// their predicates anyway: reporting a win they did not perform is exactly the
// defect the guards were introduced to remove.
func (f *fakeBookingRepo) ConfirmIfPending(_ context.Context, id uint, paymentID uint) (bool, error) {
	b, ok := f.byID[id]
	if !ok || b.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	b.Status = model.BookingStatusConfirmed
	b.PaymentID = &paymentID
	return true, nil
}

func (f *fakeBookingRepo) CancelIfNotTerminal(_ context.Context, id uint) (bool, error) {
	b, ok := f.byID[id]
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
	b, ok := f.byID[id]
	if !ok || b.Status != model.BookingStatusConfirmed {
		return false, nil
	}
	b.Status = status
	return true, nil
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

// SetDoorLockCode, ClaimLockCodeSend and ReleaseLockCodeSend satisfy the interface
// only; this package's tests exercise booking creation, not the lock-code flow.
func (f *fakeBookingRepo) SetDoorLockCode(context.Context, uint, string) error { return nil }

// ReleaseHoldIfPending mirrors the guarded UPDATE: creation releases an orphan hold
// through this path, so a fake that always claimed the row would hide a release that
// ran against a booking the webhook had already confirmed.
func (f *fakeBookingRepo) ReleaseHoldIfPending(_ context.Context, id uint) (bool, error) {
	b, ok := f.byID[id]
	if !ok || b.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	b.Status = model.BookingStatusExpired
	return true, nil
}

func (f *fakeBookingRepo) ExpireIfPending(ctx context.Context, id uint) (bool, error) {
	return f.ReleaseHoldIfPending(ctx, id)
}

func (f *fakeBookingRepo) ClaimLockCodeSend(context.Context, uint, time.Time) (bool, error) {
	return true, nil
}

func (f *fakeBookingRepo) ReleaseLockCodeSend(context.Context, uint) error { return nil }

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
	err       error
	calls     int
}

func (f *fakePaymentProvider) CreateQR(_ context.Context, req checkout.CreateQRRequest) (*checkout.QRResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
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
	h.uc = New(h.bookings, h.homes, h.slots, h.payments, h.pricing, h.provider, config.Config{BookingPendingTTLMinutes: 15}, zap.NewNop())
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
	if h.bookings.setPaymentIDCalls != 1 {
		t.Errorf("bookingRepo.SetPaymentID calls = %d, want 1 (link payment_id)", h.bookings.setPaymentIDCalls)
	}
}

func TestCreateReturnsExistingPendingBookingForSamePhone(t *testing.T) {
	h := newHarness()
	req := validRequest()
	expires := time.Now().Add(10 * time.Minute)
	// HomeID/StartTime/EndTime match req exactly: this is the legitimate case, a
	// guest retrying the same request and re-reading their own QR.
	h.bookings.setPending(&model.Booking{
		ID: 42, HomeID: req.HomeID, CustomerName: "Khach A", CustomerPhone: "84900000001",
		StartTime: req.StartTime, EndTime: req.EndTime,
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status: model.BookingStatusPendingPayment, ExpiresAt: &expires,
	})
	h.payments.byBookingID[42] = &model.Payment{
		ID: 7, BookingID: 42, Provider: model.PaymentProviderSePay, Amount: 300000,
		Status: model.PaymentStatusPending, QRContent: "QR-EXISTING",
	}

	resp, err := h.uc.Create(context.Background(), req)
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

// TestCreateRejectsMismatchedPendingHoldWithoutDisclosure proves the endpoint does not
// let a caller read back a stranger's booking merely by typing their phone number: this
// route is public and unauthenticated, so a phone with a pending hold for a DIFFERENT
// home or window must be refused, and the refusal must not leak that hold's name, home,
// or id.
func TestCreateRejectsMismatchedPendingHoldWithoutDisclosure(t *testing.T) {
	base := validRequest()
	cases := map[string]func(*model.Booking){
		"different home": func(b *model.Booking) { b.HomeID = base.HomeID + 1 },
		"different window": func(b *model.Booking) {
			b.StartTime = base.StartTime.Add(48 * time.Hour)
			b.EndTime = b.StartTime.Add(2 * time.Hour)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			pending := &model.Booking{
				ID: 999, HomeID: base.HomeID, CustomerName: "Nguoi La", CustomerPhone: "84900000001",
				StartTime: base.StartTime, EndTime: base.EndTime,
				BookingType: model.BookingTypeHourly, ComputedPrice: 500000,
				Status: model.BookingStatusPendingPayment,
			}
			mutate(pending)
			h.bookings.setPending(pending)
			// A live, payable QR on the stranger's hold: if the home/window match check
			// were ever removed, this is exactly what would leak to the caller.
			h.payments.byBookingID[999] = &model.Payment{
				ID: 111, BookingID: 999, Provider: model.PaymentProviderSePay, Amount: 500000,
				Status: model.PaymentStatusPending, QRContent: "QR-STRANGER",
			}

			resp, err := h.uc.Create(context.Background(), base)
			if resp != nil {
				t.Fatalf("resp = %+v, want nil on a mismatched hold", resp)
			}
			e, ok := apperr.As(err)
			if !ok || e.Code != apperr.CodeConflict {
				t.Fatalf("error = %v, want apperr Conflict", err)
			}
			if strings.Contains(e.Message, "Nguoi La") || strings.Contains(e.Message, "999") {
				t.Errorf("message %q leaks the stranger's stored booking", e.Message)
			}
			if h.pricing.calls != 0 || h.provider.calls != 0 {
				t.Errorf("pricing/provider must not run on a mismatched hold: pricing=%d provider=%d", h.pricing.calls, h.provider.calls)
			}
		})
	}
}

// TestCreateReleasesOrphanHoldAndCreatesFreshBooking covers a create that died after the
// booking row committed but before a payment row existed (a crash between
// bookingRepo.Create and paymentRepo.Create, or an earlier bug). Without release, every
// retry from that phone would find the orphan, fail to load a payment, and answer 500
// for the rest of the pending TTL.
func TestCreateReleasesOrphanHoldAndCreatesFreshBooking(t *testing.T) {
	h := newHarness()
	req := validRequest()
	expires := time.Now().Add(5 * time.Minute)
	orphan := &model.Booking{
		ID: 7, HomeID: req.HomeID, CustomerName: "Khach A", CustomerPhone: "84900000001",
		StartTime: req.StartTime, EndTime: req.EndTime,
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status: model.BookingStatusPendingPayment, ExpiresAt: &expires,
	}
	h.bookings.setPending(orphan)
	// No entry in h.payments.byBookingID[7]: GetByBookingID returns gorm.ErrRecordNotFound.

	resp, err := h.uc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if orphan.Status != model.BookingStatusExpired {
		t.Errorf("orphan.Status = %q, want %q (released)", orphan.Status, model.BookingStatusExpired)
	}
	if resp.ID == orphan.ID {
		t.Errorf("resp.ID = %d, want a fresh booking id, not the orphan's", resp.ID)
	}
	if len(h.bookings.created) != 1 {
		t.Errorf("created %d bookings, want 1 fresh booking", len(h.bookings.created))
	}
}

// TestCreateReleasesBookingWhenProviderFails proves a provider failure after the booking
// row committed does not strand the slot for the whole pending TTL: the booking must be
// moved out of pending_payment immediately so the window is bookable again.
func TestCreateReleasesBookingWhenProviderFails(t *testing.T) {
	h := newHarness()
	h.provider.err = errors.New("sepay: provider unreachable")

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() with a failing provider = nil error, want error")
	}
	if len(h.bookings.created) != 1 {
		t.Fatalf("created %d bookings, want 1 (booking committed before the provider call failed)", len(h.bookings.created))
	}
	if got := h.bookings.created[0].Status; got != model.BookingStatusExpired {
		t.Errorf("booking.Status = %q, want %q (released after provider failure)", got, model.BookingStatusExpired)
	}
}

// TestCreateRejectsEmptyQRFromProvider proves a provider bug that returns success with
// no QR content is treated as a failure, not a 200 the guest can't pay against.
func TestCreateRejectsEmptyQRFromProvider(t *testing.T) {
	h := newHarness()
	h.provider.qrContent = ""

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() with an empty QR = nil error, want error")
	}
	if got := h.bookings.created[0].Status; got != model.BookingStatusExpired {
		t.Errorf("booking.Status = %q, want %q (released)", got, model.BookingStatusExpired)
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

// TestCreateRejectsOverlongBooking guards the other half of the flat-price fix.
// Even priced correctly, an unbounded range is a denial-of-business: the rate
// limiter allows repeated holds and each one takes a home off the market for the
// whole pending TTL at zero cost. The refusal must land before any pricing,
// provider or slot work.
func TestCreateRejectsOverlongBooking(t *testing.T) {
	h := newHarness()
	req := validRequest()
	req.BookingType = model.BookingTypeDay
	req.EndTime = req.StartTime.Add(pricinguc.MaxBookingDuration + time.Second)

	_, err := h.uc.Create(context.Background(), req)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Create() over the duration cap error = %v, want apperr Validation", err)
	}
	if len(h.bookings.created) != 0 {
		t.Errorf("created %d bookings, want 0", len(h.bookings.created))
	}
	if h.pricing.calls != 0 || h.provider.calls != 0 {
		t.Errorf("pricing=%d provider=%d, want 0/0", h.pricing.calls, h.provider.calls)
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
