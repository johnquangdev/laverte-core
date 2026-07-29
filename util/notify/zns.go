package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

// vnTimeLayout is the day-first format Vietnamese customers and admins read;
// also used by smtp_admin.go.
const vnTimeLayout = "02/01/2006 15:04"

// znsMaxResponseBytes caps the provider response before decoding.
const znsMaxResponseBytes = 64 << 10

// ZNSNotifier is the customer channel: Zalo delivers by phone number, so the
// guest needs neither an account on this system nor an app install.
type ZNSNotifier struct {
	endpoint            string
	accessToken         string
	confirmedTemplateID string
	lockCodeTemplateID  string
	httpClient          *http.Client
}

func NewZNS(cfg *config.Config, httpClient *http.Client) *ZNSNotifier {
	return &ZNSNotifier{
		endpoint:            cfg.ZNSEndpoint,
		accessToken:         cfg.ZNSAccessToken,
		confirmedTemplateID: cfg.ZNSBookingConfirmedTemplateID,
		lockCodeTemplateID:  cfg.ZNSLockCodeTemplateID,
		httpClient:          httpClient,
	}
}

type znsRequest struct {
	Phone        string            `json:"phone"`
	TemplateID   string            `json:"template_id"`
	TemplateData map[string]string `json:"template_data"`
}

type znsResponse struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

func (z *ZNSNotifier) BookingConfirmed(ctx context.Context, b *model.Booking) error {
	return z.send(ctx, b.CustomerPhone, z.confirmedTemplateID, map[string]string{
		"booking_id":    strconv.FormatUint(uint64(b.ID), 10),
		"customer_name": b.CustomerName,
		"start_time":    b.StartTime.Format(vnTimeLayout),
		"price":         strconv.FormatInt(b.ComputedPrice, 10),
	})
}

func (z *ZNSNotifier) LockCode(ctx context.Context, b *model.Booking, code string) error {
	return z.send(ctx, b.CustomerPhone, z.lockCodeTemplateID, map[string]string{
		"booking_id": strconv.FormatUint(uint64(b.ID), 10),
		"lock_code":  code,
		"start_time": b.StartTime.Format(vnTimeLayout),
	})
}

// AdminLockCodeMissing is intentionally inert: ZNS is the customer channel and
// the admin has no Zalo template. NewComposite routes this call to SMTP.
func (z *ZNSNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }

func (z *ZNSNotifier) send(ctx context.Context, phone, templateID string, data map[string]string) error {
	if z.accessToken == "" || templateID == "" {
		return errors.New("notify/zns: access token or template id not configured")
	}

	// model.NormalizeVNPhone is the single canonicalizer the booking usecase
	// and the rate limiter also key on; a second implementation here would
	// drift from it. It reports an unrecognizable number as "", which must be
	// rejected before it reaches Zalo rather than sent as a literal "" phone.
	normalizedPhone := model.NormalizeVNPhone(phone)
	if normalizedPhone == "" {
		// No phone in the message: the caller logs this error, so the number would
		// land in the log store. The booking id the caller already logs is enough
		// to find the row.
		return errors.New("notify/zns: customer phone does not normalize to a Vietnamese number")
	}

	body, err := json.Marshal(znsRequest{
		Phone:        normalizedPhone,
		TemplateID:   templateID,
		TemplateData: data,
	})
	if err != nil {
		return fmt.Errorf("notify/zns: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, z.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify/zns: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", z.accessToken)

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify/zns: post %s: %w", z.endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out znsResponse
	// Cap the body before decoding: this is a network boundary, and the response is a
	// handful of fields.
	if err := json.NewDecoder(io.LimitReader(resp.Body, znsMaxResponseBytes)).Decode(&out); err != nil {
		return fmt.Errorf("notify/zns: decode response: %w", err)
	}
	// ZNS answers HTTP 200 even for rejected sends and reports the real outcome
	// only in the body's error field, so checking the status code alone would
	// silently drop messages.
	if out.Error != 0 {
		return fmt.Errorf("notify/zns: send failed (%d): %s", out.Error, out.Message)
	}
	return nil
}
