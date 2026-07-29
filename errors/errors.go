package errors

import (
	"errors"
	"net/http"
)

type Code string

const (
	CodeInternal       Code = "INTERNAL_ERROR"
	CodeNotFound       Code = "NOT_FOUND"
	CodeSlotConflict   Code = "SLOT_CONFLICT"
	CodeBookingExpired Code = "BOOKING_EXPIRED"
	CodeAmountMismatch Code = "PAYMENT_AMOUNT_MISMATCH"
	CodeValidation     Code = "VALIDATION_ERROR"
	CodeUnauthorized   Code = "UNAUTHORIZED"
	CodeConflict       Code = "CONFLICT"
)

type Error struct {
	Code     Code
	CodeID   string
	HTTPCode int
	Message  string
	Raw      error
}

func (e *Error) Error() string {
	if e.Raw != nil {
		return e.Message + ": " + e.Raw.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Raw }

// As reports whether err (or something it wraps) is *Error.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func Internal(raw error) *Error {
	return &Error{Code: CodeInternal, CodeID: "internal_error", HTTPCode: http.StatusInternalServerError, Message: "internal error", Raw: raw}
}

func NotFound(raw error) *Error {
	return &Error{Code: CodeNotFound, CodeID: "not_found", HTTPCode: http.StatusNotFound, Message: "khong tim thay", Raw: raw}
}

func Validation(msg string) *Error {
	return &Error{Code: CodeValidation, CodeID: "validation_error", HTTPCode: http.StatusBadRequest, Message: msg}
}

func Unauthorized(raw error) *Error {
	return &Error{Code: CodeUnauthorized, CodeID: "unauthorized", HTTPCode: http.StatusUnauthorized, Message: "chua dang nhap", Raw: raw}
}

func SlotConflict(raw error) *Error {
	return &Error{Code: CodeSlotConflict, CodeID: "slot_conflict", HTTPCode: http.StatusConflict, Message: "khung gio nay vua co nguoi dat, vui long chon gio khac", Raw: raw}
}

func BookingExpired(raw error) *Error {
	return &Error{Code: CodeBookingExpired, CodeID: "booking_expired", HTTPCode: http.StatusGone, Message: "booking da het han giu cho", Raw: raw}
}

func AmountMismatch(raw error) *Error {
	return &Error{Code: CodeAmountMismatch, CodeID: "payment_amount_mismatch", HTTPCode: http.StatusBadRequest, Message: "so tien thanh toan khong khop", Raw: raw}
}

// Conflict is for a request that collides with existing state the caller can see
// and fix — "this already exists" — as opposed to SlotConflict, which is
// specifically a booking overlapping another booking.
func Conflict(raw error, msg string) *Error {
	return &Error{Code: CodeConflict, CodeID: "conflict", HTTPCode: http.StatusConflict, Message: msg, Raw: raw}
}
