package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

func (uc *UseCase) HandleSePayWebhook(ctx context.Context, raw []byte, headers http.Header) error {
	event, err := uc.payment.VerifyWebhook(ctx, raw, headers)
	if err != nil {
		// The provider dashboard probes this endpoint with a body that carries
		// no transaction; answering 200 keeps the webhook marked healthy.
		if errors.Is(err, checkout.ErrWebhookPing) {
			return nil
		}
		return apperr.Unauthorized(err)
	}

	// Outbound/debit transfers hit the same endpoint; only money coming in
	// settles a booking.
	if !event.Success {
		return nil
	}

	bookingID, err := bookingIDFromMemo(uc.cfg.SePayTransferPrefix, event.ProviderRef)
	if err != nil {
		return apperr.Validation("khong doc duoc booking id tu noi dung chuyen khoan")
	}

	booking, err := uc.bookingRepo.GetByID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.NotFound(err)
		}
		return apperr.Internal(err)
	}

	// The provider redelivers until it gets a 200, so an already-confirmed
	// booking is a success, not an error.
	if booking.Status == model.BookingStatusConfirmed {
		return nil
	}
	if booking.Status != model.BookingStatusPendingPayment {
		return apperr.BookingExpired(nil)
	}
	if event.AmountVND != booking.ComputedPrice {
		return apperr.AmountMismatch(nil)
	}

	payment, err := uc.paymentRepo.GetByBookingID(ctx, booking.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.NotFound(err)
		}
		return apperr.Internal(err)
	}
	marked, err := uc.paymentRepo.MarkPaidIfPending(ctx, payment.ID, event.ExternalRef, time.Now())
	if err != nil {
		// A duplicate external ref means some payment already claimed this
		// provider transaction id; retrying can never make that succeed, so
		// the provider must be told to stop, not to keep sending it back.
		if errors.Is(err, paymentrepo.ErrDuplicateExternalRef) {
			return nil
		}
		return apperr.Internal(err)
	}
	if !marked {
		// The payment was already settled. Two things reach here: a concurrent
		// delivery that won the race, or a delivery whose booking update failed
		// after settlement. Every later redelivery also finds the payment
		// already paid, so without telling these apart the second case would
		// strand the booking at pending_payment forever — nothing else retries it.
		//
		// Re-read to tell them apart. Confirmed means the winner finished, so
		// there is nothing to do. Still pending means this delivery is the
		// recovery: SePay's own retry is the only mechanism that will ever
		// finish the job, so fall through and finish it.
		fresh, ferr := uc.bookingRepo.GetByID(ctx, booking.ID)
		if ferr != nil {
			if errors.Is(ferr, gorm.ErrRecordNotFound) {
				return apperr.NotFound(ferr)
			}
			return apperr.Internal(ferr)
		}
		if fresh.Status != model.BookingStatusPendingPayment {
			return nil
		}
		uc.log.Warn("recovering a settled payment whose booking was never confirmed",
			zap.Uint("booking_id", booking.ID),
			zap.Uint("payment_id", payment.ID),
			zap.String("external_ref", event.ExternalRef))
		booking = fresh
	}

	booking.Status = model.BookingStatusConfirmed
	if err := uc.bookingRepo.Update(ctx, booking); err != nil {
		// The money is already settled, so this must be findable by hand: handleErr
		// logs only the message, with no identifiers.
		uc.log.Error("payment settled but the booking could not be confirmed",
			zap.Uint("booking_id", booking.ID),
			zap.Uint("payment_id", payment.ID),
			zap.String("external_ref", event.ExternalRef),
			zap.Error(err))
		return apperr.Internal(err)
	}

	// Everything below is best-effort: the money has moved and the booking is
	// confirmed, so a side-effect failure must not make the provider retry a
	// webhook that was already fully applied.
	uc.pushCalendarEvent(ctx, booking)
	if err := uc.notifier.BookingConfirmed(ctx, booking); err != nil {
		uc.log.Error("webhook: booking-confirmed notification failed",
			zap.Uint("booking_id", booking.ID), zap.Error(err))
	}

	return nil
}

func (uc *UseCase) pushCalendarEvent(ctx context.Context, b *model.Booking) {
	home, err := uc.homeRepo.GetByID(ctx, b.HomeID)
	if err != nil {
		uc.log.Error("webhook: load home for calendar push failed",
			zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	if home.GoogleCalendarID == "" {
		return
	}

	eventID, err := uc.calendar.CreateEvent(ctx, home.GoogleCalendarID, b)
	if err != nil {
		uc.log.Error("webhook: calendar event create failed",
			zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}

	b.GoogleCalendarEventID = eventID
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		uc.log.Error("webhook: persist calendar event id failed",
			zap.Uint("booking_id", b.ID), zap.String("event_id", eventID), zap.Error(err))
	}
}

// bookingIDFromMemo recovers the booking id from a transfer memo shaped
// "<prefix><id>". Banks uppercase memos and prepend their own noise, so the
// prefix is matched case-insensitively anywhere in the string and only the
// digits immediately following it are read.
func bookingIDFromMemo(prefix, memo string) (uint, error) {
	if prefix == "" {
		return 0, errors.New("billing: empty transfer prefix")
	}
	upper := strings.ToUpper(strings.TrimSpace(memo))
	idx := strings.Index(upper, strings.ToUpper(prefix))
	if idx < 0 {
		return 0, fmt.Errorf("billing: memo %q has no prefix %q", memo, prefix)
	}

	rest := upper[idx+len(prefix):]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, fmt.Errorf("billing: memo %q has no booking id after prefix %q", memo, prefix)
	}

	id, err := strconv.ParseUint(rest[:digits], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("billing: parse booking id from memo %q: %w", memo, err)
	}
	return uint(id), nil
}
