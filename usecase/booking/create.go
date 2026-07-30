package booking

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

func (uc *UseCase) Create(ctx context.Context, req payload.CreateBookingRequest) (*presenter.BookingResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	if !model.IsValidBookingType(req.BookingType) {
		return nil, apperr.Validation("booking_type phai la 'hourly', 'overnight' hoac 'day'")
	}

	// Every phone-keyed lookup (re-use check, rate limiter, stored booking) must use
	// one canonical form, or the same number spelled differently counts as several
	// different callers and silently multiplies the intended per-phone quota.
	// NormalizeVNPhone returns "" for anything that cannot be a VN mobile number, so
	// checking for that (rather than a length bound) can't accidentally accept a
	// garbage string that happens to normalize to the right length.
	phone := model.NormalizeVNPhone(req.CustomerPhone)
	if phone == "" {
		return nil, apperr.Validation("so dien thoai khong hop le")
	}

	// Re-use check runs before pricing, slot checks and any provider call: a phone
	// still holding an un-expired pending booking must not mint a second hold or a
	// second QR.
	//
	// It only hands the booking back when the request MATCHES that hold — same home,
	// same window — which is a guest retrying and re-reading their own QR. Anyone can
	// put any phone number in this body, and this endpoint is public, so returning a
	// stored booking for a phone the caller merely typed would disclose a stranger's
	// name, which property they are staying at, when, and a payable QR. A
	// non-matching request is refused instead, saying only that the phone already has
	// a pending booking.
	existing, err := uc.bookingRepo.GetPendingByPhone(ctx, phone)
	switch {
	case err == nil:
		if existing.HomeID != req.HomeID ||
			!existing.StartTime.Equal(req.StartTime) ||
			!existing.EndTime.Equal(req.EndTime) {
			return nil, apperr.Conflict(nil, "so dien thoai nay dang co mot booking cho thanh toan — vui long hoan tat hoac doi den khi no het han")
		}
		pay, payErr := uc.paymentRepo.GetByBookingID(ctx, existing.ID)
		if payErr == nil && pay.QRContent != "" {
			resp := presenter.ToBookingResponse(existing, pay.QRContent)
			return &resp, nil
		}
		if payErr != nil && !errors.Is(payErr, gorm.ErrRecordNotFound) {
			return nil, apperr.Internal(payErr)
		}
		// A hold with no usable payment row is an orphan left by a create that died
		// after the booking row committed. Release it rather than answering 500 for
		// the rest of its TTL, and fall through to build a fresh one.
		uc.releaseBooking(ctx, existing)
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, apperr.Internal(err)
	}

	home, err := uc.homeRepo.GetByID(ctx, req.HomeID)
	if err != nil {
		return nil, apperr.NotFound(err)
	}
	if !home.IsActive {
		return nil, apperr.Validation("home dang tam ngung nhan khach")
	}

	blocked, err := uc.blockedSlotRepo.HasOverlap(ctx, req.HomeID, req.StartTime, req.EndTime)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if blocked {
		return nil, apperr.SlotConflict(nil)
	}

	now := time.Now()
	price, err := uc.pricingUC.Compute(ctx, home.Category, req.BookingType, req.StartTime, req.EndTime, now)
	if err != nil {
		return nil, err // already an apperr from usecase/pricing
	}

	expiresAt := now.Add(time.Duration(uc.cfg.BookingPendingTTLMinutes) * time.Minute)
	b := &model.Booking{
		HomeID:        req.HomeID,
		CustomerName:  req.CustomerName,
		CustomerPhone: phone,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		BookingType:   req.BookingType,
		ComputedPrice: price,
		Status:        model.BookingStatusPendingPayment,
		ExpiresAt:     &expiresAt,
	}
	if err = uc.bookingRepo.Create(ctx, b); err != nil {
		// The DB exclusion constraint, not app code, decides who wins a race
		// for the same slot.
		if errors.Is(err, bookingrepo.ErrSlotConflict) {
			return nil, apperr.SlotConflict(err)
		}
		return nil, apperr.Internal(err)
	}

	// From here the row is committed and already occupying the slot, so anything that
	// fails below must release it. Leaving it would make that window unbookable for
	// the whole pending TTL for a booking nobody can pay, and the guest's own retry
	// would keep finding it.
	release := func(cause error) error {
		uc.releaseBooking(ctx, b)
		return cause
	}

	qr, err := uc.payment.CreateQR(ctx, checkout.CreateQRRequest{BookingID: b.ID, AmountVND: price})
	if err != nil {
		return nil, release(apperr.Internal(err))
	}
	if qr.QRContent == "" {
		return nil, release(apperr.Internal(errors.New("checkout: provider returned an empty QR")))
	}

	pay := &model.Payment{
		BookingID: b.ID,
		Provider:  model.PaymentProviderSePay,
		Amount:    price,
		Status:    model.PaymentStatusPending,
		QRContent: qr.QRContent,
	}
	if err = uc.paymentRepo.Create(ctx, pay); err != nil {
		return nil, release(apperr.Internal(err))
	}

	b.PaymentID = &pay.ID
	if err = uc.bookingRepo.SetPaymentID(ctx, b.ID, pay.ID); err != nil {
		// The payment row already carries the QR and is reachable via GetByBookingID
		// independent of this link, so a guest retry recovers through the match path
		// above rather than needing a release here.
		return nil, apperr.Internal(err)
	}

	resp := presenter.ToBookingResponse(b, qr.QRContent)
	return &resp, nil
}

// releaseBooking frees a hold's slot immediately by moving it out of the statuses the
// exclusion constraint covers. Best-effort by design: the caller is already returning
// an error, and the expiry sweep would collect the row eventually — this only stops the
// window being unbookable until then.
func (uc *UseCase) releaseBooking(ctx context.Context, b *model.Booking) {
	released, err := uc.bookingRepo.ReleaseHoldIfPending(ctx, b.ID)
	if err != nil {
		uc.log.Error("could not release a booking hold after a failed create",
			zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	// The hold left pending_payment while this request was failing — the transfer
	// landed and the webhook confirmed it. Its slot is no longer ours to hand back.
	if !released {
		return
	}
	b.Status = model.BookingStatusExpired
}
