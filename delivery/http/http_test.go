package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
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
