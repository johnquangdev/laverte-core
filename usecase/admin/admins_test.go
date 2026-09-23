package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
)

type fakeUserRepo struct {
	users    map[uint]*model.User
	roleSets map[uint]string
}

func (f *fakeUserRepo) GetByOAuth(context.Context, string, string) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeUserRepo) GetByID(_ context.Context, id uint) (*model.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *u
	return &cp, nil
}
func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeUserRepo) Create(context.Context, *model.User) error { return nil }
func (f *fakeUserRepo) ListByRole(context.Context, string) ([]*model.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) SetRole(_ context.Context, id uint, role string, _ *uint, _ *time.Time) error {
	if _, ok := f.users[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	f.roleSets[id] = role
	return nil
}

func newRepo() *fakeUserRepo {
	return &fakeUserRepo{
		users:    map[uint]*model.User{9: {ID: 9, Email: "Staff@LaVerte.vn", Role: model.RoleUser}},
		roleSets: map[uint]string{},
	}
}

func TestGrantAdminByEmailResolvesTheUser(t *testing.T) {
	repo := newRepo()
	err := New(repo).GrantAdmin(context.Background(), payload.GrantAdminRequest{Email: " staff@laverte.vn "}, 1)
	if err != nil {
		t.Fatalf("GrantAdmin() error = %v", err)
	}
	if repo.roleSets[9] != model.RoleAdmin {
		t.Errorf("role set = %v, want user 9 made admin", repo.roleSets)
	}
}

func TestGrantAdminByUnknownEmailExplainsTheSignInStep(t *testing.T) {
	repo := newRepo()
	err := New(repo).GrantAdmin(context.Background(), payload.GrantAdminRequest{Email: "nobody@laverte.vn"}, 1)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("error = %v, want a validation error", err)
	}
	if len(repo.roleSets) != 0 {
		t.Errorf("role set = %v, want no change", repo.roleSets)
	}
}

func TestGrantAdminWithNeitherEmailNorIDIsRejected(t *testing.T) {
	repo := newRepo()
	err := New(repo).GrantAdmin(context.Background(), payload.GrantAdminRequest{}, 1)
	if e, ok := apperr.As(err); !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("error = %v, want a validation error", err)
	}
}

func TestGrantAdminByIDStillWorks(t *testing.T) {
	repo := newRepo()
	if err := New(repo).GrantAdmin(context.Background(), payload.GrantAdminRequest{UserID: 9}, 1); err != nil {
		t.Fatalf("GrantAdmin() error = %v", err)
	}
	if repo.roleSets[9] != model.RoleAdmin {
		t.Errorf("role set = %v, want user 9 made admin", repo.roleSets)
	}
}
