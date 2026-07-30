package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	"github.com/johnquangdev/laverte-home/util"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

type stubAuthUC struct{}

func (stubAuthUC) LoginURL(context.Context) (*presenter.GoogleLoginURLResponse, error) {
	return nil, nil
}
func (stubAuthUC) Callback(context.Context, payload.GoogleCallbackRequest) (*presenter.SessionResponse, error) {
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

type stubBookingAdminUC struct{}

func (stubBookingAdminUC) ListByHomeAndDate(context.Context, uint, time.Time) ([]presenter.AdminBookingResponse, error) {
	return nil, nil
}
func (stubBookingAdminUC) CreateWalkIn(context.Context, payload.CreateWalkInBookingRequest, uint) (*presenter.AdminBookingResponse, error) {
	return nil, nil
}
func (stubBookingAdminUC) Cancel(context.Context, uint) error              { return nil }
func (stubBookingAdminUC) Complete(context.Context, uint) error            { return nil }
func (stubBookingAdminUC) NoShow(context.Context, uint) error              { return nil }
func (stubBookingAdminUC) SetLockCode(context.Context, uint, string) error { return nil }
func (stubBookingAdminUC) SendLockCode(context.Context, uint) error        { return nil }

type stubBillingUC struct{}

func (stubBillingUC) HandleSePayWebhook(context.Context, []byte, http.Header) error { return nil }

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
		BookingAdminUC:    stubBookingAdminUC{},
		BillingUC:         stubBillingUC{},
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
		BookingAdminUC:    stubBookingAdminUC{},
		BillingUC:         stubBillingUC{},
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
