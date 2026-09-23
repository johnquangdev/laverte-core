package checkout

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/johnquangdev/laverte-core/config"
)

type sepayProvider struct {
	bankAccount    string
	bankCode       string
	webhookSecret  string
	transferPrefix string
}

func NewSePay(cfg config.Config) IPaymentProvider {
	return &sepayProvider{
		bankAccount:    cfg.SePayBankAccount,
		bankCode:       cfg.SePayBankCode,
		webhookSecret:  cfg.SePayWebhookSecret,
		transferPrefix: cfg.SePayTransferPrefix,
	}
}

// alphanumUpper keeps only what reliably survives a bank transfer's content
// field: banking apps strip or reject punctuation, and some echo the memo back
// uppercased, so a same-memo lookup must not depend on either.
func alphanumUpper(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bookingMemo derives the transfer memo for a booking (e.g. "LAVERTE42").
// Deriving it from the booking ID instead of a random nonce means a webhook can
// be matched back to its booking without a lookup table.
func bookingMemo(prefix string, bookingID uint) string {
	return alphanumUpper(prefix) + strconv.FormatUint(uint64(bookingID), 10)
}

// parseBookingMemo recovers a bookingMemo from a bank's transfer content, which
// arrives wrapped in free text and with punctuation gone ("CT DEN:LAVERTE42
// chuyen tien"). The digit run stops at the first non-digit so trailing words
// can't be absorbed into the booking id. Returns "" when nothing matches.
func parseBookingMemo(prefix, content string) string {
	cleanPrefix := alphanumUpper(prefix)
	if cleanPrefix == "" {
		return ""
	}
	cleaned := alphanumUpper(content)

	// Scan every occurrence, not just the first: bank memo text is free-form and can
	// carry the prefix more than once (a payer note plus the QR's own memo), and
	// stopping at the first one would miss the real booking id entirely.
	for offset := 0; ; {
		i := strings.Index(cleaned[offset:], cleanPrefix)
		if i < 0 {
			return ""
		}
		rest := cleaned[offset+i+len(cleanPrefix):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end > 0 {
			return cleanPrefix + rest[:end]
		}
		offset += i + len(cleanPrefix)
	}
}

// CreateQR returns a URL-based QR renderer rather than a hand-rolled EMVCo
// payload. vietqr.app/img is the endpoint SePay's own docs document for this
// (developer.sepay.vn/vi/tien-ich-khac/tao-qr-code).
func (p *sepayProvider) CreateQR(_ context.Context, req CreateQRRequest) (*QRResult, error) {
	if p.bankAccount == "" || p.bankCode == "" {
		return nil, errors.New("sepay: bank account/code not configured")
	}
	// Without a prefix the memo would be bare digits, which any unrelated
	// transfer whose content happens to contain that number would match.
	if alphanumUpper(p.transferPrefix) == "" {
		return nil, errors.New("sepay: transfer prefix not configured")
	}

	memo := bookingMemo(p.transferPrefix, req.BookingID)
	qrURL := fmt.Sprintf("https://vietqr.app/img?acc=%s&bank=%s&amount=%d&des=%s",
		url.QueryEscape(p.bankAccount), url.QueryEscape(p.bankCode), req.AmountVND, url.QueryEscape(memo))
	return &QRResult{QRContent: qrURL, ProviderRef: memo}, nil
}

// sepayWebhookPayload is the subset of SePay's webhook body settlement needs.
// transferType is "in" for a credit (money received) and "out" for a debit.
// ID is documented as the value that does not change across retries and
// replays, i.e. stable per redelivery but unique per actual transfer — unlike
// Content, which a second real transfer can legitimately repeat.
type sepayWebhookPayload struct {
	ID             int64  `json:"id"`
	AccountNumber  string `json:"accountNumber"`
	Code           string `json:"code"`
	Content        string `json:"content"`
	TransferAmount int64  `json:"transferAmount"`
	TransferType   string `json:"transferType"`
}

// sepayTestWebhookContentMarker is the fixed Vietnamese phrase SePay's
// dashboard "Test" button always sends as its connectivity-check content.
// accountNumber can't be used to detect the ping — it reuses a plausible real
// account — and without this check the ping passes HMAC verification, then
// falls through to a memo lookup that never matches and surfaces as a
// misleading "malformed payload" instead of the harmless ping it is.
const sepayTestWebhookContentMarker = "giao dich thu nghiem"

func isSePayWebhookPing(payload sepayWebhookPayload) bool {
	return strings.Contains(strings.ToLower(payload.Content), sepayTestWebhookContentMarker)
}

// sepayReplayWindow bounds how stale a signed webhook's timestamp may be —
// SePay's docs (developer.sepay.vn/vi/sepay-webhooks/xac-thuc) treat requests
// more than 5 minutes off the current time as a replay risk.
const sepayReplayWindow = 5 * time.Minute

// verifySePayHMAC implements SePay's recommended webhook auth scheme:
// HMAC-SHA256 over "<unix-timestamp>.<raw body>", sent as
// "X-SePay-Signature: sha256=<hex>" alongside "X-SePay-Timestamp". Must run
// against the raw, unparsed body — re-marshaling would silently break the
// signature on any key-order or whitespace difference.
func verifySePayHMAC(raw []byte, timestampHeader, signatureHeader, secret string) error {
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return errors.New("sepay: missing or malformed X-SePay-Timestamp")
	}
	if age := time.Since(time.Unix(ts, 0)); age > sepayReplayWindow || age < -sepayReplayWindow {
		return errors.New("sepay: webhook timestamp outside replay window")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestampHeader))
	mac.Write([]byte("."))
	mac.Write(raw)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(signatureHeader)) != 1 {
		return errors.New("sepay: invalid webhook signature")
	}
	return nil
}

// VerifyWebhook accepts only HMAC-SHA256-authenticated deliveries: SePay also
// offers API-key and OAuth2 modes, but only HMAC binds the raw payload to a
// timestamp and so protects integrity and stale replay at once. Configure the
// SePay dashboard endpoint accordingly.
//
// Code is preferred over Content for the memo because SePay documents content
// as the unprocessed bank memo while code is the field it extracted per the
// merchant's payment-code configuration.
func (p *sepayProvider) VerifyWebhook(_ context.Context, raw []byte, headers http.Header) (*WebhookEvent, error) {
	if p.webhookSecret == "" {
		return nil, errors.New("sepay: webhook secret not configured")
	}

	if err := verifySePayHMAC(raw, headers.Get("X-SePay-Timestamp"), headers.Get("X-SePay-Signature"), p.webhookSecret); err != nil {
		return nil, err
	}

	var payload sepayWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("sepay: malformed webhook payload")
	}

	memo := parseBookingMemo(p.transferPrefix, payload.Code)
	if memo == "" {
		memo = parseBookingMemo(p.transferPrefix, payload.Content)
	}

	// Ping detection runs only when nothing parseable was found. The marker is a
	// substring match on the same free-text field the memo comes from, so checking
	// it first would silently discard a real settlement whose bank memo happened to
	// contain the phrase — money received, booking never confirmed, nothing logged.
	// A genuine settlement always carries a memo; the dashboard ping never does.
	event := &WebhookEvent{
		ProviderRef: memo,
		ExternalRef: strconv.FormatInt(payload.ID, 10),
		Success:     payload.TransferType == "in",
		AmountVND:   payload.TransferAmount,
		Content:     payload.Content,
		Raw:         string(raw),
	}
	if memo == "" {
		if isSePayWebhookPing(payload) {
			return nil, ErrWebhookPing
		}
		return event, ErrNoBookingMemo
	}
	return event, nil
}
