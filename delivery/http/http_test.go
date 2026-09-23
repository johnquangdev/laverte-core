package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-core/config"
	jwtmw "github.com/johnquangdev/laverte-core/delivery/http/middleware"
	"github.com/johnquangdev/laverte-core/payload"
	"github.com/johnquangdev/laverte-core/presenter"
	"github.com/johnquangdev/laverte-core/util"
	"github.com/johnquangdev/laverte-core/util/ratelimit"
)

type stubAuthUC struct{}

func (stubAuthUC) LoginURL(context.Context) (*presenter.GoogleLoginURLResponse, error) {
	return nil, nil
}
func (stubAuthUC) Callback(context.Context, payload.GoogleCallbackRequest) (*presenter.SessionResponse, error) {
	return nil, nil
}
func (stubAuthUC) PasswordLogin(context.Context, payload.PasswordLoginRequest) (*presenter.SessionResponse, error) {
	return nil, nil
}
func (stubAuthUC) RefreshToken(context.Context, string) (*presenter.SessionResponse, error) {
	return nil, nil
}
func (stubAuthUC) Logout(context.Context, string) error { return nil }

type stubAdminUC struct{}

func (stubAdminUC) ListAdmins(context.Context) ([]presenter.AdminListItemResponse, error) {
	return nil, nil
}
func (stubAdminUC) GrantAdmin(context.Context, payload.GrantAdminRequest, uint) error { return nil }
func (stubAdminUC) RevokeAdmin(context.Context, uint) error                           { return nil }

type stubHomeAdminUC struct{}

func (stubHomeAdminUC) Create(context.Context, payload.CreateHomeRequest) (*presenter.HomeResponse, error) {
	return nil, nil
}
func (stubHomeAdminUC) Update(context.Context, uint, payload.UpdateHomeRequest) (*presenter.HomeResponse, error) {
	return nil, nil
}
func (stubHomeAdminUC) List(context.Context) ([]presenter.HomeResponse, error) { return nil, nil }
func (stubHomeAdminUC) ListActive(context.Context) ([]presenter.PublicHomeResponse, error) {
	return nil, nil
}

type stubPricingAdminUC struct{}

func (stubPricingAdminUC) Create(context.Context, payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	return nil, nil
}
func (stubPricingAdminUC) Supersede(context.Context, uint, payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	return nil, nil
}
func (stubPricingAdminUC) ListByCategory(context.Context, string) ([]presenter.PricingRuleResponse, error) {
	return nil, nil
}

type stubBlockedSlotUC struct{}

func (stubBlockedSlotUC) Create(context.Context, payload.CreateBlockedSlotRequest, uint) (*presenter.BlockedSlotResponse, error) {
	return nil, nil
}
func (stubBlockedSlotUC) Delete(context.Context, uint) error { return nil }
func (stubBlockedSlotUC) ListByHome(context.Context, uint) ([]presenter.BlockedSlotResponse, error) {
	return nil, nil
}

type stubBookingUC struct{}

func (stubBookingUC) Create(context.Context, payload.CreateBookingRequest) (*presenter.BookingResponse, error) {
	return nil, nil
}

func (stubBookingUC) Availability(context.Context, uint, time.Time, time.Time) (*presenter.AvailabilityResponse, error) {
	return nil, nil
}

// stubBookingAdminUC records the day it was handed so a router test can assert
// which clock the handler parsed ?date= in.
type stubBookingAdminUC struct{ gotDay time.Time }

func (s *stubBookingAdminUC) ListByHomeAndDate(_ context.Context, _ uint, day time.Time) ([]presenter.AdminBookingResponse, error) {
	s.gotDay = day
	return nil, nil
}
func (*stubBookingAdminUC) CreateWalkIn(context.Context, payload.CreateWalkInBookingRequest, uint) (*presenter.AdminBookingResponse, error) {
	return nil, nil
}
func (*stubBookingAdminUC) Cancel(context.Context, uint) error              { return nil }
func (*stubBookingAdminUC) Complete(context.Context, uint) error            { return nil }
func (*stubBookingAdminUC) NoShow(context.Context, uint) error              { return nil }
func (*stubBookingAdminUC) SetLockCode(context.Context, uint, string) error { return nil }
func (*stubBookingAdminUC) SendLockCode(context.Context, uint) error        { return nil }

type stubBillingUC struct{}

func (stubBillingUC) HandleSePayWebhook(context.Context, []byte, http.Header) error { return nil }

// stubOverviewUC records the range it was handed, for the same reason.
type stubOverviewUC struct{ gotFrom, gotTo, gotMonth time.Time }

func (s *stubOverviewUC) Summary(_ context.Context, from, to time.Time) (*presenter.OverviewResponse, error) {
	s.gotFrom, s.gotTo = from, to
	return &presenter.OverviewResponse{}, nil
}

func (s *stubOverviewUC) Breakdown(_ context.Context, month time.Time) (*presenter.OverviewBreakdownResponse, error) {
	s.gotMonth = month
	return &presenter.OverviewBreakdownResponse{}, nil
}

type stubPaymentAdminUC struct{ gotFrom, gotTo time.Time }

func (s *stubPaymentAdminUC) List(_ context.Context, from, to time.Time) ([]presenter.AdminPaymentResponse, error) {
	s.gotFrom, s.gotTo = from, to
	return nil, nil
}
func (*stubPaymentAdminUC) Refund(context.Context, uint, uint, string) (*presenter.AdminPaymentResponse, error) {
	return &presenter.AdminPaymentResponse{}, nil
}
func (*stubPaymentAdminUC) ListUnmatched(context.Context, bool) ([]presenter.UnmatchedTransferResponse, error) {
	return nil, nil
}
func (*stubPaymentAdminUC) ResolveUnmatched(context.Context, uint, uint, string) (*presenter.UnmatchedTransferResponse, error) {
	return &presenter.UnmatchedTransferResponse{}, nil
}

// stubTokenStore reports nothing revoked, so router tests need no Redis.
type stubTokenStore struct{}

func (stubTokenStore) SaveState(context.Context, string) error             { return nil }
func (stubTokenStore) ValidateState(context.Context, string) (bool, error) { return true, nil }
func (stubTokenStore) BlacklistToken(context.Context, string) error        { return nil }
func (stubTokenStore) IsBlacklisted(context.Context, string) (bool, error) { return false, nil }

// newTestServer builds a router with named-field Deps so it needs no real
// database, Redis, or OAuth provider to exercise routing and middleware wiring.
func newTestServer() *Server {
	return NewServer(config.Config{FrontendURL: "http://localhost:3000"}, zap.NewNop(), Deps{
		Limiter:           ratelimit.NewMemory(),
		TokenStore:        stubTokenStore{},
		AuthUC:            stubAuthUC{},
		AdminUC:           stubAdminUC{},
		AdminRoleResolver: jwtmw.AdminRoleResolverFunc(func(context.Context, uint) (string, error) { return "", nil }),
		HomeAdminUC:       stubHomeAdminUC{},
		PricingAdminUC:    stubPricingAdminUC{},
		BlockedSlotUC:     stubBlockedSlotUC{},
		BookingUC:         stubBookingUC{},
		BookingAdminUC:    &stubBookingAdminUC{},
		BillingUC:         stubBillingUC{},
		OverviewUC:        &stubOverviewUC{},
		PaymentAdminUC:    &stubPaymentAdminUC{},
	})
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// TestAdminHomeCreateRejectsEmptyBody proves the validator wired into Echo via
// e.Validator actually runs: the `validate:"required"` tags on
// CreateHomeRequest are decoration unless something invokes them, and removing
// the e.Validator line in NewServer previously let an empty body through as if
// it were valid.
func TestAdminHomeCreateRejectsEmptyBody(t *testing.T) {
	cfg := config.Config{
		FrontendURL: "http://localhost:3000", JWTAccessSecret: "test-secret", AdminUserIDs: []uint{7},
		RateLimitAuthedPerMin: 100,
	}
	srv := NewServer(cfg, zap.NewNop(), Deps{
		Limiter:           ratelimit.NewMemory(),
		TokenStore:        stubTokenStore{},
		AuthUC:            stubAuthUC{},
		AdminUC:           stubAdminUC{},
		AdminRoleResolver: jwtmw.AdminRoleResolverFunc(func(context.Context, uint) (string, error) { return "", nil }),
		HomeAdminUC:       stubHomeAdminUC{},
		PricingAdminUC:    stubPricingAdminUC{},
		BlockedSlotUC:     stubBlockedSlotUC{},
		BookingUC:         stubBookingUC{},
		BookingAdminUC:    &stubBookingAdminUC{},
		BillingUC:         stubBillingUC{},
		OverviewUC:        &stubOverviewUC{},
		PaymentAdminUC:    &stubPaymentAdminUC{},
	})

	token, err := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/homes", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// testZone is deliberately an offset no host runs on, so a handler that fell back
// to time.Now().Location() cannot accidentally agree with it.
var testZone = time.FixedZone("TESTZONE", 13*60*60)

// adminServer wires a router whose admin date parsing must use testZone, and hands
// back the recording stubs so a test can read what the handlers computed.
func adminServer(t *testing.T) (*Server, *stubBookingAdminUC, *stubOverviewUC, string) {
	srv, bookings, overview, _, token := adminServerWithPayments(t)
	return srv, bookings, overview, token
}

func adminServerWithPayments(t *testing.T) (*Server, *stubBookingAdminUC, *stubOverviewUC, *stubPaymentAdminUC, string) {
	t.Helper()
	cfg := config.Config{
		FrontendURL: "http://localhost:3000", JWTAccessSecret: "test-secret", AdminUserIDs: []uint{7},
		RateLimitAuthedPerMin: 100,
	}
	cfg.SePayWebhookSecret = "never-leaves-the-server"
	bookings := &stubBookingAdminUC{}
	overview := &stubOverviewUC{}
	payments := &stubPaymentAdminUC{}
	srv := NewServer(cfg, zap.NewNop(), Deps{
		Limiter:           ratelimit.NewMemory(),
		TokenStore:        stubTokenStore{},
		AuthUC:            stubAuthUC{},
		AdminUC:           stubAdminUC{},
		AdminRoleResolver: jwtmw.AdminRoleResolverFunc(func(context.Context, uint) (string, error) { return "", nil }),
		HomeAdminUC:       stubHomeAdminUC{},
		PricingAdminUC:    stubPricingAdminUC{},
		BlockedSlotUC:     stubBlockedSlotUC{},
		BookingUC:         stubBookingUC{},
		BookingAdminUC:    bookings,
		BillingUC:         stubBillingUC{},
		OverviewUC:        overview,
		PaymentAdminUC:    payments,
		Location:          testZone,
	})
	token, err := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	return srv, bookings, overview, payments, token
}

func adminGET(t *testing.T, srv *Server, token, target string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200 (body: %s)", target, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestAdminBookingListParsesDateInConfiguredZone pins which clock a ?date= means.
// Nothing in the deployment sets TZ, so on a UTC container a date parsed in the
// host's zone returns a 24-hour window shifted by the business's UTC offset — the
// admin sees a day's bookings that is not the day they asked for.
func TestAdminBookingListParsesDateInConfiguredZone(t *testing.T) {
	srv, bookings, _, token := adminServer(t)

	adminGET(t, srv, token, "/api/v1/admin/bookings?home_id=1&date=2026-08-01")

	want := time.Date(2026, 8, 1, 0, 0, 0, 0, testZone)
	if !bookings.gotDay.Equal(want) {
		t.Errorf("day = %v, want %v (midnight in the configured zone)", bookings.gotDay, want)
	}
}

// TestAdminOverviewParsesRangeInConfiguredZone is the same proof for the revenue
// range, where the shift moves takings between periods at both ends.
func TestAdminOverviewParsesRangeInConfiguredZone(t *testing.T) {
	srv, _, overview, token := adminServer(t)

	adminGET(t, srv, token, "/api/v1/admin/overview?from=2026-08-01&to=2026-09-01")

	wantFrom := time.Date(2026, 8, 1, 0, 0, 0, 0, testZone)
	wantTo := time.Date(2026, 9, 1, 0, 0, 0, 0, testZone)
	if !overview.gotFrom.Equal(wantFrom) {
		t.Errorf("from = %v, want %v", overview.gotFrom, wantFrom)
	}
	if !overview.gotTo.Equal(wantTo) {
		t.Errorf("to = %v, want %v", overview.gotTo, wantTo)
	}
}

// The default month-to-date window is built from time.Now() rather than a query
// param, so it needs its own guard: a host-zone "now" can put the boundaries in the
// wrong month entirely near a month edge.
func TestAdminOverviewDefaultWindowUsesConfiguredZone(t *testing.T) {
	srv, _, overview, token := adminServer(t)

	adminGET(t, srv, token, "/api/v1/admin/overview")

	now := time.Now().In(testZone)
	wantFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, testZone)
	if !overview.gotFrom.Equal(wantFrom) {
		t.Errorf("from = %v, want %v (first of the month in the configured zone)", overview.gotFrom, wantFrom)
	}
	if _, offset := overview.gotTo.Zone(); offset != 13*60*60 {
		t.Errorf("to offset = %ds, want the configured zone's 46800s", offset)
	}
}

func TestAdminOverviewBreakdownParsesMonthInConfiguredZone(t *testing.T) {
	srv, _, overview, token := adminServer(t)

	adminGET(t, srv, token, "/api/v1/admin/overview/breakdown?month=2026-08")

	want := time.Date(2026, 8, 1, 0, 0, 0, 0, testZone)
	if !overview.gotMonth.Equal(want) {
		t.Errorf("month = %v, want %v", overview.gotMonth, want)
	}
}

func TestAdminPaymentsParsesRangeInConfiguredZone(t *testing.T) {
	srv, _, _, payments, token := adminServerWithPayments(t)

	adminGET(t, srv, token, "/api/v1/admin/payments?from=2026-08-01&to=2026-08-08")

	if want := time.Date(2026, 8, 1, 0, 0, 0, 0, testZone); !payments.gotFrom.Equal(want) {
		t.Errorf("from = %v, want %v", payments.gotFrom, want)
	}
	if want := time.Date(2026, 8, 8, 0, 0, 0, 0, testZone); !payments.gotTo.Equal(want) {
		t.Errorf("to = %v, want %v", payments.gotTo, want)
	}
}

// The settings view is admin-only and must report a secret's presence, never
// its value.
func TestAdminSettingsNeverEchoesSecrets(t *testing.T) {
	srv, _, _, token := adminServer(t)

	body := adminGET(t, srv, token, "/api/v1/admin/settings")

	if strings.Contains(body, "never-leaves-the-server") {
		t.Fatalf("settings body leaks the webhook secret: %s", body)
	}
	if !strings.Contains(body, `"webhook_secret_set":true`) {
		t.Errorf("settings body = %s, want webhook_secret_set true", body)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /admin/settings status = %d, want 401", rec.Code)
	}
}

func TestSplitOrigins(t *testing.T) {
	got := splitOrigins("http://a.com, http://b.com,,http://c.com")
	want := []string{"http://a.com", "http://b.com", "http://c.com"}
	if len(got) != len(want) {
		t.Fatalf("splitOrigins() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
