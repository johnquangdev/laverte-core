package bookingadmin

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
	"github.com/johnquangdev/laverte-home/util/gcalendar"
	"github.com/johnquangdev/laverte-home/util/notify"
)

type UseCase struct {
	bookingRepo     bookingrepo.IRepository
	homeRepo        homerepo.IRepository
	blockedSlotRepo blockedslotrepo.IRepository
	paymentRepo     paymentrepo.IRepository
	pricingUC       pricinguc.IUseCase
	notifier        notify.INotifier
	calendar        gcalendar.ICalendar
	log             *zap.Logger
}

func New(
	bookingRepo bookingrepo.IRepository,
	homeRepo homerepo.IRepository,
	blockedSlotRepo blockedslotrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	pricingUC pricinguc.IUseCase,
	notifier notify.INotifier,
	calendar gcalendar.ICalendar,
	log *zap.Logger,
) IUseCase {
	return &UseCase{
		bookingRepo:     bookingRepo,
		homeRepo:        homeRepo,
		blockedSlotRepo: blockedSlotRepo,
		paymentRepo:     paymentRepo,
		pricingUC:       pricingUC,
		notifier:        notifier,
		calendar:        calendar,
		log:             log,
	}
}

func (uc *UseCase) ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]presenter.AdminBookingResponse, error) {
	rows, err := uc.bookingRepo.ListByHomeAndDate(ctx, homeID, day)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.AdminBookingResponse, 0, len(rows))
	for _, b := range rows {
		out = append(out, presenter.ToAdminBookingResponse(b))
	}
	return out, nil
}

func (uc *UseCase) CreateWalkIn(ctx context.Context, req payload.CreateWalkInBookingRequest, adminID uint) (*presenter.AdminBookingResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	if !model.IsValidBookingType(req.BookingType) {
		return nil, apperr.Validation("booking_type phai la 'hourly', 'overnight' hoac 'day'")
	}
	// One canonical spelling for the stored row and any later phone-keyed lookup
	// (the guest usecase's re-use check, the rate limiter), so the same number
	// cannot hold several slots under different formats.
	phone := model.NormalizeVNPhone(req.CustomerPhone)
	if phone == "" {
		return nil, apperr.Validation("so dien thoai khong hop le")
	}

	home, err := uc.homeRepo.GetByID(ctx, req.HomeID)
	if err != nil {
		return nil, apperr.NotFound(err)
	}

	blocked, err := uc.blockedSlotRepo.HasOverlap(ctx, req.HomeID, req.StartTime, req.EndTime)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if blocked {
		return nil, apperr.SlotConflict(nil)
	}

	price, err := uc.pricingUC.Compute(ctx, home.Category, req.BookingType, req.StartTime, req.EndTime, time.Now())
	if err != nil {
		return nil, err
	}

	b := &model.Booking{
		HomeID:           req.HomeID,
		CustomerName:     req.CustomerName,
		CustomerPhone:    phone,
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		BookingType:      req.BookingType,
		ComputedPrice:    price,
		Status:           model.BookingStatusConfirmed,
		CreatedByAdminID: &adminID,
		// A walk-in guest is physically in the room, so the slot is held
		// outright — leaving ExpiresAt nil keeps the pending-expiry sweep from
		// ever reclaiming it.
		ExpiresAt: nil,
	}
	if err := uc.bookingRepo.Create(ctx, b); err != nil {
		if errors.Is(err, bookingrepo.ErrSlotConflict) {
			return nil, apperr.SlotConflict(err)
		}
		return nil, apperr.Internal(err)
	}

	if req.PaidCash {
		now := time.Now()
		p := &model.Payment{
			BookingID: b.ID,
			Provider:  model.PaymentProviderCash,
			Status:    model.PaymentStatusPaid,
			Amount:    price,
			PaidAt:    &now,
		}
		if err := uc.paymentRepo.Create(ctx, p); err != nil {
			return nil, apperr.Internal(err)
		}
		b.PaymentID = &p.ID
		if err := uc.bookingRepo.SetPaymentID(ctx, b.ID, p.ID); err != nil {
			return nil, apperr.Internal(err)
		}
	}

	uc.pushCalendarEvent(ctx, home, b)

	resp := presenter.ToAdminBookingResponse(b)
	return &resp, nil
}

// pushCalendarEvent is best-effort, the same stance the SePay webhook takes:
// the booking row is already committed and the guest is already checked in, so
// a Calendar outage must not turn a real stay into a failed request.
func (uc *UseCase) pushCalendarEvent(ctx context.Context, home *model.Home, b *model.Booking) {
	if home.GoogleCalendarID == "" {
		return
	}
	eventID, err := uc.calendar.CreateEvent(ctx, home.GoogleCalendarID, b)
	if err != nil {
		uc.log.Error("walk-in calendar push failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	b.GoogleCalendarEventID = eventID
	if err := uc.bookingRepo.SetCalendarEventID(ctx, b.ID, eventID); err != nil {
		uc.log.Error("persist calendar event id failed", zap.Uint("booking_id", b.ID), zap.Error(err))
	}
}

// errNotConfirmed is the refusal every close-out action shares. Each checks the
// status twice — once against the row it read, once in the SQL guard that
// actually enforces it — and the admin must not be able to tell which one caught
// it, since both mean the same thing: there is no confirmed stay to close.
func errNotConfirmed() error {
	return apperr.Validation("chi booking dang 'confirmed' moi doi duoc trang thai nay")
}

func (uc *UseCase) Cancel(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	switch b.Status {
	case model.BookingStatusCancelled, model.BookingStatusExpired:
		return apperr.Validation("booking da huy hoac da het han")
	case model.BookingStatusCompleted, model.BookingStatusNoShow:
		// These are terminal and the payment behind them is already counted as
		// revenue. There is no refund concept in this system, so flipping one to
		// cancelled would leave the ledger saying paid and the booking saying it
		// never happened, with nothing to reconcile from.
		return apperr.Validation("booking da ket thuc, khong the huy")
	}

	cancelled, err := uc.bookingRepo.CancelIfNotTerminal(ctx, id)
	if err != nil {
		return apperr.Internal(err)
	}
	// The status check above ran against a row read moments ago; the SQL guard is
	// what actually enforces it. Losing means a terminal status arrived in between —
	// the sweep expired the hold, or another admin got there first — and the admin
	// must be told, or they walk away believing a stay is cancelled when it is not.
	if !cancelled {
		return apperr.Validation("booking da huy, het han hoac da ket thuc — khong the huy")
	}

	uc.deleteCalendarEvent(ctx, b)
	return nil
}

// Complete closes out a confirmed stay. Unlike NoShow, it also requires the
// stay to have actually started: completed sits outside the DB's overlap
// exclusion constraint on purpose, so that a past stay stops blocking its
// slot — which makes completing a booking early a deliberate release of the
// remaining hours. Completing one that never began would free the slot for a
// stay the record claims already happened.
func (uc *UseCase) Complete(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.Status != model.BookingStatusConfirmed {
		return errNotConfirmed()
	}
	if b.StartTime.After(time.Now()) {
		return apperr.Validation("booking chua bat dau, khong the hoan thanh")
	}

	completed, err := uc.bookingRepo.CompleteIfConfirmed(ctx, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if !completed {
		return errNotConfirmed()
	}
	return nil
}

// NoShow closes out a confirmed stay the guest never arrived for. A booking that
// was never confirmed has no stay to close out, so the request is a mistake
// rather than a no-op.
func (uc *UseCase) NoShow(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.Status != model.BookingStatusConfirmed {
		return errNotConfirmed()
	}
	marked, err := uc.bookingRepo.NoShowIfConfirmed(ctx, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if !marked {
		return errNotConfirmed()
	}
	return nil
}

// deleteCalendarEvent mirrors pushCalendarEvent's best-effort stance — the
// cancellation is already persisted, a leftover Calendar event is cleaned up by
// hand.
func (uc *UseCase) deleteCalendarEvent(ctx context.Context, b *model.Booking) {
	if b.GoogleCalendarEventID == "" {
		return
	}
	home, err := uc.homeRepo.GetByID(ctx, b.HomeID)
	if err != nil {
		uc.log.Error("load home for calendar delete failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	if home.GoogleCalendarID == "" {
		return
	}
	if err := uc.calendar.DeleteEvent(ctx, home.GoogleCalendarID, b.GoogleCalendarEventID); err != nil {
		uc.log.Error("calendar event delete failed", zap.Uint("booking_id", b.ID), zap.Error(err))
	}
}
