package checkout

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
)

const testWebhookSecret = "top-secret"

func testSePay() IPaymentProvider {
	return NewSePay(config.Config{
		SePayBankAccount:    "0123456789",
		SePayBankCode:       "MSB",
		SePayWebhookSecret:  testWebhookSecret,
		SePayTransferPrefix: "LAVERTE",
	})
}

// signedHeaders recomputes the HMAC the same way SePay does, so the test stays
// self-consistent instead of asserting a hard-coded hex digest.
func signedHeaders(body []byte, secret string, at time.Time) http.Header {
	tsStr := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(body)

	h := http.Header{}
	h.Set("X-SePay-Timestamp", tsStr)
	h.Set("X-SePay-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return h
}

func paidWebhookBody() []byte {
	return []byte(`{"id":998877,"accountNumber":"0123456789","code":"LAVERTE42","content":"CT DEN:LAVERTE42 chuyen tien","transferAmount":350000,"transferType":"in"}`)
}

func TestCreateQRBuildsVietQRURLWithBookingMemo(t *testing.T) {
	res, err := testSePay().CreateQR(context.Background(), CreateQRRequest{BookingID: 42, AmountVND: 350000})
	if err != nil {
		t.Fatalf("CreateQR() error = %v", err)
	}
	if res.ProviderRef != "LAVERTE42" {
		t.Errorf("ProviderRef = %q, want LAVERTE42", res.ProviderRef)
	}
	for _, want := range []string{"https://vietqr.app/img?", "acc=0123456789", "bank=MSB", "amount=350000", "des=LAVERTE42"} {
		if !strings.Contains(res.QRContent, want) {
			t.Errorf("QRContent = %q, missing %q", res.QRContent, want)
		}
	}
}

func TestVerifyWebhookAcceptsCorrectlySignedBody(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	ev, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if ev.ProviderRef != "LAVERTE42" {
		t.Errorf("ProviderRef = %q, want LAVERTE42", ev.ProviderRef)
	}
	if ev.ExternalRef != "998877" {
		t.Errorf("ExternalRef = %q, want 998877", ev.ExternalRef)
	}
	if ev.AmountVND != 350000 {
		t.Errorf("AmountVND = %d, want 350000", ev.AmountVND)
	}
	if !ev.Success {
		t.Error("Success = false, want true for transferType \"in\"")
	}
	if ev.Raw != string(body) {
		t.Error("Raw should carry the unmodified body")
	}
}

func TestVerifyWebhookRejectsWrongSecret(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, "attacker-secret", time.Now())

	if _, err := testSePay().VerifyWebhook(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyWebhook() with a body signed by the wrong secret = nil error, want error")
	}
}

func TestVerifyWebhookRejectsStaleTimestamp(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, testWebhookSecret, time.Now().Add(-sepayReplayWindow-time.Minute))

	if _, err := testSePay().VerifyWebhook(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyWebhook() outside the replay window = nil error, want error")
	}
}

func TestVerifyWebhookReturnsPingForDashboardTest(t *testing.T) {
	body := []byte(`{"id":1,"accountNumber":"0000000001","code":"","content":"Giao dich thu nghiem","transferAmount":2000,"transferType":"in"}`)
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	_, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if !errors.Is(err, ErrWebhookPing) {
		t.Fatalf("VerifyWebhook() error = %v, want ErrWebhookPing", err)
	}
}

// TestVerifyWebhookRejectsRemarshalledBody guards the property the whole design
// depends on: the HMAC covers the raw bytes, not a struct GORM/encoding-json
// would re-serialize. It deliberately uses a body whose key order and spacing
// differ from Go's canonical json.Marshal output and asserts the re-marshalled
// bytes actually differ — a same-order, no-whitespace fixture would round-trip
// byte-identical and the test would false-pass without proving anything.
func TestVerifyWebhookRejectsRemarshalledBody(t *testing.T) {
	original := []byte(`{"transferType": "in", "id": 998877, "accountNumber": "0123456789", ` +
		`"code": "LAVERTE42", "content": "CT DEN:LAVERTE42 chuyen tien", "transferAmount": 350000}`)
	headers := signedHeaders(original, testWebhookSecret, time.Now())

	if _, err := testSePay().VerifyWebhook(context.Background(), original, headers); err != nil {
		t.Fatalf("sanity check: correctly-signed original body should verify, got err = %v", err)
	}

	var payload sepayWebhookPayload
	if err := json.Unmarshal(original, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	remarshalled, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	if bytes.Equal(remarshalled, original) {
		t.Fatal("test setup invalid: re-marshalled body is byte-identical to the original, proves nothing")
	}

	if _, err := testSePay().VerifyWebhook(context.Background(), remarshalled, headers); err == nil {
		t.Fatal("VerifyWebhook() on a re-marshalled body against the original signature = nil error, want error")
	}
}

func TestVerifyWebhookFailsClosedWithoutSecret(t *testing.T) {
	sepay := NewSePay(config.Config{
		SePayBankAccount:    "0123456789",
		SePayBankCode:       "MSB",
		SePayWebhookSecret:  "",
		SePayTransferPrefix: "LAVERTE",
	})
	body := paidWebhookBody()
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	if _, err := sepay.VerifyWebhook(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyWebhook() with no webhook secret configured = nil error, want error")
	}
}

// TestVerifyWebhookPrefersMemoOverPingMarker is the regression for the case
// where a real settlement's memo happens to share text with the dashboard
// ping phrase: it must be settled, not silently classified as a ping.
func TestVerifyWebhookPrefersMemoOverPingMarker(t *testing.T) {
	body := []byte(`{"id":55,"accountNumber":"0123456789","code":"","content":"LAVERTE42 giao dich thu nghiem","transferAmount":350000,"transferType":"in"}`)
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	ev, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v, want a settled event (memo present)", err)
	}
	if ev.ProviderRef != "LAVERTE42" {
		t.Errorf("ProviderRef = %q, want LAVERTE42", ev.ProviderRef)
	}
}

// TestVerifyWebhookDetectsPingWhenNoMemo confirms the ping classification is
// still reachable once it is demoted to a last resort: no memo anywhere in
// the payload, content is the marker phrase alone.
func TestVerifyWebhookDetectsPingWhenNoMemo(t *testing.T) {
	body := []byte(`{"id":1,"accountNumber":"0000000001","code":"","content":"Giao dich thu nghiem","transferAmount":2000,"transferType":"in"}`)
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	_, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if !errors.Is(err, ErrWebhookPing) {
		t.Fatalf("VerifyWebhook() error = %v, want ErrWebhookPing", err)
	}
}

// TestParseBookingMemoScansEveryOccurrence covers a memo whose first prefix
// occurrence carries no digits (a payer's free-text note) and whose booking id
// only appears at a later occurrence (the QR's own memo).
func TestParseBookingMemoScansEveryOccurrence(t *testing.T) {
	got := parseBookingMemo("LAVERTE", "tin nhan tu LAVERTE freetext roi den LAVERTE77 chuyen tien")
	if got != "LAVERTE77" {
		t.Errorf("parseBookingMemo() = %q, want LAVERTE77", got)
	}
}

func TestParseBookingMemoSurvivesStrippedPunctuation(t *testing.T) {
	if got := parseBookingMemo("LAVERTE", "CT DEN:LAVERTE42 chuyen tien"); got != "LAVERTE42" {
		t.Errorf("parseBookingMemo() = %q, want LAVERTE42", got)
	}
	if got := parseBookingMemo("LAVERTE", "ck laverte-42"); got != "LAVERTE42" {
		t.Errorf("parseBookingMemo() lowercase+dash = %q, want LAVERTE42", got)
	}
	if got := parseBookingMemo("LAVERTE", "chuyen tien khong co memo"); got != "" {
		t.Errorf("parseBookingMemo() with no memo = %q, want empty", got)
	}
	if got := parseBookingMemo("LAVERTE", "CT DEN:LAVERTE chuyen tien"); got != "" {
		t.Errorf("parseBookingMemo() with prefix but no digits = %q, want empty", got)
	}
}
