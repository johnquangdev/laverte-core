package checkout

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
