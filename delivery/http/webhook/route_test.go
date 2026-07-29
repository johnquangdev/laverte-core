package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	apperr "github.com/johnquangdev/laverte-home/errors"
)

// fakeUC records the exact bytes it received so a test can prove the handler
// forwards the raw body unmodified — VerifyWebhook's HMAC covers those exact
// bytes, so any re-encoding before this call would invalidate every signature.
type fakeUC struct {
	gotRaw []byte
	err    error
}

func (f *fakeUC) HandleSePayWebhook(_ context.Context, raw []byte, _ http.Header) error {
	f.gotRaw = raw
	return f.err
}

func passErr(c echo.Context, err error) error {
	if e, ok := apperr.As(err); ok {
		return c.JSON(e.HTTPCode, map[string]string{"code": string(e.Code)})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"code": "internal"})
}
func passOK(c echo.Context, data any) error { return c.JSON(http.StatusOK, data) }

func newRouter(uc *fakeUC) *echo.Echo {
	e := echo.New()
	Init(e.Group(""), uc, passErr, passOK)
	return e
}

func doWebhookRequest(e *echo.Echo, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/sepay", strings.NewReader(body))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestSepayForwardsRawBodyUnmodified proves the handler passes the exact
// bytes the provider sent, not a re-marshaled struct.
func TestSepayForwardsRawBodyUnmodified(t *testing.T) {
	body := `{"gateway":"ACB","content":"LAVERTE42","transferAmount":300000}`
	uc := &fakeUC{}
	rec := doWebhookRequest(newRouter(uc), body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if string(uc.gotRaw) != body {
		t.Errorf("usecase received body = %q, want %q", uc.gotRaw, body)
	}
}

// TestSepayReturnsOKOnNilError proves a nil usecase error answers 200.
func TestSepayReturnsOKOnNilError(t *testing.T) {
	uc := &fakeUC{}
	rec := doWebhookRequest(newRouter(uc), `{}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// TestSepayReturnsErrorStatusOnAppError proves an apperr from the usecase is
// translated to its HTTPCode by the injected error handler, not swallowed
// into a 200.
func TestSepayReturnsErrorStatusOnAppError(t *testing.T) {
	uc := &fakeUC{err: apperr.AmountMismatch(nil)}
	rec := doWebhookRequest(newRouter(uc), `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// TestSepayReturnsInternalStatusOnNonAppError proves a raw (non-apperr)
// error from the usecase falls through to the handleErr default, not a 200.
func TestSepayReturnsInternalStatusOnNonAppError(t *testing.T) {
	uc := &fakeUC{err: errors.New("boom")}
	rec := doWebhookRequest(newRouter(uc), `{}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body=%s", rec.Code, rec.Body.String())
	}
}
