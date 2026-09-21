package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/config"
	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/util/oauth"
	"github.com/johnquangdev/laverte-core/util/tokenstore"
)

type fakeUserRepo struct{ users map[uint]*model.User }

func (f *fakeUserRepo) GetByOAuth(context.Context, string, string) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeUserRepo) GetByID(_ context.Context, id uint) (*model.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}
func (f *fakeUserRepo) Create(_ context.Context, u *model.User) error                  { f.users[u.ID] = u; return nil }
func (f *fakeUserRepo) ListByRole(context.Context, string) ([]*model.User, error)      { return nil, nil }
func (f *fakeUserRepo) SetRole(context.Context, uint, string, *uint, *time.Time) error { return nil }

type fakeTokenRepo struct {
	byID   map[string]*model.RefreshToken
	family map[string]bool // true once revoked
}

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{byID: map[string]*model.RefreshToken{}, family: map[string]bool{}}
}
func (f *fakeTokenRepo) Create(_ context.Context, t *model.RefreshToken) error {
	f.byID[t.TokenID] = t
	return nil
}
func (f *fakeTokenRepo) GetByTokenID(_ context.Context, tokenID string) (*model.RefreshToken, error) {
	t, ok := f.byID[tokenID]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}
func (f *fakeTokenRepo) Revoke(_ context.Context, tokenID string) error {
	now := time.Now()
	f.byID[tokenID].RevokedAt = &now
	return nil
}
func (f *fakeTokenRepo) RevokeFamily(_ context.Context, familyID string) error {
	f.family[familyID] = true
	for _, t := range f.byID {
		if t.FamilyID == familyID {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	return nil
}

func TestRefreshTokenReuseRevokesFamily(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "a", JWTRefreshSecret: "r", JWTAccessTTLMinutes: 15, JWTRefreshTTLDays: 30}
	userRepo := &fakeUserRepo{users: map[uint]*model.User{1: {ID: 1, Email: "a@b.com"}}}
	tokenRepo := newFakeTokenRepo()
	uc := New(userRepo, tokenRepo, nil, nil, cfg, zap.NewNop()).(*UseCase)

	session, err := uc.issueTokenPair(context.Background(), userRepo.users[1], "fam-1")
	if err != nil {
		t.Fatalf("issueTokenPair() error = %v", err)
	}

	if _, err := uc.RefreshToken(context.Background(), session.RefreshToken); err != nil {
		t.Fatalf("first RefreshToken() error = %v", err)
	}

	// A replayed refresh token means the token leaked, so rejecting this one call
	// is not enough — every token in its family has to die with it.
	if _, err := uc.RefreshToken(context.Background(), session.RefreshToken); err == nil {
		t.Fatal("second RefreshToken() with reused token = nil error, want error")
	}
	if !tokenRepo.family["fam-1"] {
		t.Error("expected family fam-1 to be revoked after reuse")
	}
}

// A bad refresh token is routine client behaviour, not a server fault: it has to
// surface as 401, or handleErr will classify it as an internal error and answer 500.
func TestRefreshTokenBadTokenIsUnauthorized(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "a", JWTRefreshSecret: "r", JWTAccessTTLMinutes: 15, JWTRefreshTTLDays: 30}
	uc := New(&fakeUserRepo{users: map[uint]*model.User{}}, newFakeTokenRepo(), nil, nil, cfg, zap.NewNop())

	_, err := uc.RefreshToken(context.Background(), "not-a-jwt")
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("error = %v, want an *apperr.Error", err)
	}
	if e.HTTPCode != http.StatusUnauthorized {
		t.Errorf("HTTPCode = %d, want 401", e.HTTPCode)
	}
}

var _ = oauth.IOAuthProvider(nil)
var _ = tokenstore.ITokenStore(nil)
