package booking

import (
	"context"
	"errors"
	"time"

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

	// Re-use check runs before pricing, slot checks and any provider call: a
	// phone still holding an un-expired pending booking gets its existing QR
	// back, so one caller cannot mint an unbounded pile of unpaid QR codes.
	existing, err := uc.bookingRepo.GetPendingByPhone(ctx, req.CustomerPhone)
	switch {
	case err == nil:
		pay, payErr := uc.paymentRepo.GetByBookingID(ctx, existing.ID)
		if payErr != nil {
			return nil, apperr.Internal(payErr)
		}
		resp := presenter.ToBookingResponse(existing, pay.QRContent)
		return &resp, nil
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
		CustomerPhone: req.CustomerPhone,
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

	qr, err := uc.payment.CreateQR(ctx, checkout.CreateQRRequest{BookingID: b.ID, AmountVND: price})
	if err != nil {
		return nil, apperr.Internal(err)
	}

	pay := &model.Payment{
		BookingID: b.ID,
		Provider:  model.PaymentProviderSePay,
		Amount:    price,
		Status:    model.PaymentStatusPending,
		QRContent: qr.QRContent,
	}
	if err = uc.paymentRepo.Create(ctx, pay); err != nil {
		return nil, apperr.Internal(err)
	}

	b.PaymentID = &pay.ID
	if err = uc.bookingRepo.Update(ctx, b); err != nil {
		return nil, apperr.Internal(err)
	}

	resp := presenter.ToBookingResponse(b, qr.QRContent)
	return &resp, nil
}
