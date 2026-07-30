package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/internal/testdb"
	"github.com/johnquangdev/laverte-home/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Own database per package, for the reason internal/testdb documents.
	return testdb.New(t, "user")
}

func seedUser(t *testing.T, db *gorm.DB, email, role string) *model.User {
	t.Helper()
	u := &model.User{
		Email: email, OAuthProvider: "google", OAuthID: email, Role: role,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// TestSetRoleReportsAnIDThatMatchedNothing is the whole point of the RowsAffected
// check: DELETE /admin/admins/<typo> is the only way to revoke an admin, and an UPDATE
// that matched no row is indistinguishable from a successful one at the SQL level.
// Answering success there tells the superadmin the access is gone while it is not.
func TestSetRoleReportsAnIDThatMatchedNothing(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	admin := seedUser(t, db, "admin@laverte.vn", model.RoleAdmin)

	if err := repo.SetRole(ctx, admin.ID, model.RoleUser, nil, nil); err != nil {
		t.Fatalf("SetRole() on an existing user error = %v, want nil", err)
	}
	got, err := repo.GetByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Role != model.RoleUser {
		t.Errorf("Role = %q, want %q", got.Role, model.RoleUser)
	}

	// admin.ID + 1000 is an id no seeded row can have.
	err = repo.SetRole(ctx, admin.ID+1000, model.RoleUser, nil, nil)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("SetRole() on an unknown id error = %v, want gorm.ErrRecordNotFound", err)
	}
}

// TestSetRoleStampsAndClearsTheGrantTrail proves grantedBy/grantedAt travel with
// the role rather than being left behind: a revoked admin that keeps its
// admin_granted_at would keep sorting among the admins in ListByRole's ordering.
func TestSetRoleStampsAndClearsTheGrantTrail(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	granter := seedUser(t, db, "boss@laverte.vn", model.RoleSuperAdmin)
	target := seedUser(t, db, "staff@laverte.vn", model.RoleUser)

	grantedAt := time.Now().Truncate(time.Second)
	if err := repo.SetRole(ctx, target.ID, model.RoleAdmin, &granter.ID, &grantedAt); err != nil {
		t.Fatalf("SetRole() grant error = %v", err)
	}
	got, err := repo.GetByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Role != model.RoleAdmin {
		t.Errorf("Role = %q, want %q", got.Role, model.RoleAdmin)
	}
	if got.AdminGrantedBy == nil || *got.AdminGrantedBy != granter.ID {
		t.Errorf("AdminGrantedBy = %v, want %d", got.AdminGrantedBy, granter.ID)
	}
	if got.AdminGrantedAt == nil {
		t.Error("AdminGrantedAt = nil, want the grant timestamp")
	}

	if err = repo.SetRole(ctx, target.ID, model.RoleUser, nil, nil); err != nil {
		t.Fatalf("SetRole() revoke error = %v", err)
	}
	got, err = repo.GetByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.AdminGrantedBy != nil || got.AdminGrantedAt != nil {
		t.Errorf("grant trail = %v/%v after revoke, want both nil", got.AdminGrantedBy, got.AdminGrantedAt)
	}
}
