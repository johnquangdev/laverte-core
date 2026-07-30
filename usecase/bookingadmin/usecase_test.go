package bookingadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
)

type fakeBookingRepo struct {
	rows      map[uint]*model.Booking
	nextID    uint
	createErr error

	// Call counters. Several paths here have both a read-time status check and a SQL
	// guard, and only the counter distinguishes "the guard ran" from "the value
	// happened to already be right".
	setCodeCalls int
	claimCalls   int
	releaseCalls int

	// beforeClaim runs inside ClaimLockCodeSend, standing in for another writer
	// winning the claim between the caller's read and its own attempt.
	beforeClaim func()

	// afterGetByID runs inside GetByID, given the stored row rather than the copy
	// handed to the caller — it stands in for a concurrent writer (e.g. the alert
	// sweep) landing its own change on the DB row between this read and whatever
	// write the caller makes from its now-stale copy.
	afterGetByID func(stored *model.Booking)
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{rows: map[uint]*model.Booking{}}
}

func (f *fakeBookingRepo) seed(b *model.Booking) *model.Booking {
	f.nextID++
	b.ID = f.nextID
	f.rows[b.ID] = b
	return b
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.seed(b)
	return nil
}

// GetByID returns a copy, mirroring GORM's First: the caller's local struct is a
// snapshot from read time, not a live view of the row. Without that, a usecase's
// read-time status check and the SQL guard behind it could never disagree, so no
// test could show the guard doing any work. afterGetByID fires after the snapshot
// is taken, so it can land a change the caller cannot see.
func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	b, ok := f.rows[id]
	if !ok {
		return nil, errors.New("booking not found")
	}
	cp := *b
	if f.afterGetByID != nil {
		f.afterGetByID(b)
	}
	return &cp, nil
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

// The four guards below mirror their SQL predicates against the stored row rather
// than the caller's snapshot. That is the whole reason they exist: GetByID above
// hands out a copy, so a guard that trusted the caller could never be shown to
// lose a race it should lose.
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

func (f *fakeBookingRepo) ConfirmIfPending(_ context.Context, id uint, paymentID uint) (bool, error) {
	b, ok := f.rows[id]
	if !ok || b.Status != model.BookingStatusPendingPayment {
		return false, nil
	}
	b.Status = model.BookingStatusConfirmed
	b.PaymentID = &paymentID
	return true, nil
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
func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(_ context.Context, homeID uint, _ time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	for _, b := range f.rows {
		if b.HomeID == homeID {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
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

// ExpireIfPending mirrors the guarded UPDATE rather than reporting a constant success:
// the admin operations here move bookings out of pending_payment, so a fake that always
// claimed the row would let a future test pass while the real query declined.
func (f *fakeBookingRepo) ExpireIfPending(_ context.Context, id uint) (bool, error) {
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
	if f.beforeClaim != nil {
		f.beforeClaim()
	}
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

// CountConfirmedBetween satisfies the interface only; this package's tests
// exercise the booking admin actions, not the revenue overview.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakeHomeRepo struct{ homes map[uint]*model.Home }

func (f *fakeHomeRepo) Create(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeHomeRepo) Update(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeHomeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	h, ok := f.homes[id]
	if !ok {
		return nil, errors.New("home not found")
	}
	return h, nil
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
	rows   map[uint]*model.Payment
	nextID uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{rows: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error {
	f.nextID++
	p.ID = f.nextID
	f.rows[p.ID] = p
	return nil
}
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
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakePricing struct {
	price int64
	err   error
}

func (f *fakePricing) Compute(context.Context, string, string, time.Time, time.Time, time.Time) (int64, error) {
	return f.price, f.err
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

type fakeCalendar struct {
	eventID      string
	createErr    error
	createCalls  int
	deleteCalls  int
	deletedEvent string
}

func (f *fakeCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	f.createCalls++
	return f.eventID, f.createErr
}
func (f *fakeCalendar) DeleteEvent(_ context.Context, _ string, eventID string) error {
	f.deleteCalls++
	f.deletedEvent = eventID
	return nil
}

type deps struct {
	bookings *fakeBookingRepo
	homes    *fakeHomeRepo
	blocked  *fakeBlockedSlotRepo
	payments *fakePaymentRepo
	pricing  *fakePricing
	notifier *fakeNotifier
	calendar *fakeCalendar
}

func newTestUseCase() (*UseCase, *deps) {
	d := &deps{
		bookings: newFakeBookingRepo(),
		homes: &fakeHomeRepo{homes: map[uint]*model.Home{
			1: {ID: 1, Name: "Nest 1", Category: model.HomeCategoryNest, GoogleCalendarID: "cal-1", IsActive: true},
		}},
		blocked:  &fakeBlockedSlotRepo{},
		payments: newFakePaymentRepo(),
		pricing:  &fakePricing{price: 300000},
		notifier: &fakeNotifier{},
		calendar: &fakeCalendar{eventID: "evt-1"},
	}
	uc := New(d.bookings, d.homes, d.blocked, d.payments, d.pricing, d.notifier, d.calendar, zap.NewNop()).(*UseCase)
	return uc, d
}

func walkInRequest() payload.CreateWalkInBookingRequest {
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	return payload.CreateWalkInBookingRequest{
		HomeID: 1, CustomerName: "Khach", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(2 * time.Hour),
		BookingType: model.BookingTypeHourly,
	}
}

func TestCreateWalkInIsConfirmedAndAttributedToAdmin(t *testing.T) {
	uc, d := newTestUseCase()

	resp, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	if err != nil {
		t.Fatalf("CreateWalkIn() error = %v", err)
	}
	if resp.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", resp.Status, model.BookingStatusConfirmed)
	}
	if resp.CreatedByAdminID == nil || *resp.CreatedByAdminID != 77 {
		t.Errorf("CreatedByAdminID = %v, want 77", resp.CreatedByAdminID)
	}
	if resp.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil (a walk-in never expires)", resp.ExpiresAt)
	}
	if d.calendar.createCalls != 1 {
		t.Errorf("calendar.CreateEvent calls = %d, want 1", d.calendar.createCalls)
	}
	stored := d.bookings.rows[resp.ID]
	if stored.GoogleCalendarEventID != "evt-1" {
		t.Errorf("GoogleCalendarEventID = %q, want evt-1", stored.GoogleCalendarEventID)
	}
	if stored.CustomerPhone != "84900000001" {
		t.Errorf("CustomerPhone = %q, want normalized 84900000001", stored.CustomerPhone)
	}
}

func TestCreateWalkInWithPaidCashCreatesCashPayment(t *testing.T) {
	uc, d := newTestUseCase()
	req := walkInRequest()
	req.PaidCash = true

	resp, err := uc.CreateWalkIn(context.Background(), req, 77)
	if err != nil {
		t.Fatalf("CreateWalkIn() error = %v", err)
	}

	p, err := d.payments.GetByBookingID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("GetByBookingID() error = %v", err)
	}
	if p.Provider != model.PaymentProviderCash || p.Status != model.PaymentStatusPaid {
		t.Errorf("payment = %q/%q, want %q/%q", p.Provider, p.Status, model.PaymentProviderCash, model.PaymentStatusPaid)
	}
	if p.Amount != 300000 {
		t.Errorf("Amount = %d, want 300000", p.Amount)
	}
	if p.PaidAt == nil {
		t.Error("PaidAt = nil, want a timestamp")
	}
	if stored := d.bookings.rows[resp.ID]; stored.PaymentID == nil || *stored.PaymentID != p.ID {
		t.Errorf("booking.PaymentID = %v, want %d", stored.PaymentID, p.ID)
	}
}

// The walk-in path prices and holds slots exactly like the guest path, so it needs
// the same cap: an admin typo of the year in end_time would otherwise take a home
// off the market indefinitely at one night's rate.
func TestCreateWalkInRejectsOverlongBooking(t *testing.T) {
	uc, d := newTestUseCase()
	req := walkInRequest()
	req.BookingType = model.BookingTypeDay
	req.EndTime = req.StartTime.Add(pricinguc.MaxBookingDuration + time.Second)

	_, err := uc.CreateWalkIn(context.Background(), req, 77)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("CreateWalkIn() over the duration cap error = %v, want apperr Validation", err)
	}
	if len(d.bookings.rows) != 0 {
		t.Errorf("seeded %d bookings, want 0", len(d.bookings.rows))
	}
}

func TestCreateWalkInRejectsBlockedSlot(t *testing.T) {
	uc, d := newTestUseCase()
	d.blocked.overlap = true

	_, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Fatalf("CreateWalkIn() error = %v, want CodeSlotConflict", err)
	}
	if e.HTTPCode != 409 {
		t.Errorf("HTTPCode = %d, want 409", e.HTTPCode)
	}
}

func TestCreateWalkInMapsRepoSlotConflict(t *testing.T) {
	uc, d := newTestUseCase()
	d.bookings.createErr = bookingrepo.ErrSlotConflict

	_, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Fatalf("CreateWalkIn() error = %v, want CodeSlotConflict", err)
	}
	if e.HTTPCode != 409 {
		t.Errorf("HTTPCode = %d, want 409 (not 500)", e.HTTPCode)
	}
}

func TestCancelRejectsAlreadyCancelled(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusCancelled})

	err := uc.Cancel(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Cancel() error = %v, want CodeValidation", err)
	}
}

func TestCancelRejectsTerminalStatuses(t *testing.T) {
	for _, status := range []string{model.BookingStatusCompleted, model.BookingStatusNoShow} {
		t.Run(status, func(t *testing.T) {
			uc, d := newTestUseCase()
			b := d.bookings.seed(&model.Booking{HomeID: 1, Status: status})

			err := uc.Cancel(context.Background(), b.ID)
			e, ok := apperr.As(err)
			if !ok || e.Code != apperr.CodeValidation {
				t.Fatalf("Cancel() error = %v, want CodeValidation", err)
			}
			if b.Status != status {
				t.Errorf("Status = %q, want %q unchanged: the payment behind a finished stay is already counted as revenue and there is nothing to refund from",
					b.Status, status)
			}
		})
	}
}

func TestCancelDeletesCalendarEvent(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, GoogleCalendarEventID: "evt-9"})

	if err := uc.Cancel(context.Background(), b.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if stored := d.bookings.rows[b.ID]; stored.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q", stored.Status, model.BookingStatusCancelled)
	}
	if d.calendar.deleteCalls != 1 || d.calendar.deletedEvent != "evt-9" {
		t.Errorf("DeleteEvent calls = %d, event = %q; want 1, evt-9", d.calendar.deleteCalls, d.calendar.deletedEvent)
	}
}

// TestCancelRefusesWhenRowWentTerminalMidRequest is the admin-facing half of the
// reviewer's second scenario. afterGetByID lands a terminal status on the stored
// row after Cancel has taken its snapshot, which is what an expiry sweep or a
// second admin does in the real system. Answering {"ok":true} there would tell
// the admin a stay is off while it is still on the books — and would delete its
// Calendar event, removing the only place the discrepancy was visible.
func TestCancelRefusesWhenRowWentTerminalMidRequest(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed, GoogleCalendarEventID: "evt-9",
	})
	d.bookings.afterGetByID = func(stored *model.Booking) { stored.Status = model.BookingStatusCompleted }

	err := uc.Cancel(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Cancel() error = %v, want CodeValidation", err)
	}
	if stored := d.bookings.rows[b.ID]; stored.Status != model.BookingStatusCompleted {
		t.Errorf("Status = %q, want %q left standing", stored.Status, model.BookingStatusCompleted)
	}
	if d.calendar.deleteCalls != 0 {
		t.Errorf("DeleteEvent calls = %d, want 0 — nothing was cancelled", d.calendar.deleteCalls)
	}
}

// TestCloseOutRefusesWhenBookingLeftConfirmedMidRequest covers Complete and NoShow
// together: both release the slot (completed and no_show sit outside the overlap
// exclusion constraint), so winning against a row that is no longer confirmed
// would put an occupied window back on sale.
func TestCloseOutRefusesWhenBookingLeftConfirmedMidRequest(t *testing.T) {
	actions := map[string]func(*UseCase, context.Context, uint) error{
		"complete": func(uc *UseCase, ctx context.Context, id uint) error { return uc.Complete(ctx, id) },
		"no-show":  func(uc *UseCase, ctx context.Context, id uint) error { return uc.NoShow(ctx, id) },
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			uc, d := newTestUseCase()
			b := d.bookings.seed(&model.Booking{
				HomeID: 1, Status: model.BookingStatusConfirmed,
				StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour),
			})
			d.bookings.afterGetByID = func(stored *model.Booking) {
				stored.Status = model.BookingStatusCancelled
			}

			err := action(uc, context.Background(), b.ID)
			e, ok := apperr.As(err)
			if !ok || e.Code != apperr.CodeValidation {
				t.Fatalf("error = %v, want CodeValidation", err)
			}
			if stored := d.bookings.rows[b.ID]; stored.Status != model.BookingStatusCancelled {
				t.Errorf("Status = %q, want %q left standing", stored.Status, model.BookingStatusCancelled)
			}
		})
	}
}

func TestCompleteRejectsPendingPayment(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment})

	err := uc.Complete(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Complete() error = %v, want CodeValidation", err)
	}
}

func TestCompleteRejectsBookingThatHasNotStarted(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(time.Hour), EndTime: time.Now().Add(3 * time.Hour),
	})

	err := uc.Complete(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Complete() error = %v, want CodeValidation for a booking that has not started", err)
	}
	if b.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want unchanged %q", b.Status, model.BookingStatusConfirmed)
	}
}

func TestCompleteAcceptsBookingThatHasStarted(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour),
	})

	if err := uc.Complete(context.Background(), b.ID); err != nil {
		t.Fatalf("Complete() error = %v, want nil for a booking already underway", err)
	}
	if stored := d.bookings.rows[b.ID]; stored.Status != model.BookingStatusCompleted {
		t.Errorf("Status = %q, want %q", stored.Status, model.BookingStatusCompleted)
	}
}
