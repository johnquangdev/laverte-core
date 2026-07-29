package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type capturedZNS struct {
	accessToken string
	body        znsRequest
}

// znsStub replies with responseBody and hands the captured request back over a
// channel, which gives the assertions a happens-before edge on the handler.
func znsStub(t *testing.T, responseBody string) (*httptest.Server, <-chan capturedZNS) {
	t.Helper()
	ch := make(chan capturedZNS, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got capturedZNS
		got.accessToken = r.Header.Get("access_token")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		ch <- got
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func znsConfig(endpoint string) *config.Config {
	return &config.Config{
		ZNSEndpoint:                   endpoint,
		ZNSAccessToken:                "tok-abc",
		ZNSBookingConfirmedTemplateID: "tpl-confirmed",
		ZNSLockCodeTemplateID:         "tpl-lock",
	}
}

func testBooking() *model.Booking {
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	return &model.Booking{
		ID:            42,
		HomeID:        7,
		CustomerName:  "Nguyen Van A",
		CustomerPhone: "090 123 4567",
		StartTime:     start,
		EndTime:       start.Add(3 * time.Hour),
		BookingType:   model.BookingTypeHourly,
		ComputedPrice: 450000,
	}
}

func TestZNSBookingConfirmedPostsTemplateAndNormalizedPhone(t *testing.T) {
	srv, captured := znsStub(t, `{"error":0,"message":"Success"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	if err := n.BookingConfirmed(context.Background(), testBooking()); err != nil {
		t.Fatalf("BookingConfirmed() error = %v", err)
	}

	got := <-captured
	if got.accessToken != "tok-abc" {
		t.Errorf("access_token header = %q, want tok-abc", got.accessToken)
	}
	if got.body.TemplateID != "tpl-confirmed" {
		t.Errorf("template_id = %q, want tpl-confirmed", got.body.TemplateID)
	}
	if got.body.Phone != "84901234567" {
		t.Errorf("phone = %q, want 84901234567", got.body.Phone)
	}
	if got.body.TemplateData["booking_id"] != "42" {
		t.Errorf("template_data[booking_id] = %q, want 42", got.body.TemplateData["booking_id"])
	}
	if got.body.TemplateData["customer_name"] != "Nguyen Van A" {
		t.Errorf("template_data[customer_name] = %q", got.body.TemplateData["customer_name"])
	}
	if got.body.TemplateData["start_time"] != "01/08/2026 14:00" {
		t.Errorf("template_data[start_time] = %q, want 01/08/2026 14:00", got.body.TemplateData["start_time"])
	}
	if got.body.TemplateData["price"] != "450000" {
		t.Errorf("template_data[price] = %q, want 450000", got.body.TemplateData["price"])
	}
}

func TestZNSErrorFieldFailsDespiteHTTP200(t *testing.T) {
	srv, captured := znsStub(t, `{"error":1,"message":"bad template"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	err := n.BookingConfirmed(context.Background(), testBooking())
	if err == nil {
		t.Fatal("BookingConfirmed() = nil error, want error even though the status was 200")
	}
	if !strings.Contains(err.Error(), "bad template") {
		t.Errorf("error = %v, want it to carry the ZNS message", err)
	}
	<-captured
}

func TestZNSLockCodeSendsTheCode(t *testing.T) {
	srv, captured := znsStub(t, `{"error":0,"message":"Success"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	if err := n.LockCode(context.Background(), testBooking(), "482913"); err != nil {
		t.Fatalf("LockCode() error = %v", err)
	}

	got := <-captured
	if got.body.TemplateID != "tpl-lock" {
		t.Errorf("template_id = %q, want tpl-lock", got.body.TemplateID)
	}
	if got.body.TemplateData["lock_code"] != "482913" {
		t.Errorf("template_data[lock_code] = %q, want 482913", got.body.TemplateData["lock_code"])
	}
	if got.body.TemplateData["start_time"] != "01/08/2026 14:00" {
		t.Errorf("template_data[start_time] = %q, want 01/08/2026 14:00", got.body.TemplateData["start_time"])
	}
}

// TestZNSUnnormalizablePhoneFailsWithoutCallingZNS guards the "" case
// model.NormalizeVNPhone reports for input it cannot canonicalize: the send
// must be rejected locally rather than posting an empty phone to Zalo.
func TestZNSUnnormalizablePhoneFailsWithoutCallingZNS(t *testing.T) {
	srv, captured := znsStub(t, `{"error":0,"message":"Success"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	b := testBooking()
	b.CustomerPhone = "not-a-phone"

	err := n.BookingConfirmed(context.Background(), b)
	if err == nil {
		t.Fatal("BookingConfirmed() = nil error, want error for an unnormalizable phone")
	}
	// The caller logs this error verbatim, so the raw phone must not ride along in it.
	if strings.Contains(err.Error(), b.CustomerPhone) {
		t.Errorf("error = %v, must not contain the raw customer phone", err)
	}

	select {
	case <-captured:
		t.Fatal("ZNS was called despite the phone failing to normalize")
	default:
	}
}
