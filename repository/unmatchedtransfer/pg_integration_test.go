package unmatchedtransfer

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-core/internal/testdb"
	"github.com/johnquangdev/laverte-core/model"
)

func TestRecordResolveAndList(t *testing.T) {
	db := testdb.New(t, "unmatchedtransfer")
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()

	admin := &model.User{Email: "admin@test.local", OAuthProvider: "test", OAuthID: "1", Role: model.RoleAdmin}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	newTransfer := func() *model.UnmatchedTransfer {
		return &model.UnmatchedTransfer{
			SePayTransactionRef: "TXN-1", Amount: 980000, Content: "CK TU NGUYEN VAN A",
			Reason: model.UnmatchedReasonNoMemo, ReceivedAt: time.Now(),
		}
	}
	inserted, err := repo.RecordIfNew(ctx, newTransfer())
	if err != nil || !inserted {
		t.Fatalf("first RecordIfNew() = %v, %v; want true, nil", inserted, err)
	}
	// A redelivery of the same provider transaction must not add a row.
	inserted, err = repo.RecordIfNew(ctx, newTransfer())
	if err != nil || inserted {
		t.Fatalf("second RecordIfNew() = %v, %v; want false, nil", inserted, err)
	}

	open, err := repo.List(ctx, true, 10)
	if err != nil || len(open) != 1 {
		t.Fatalf("List(open) = %d rows, %v; want 1", len(open), err)
	}
	id := open[0].ID

	resolved, err := repo.ResolveIfOpen(ctx, id, admin.ID, "da hoan", time.Now())
	if err != nil || !resolved {
		t.Fatalf("ResolveIfOpen() = %v, %v; want true, nil", resolved, err)
	}
	resolved, err = repo.ResolveIfOpen(ctx, id, admin.ID, "lan hai", time.Now())
	if err != nil || resolved {
		t.Fatalf("second ResolveIfOpen() = %v, %v; want false, nil", resolved, err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ResolutionNote != "da hoan" || got.ResolvedAt == nil || got.ResolvedByAdminID == nil {
		t.Errorf("resolved row = %+v, want the first resolution kept", got)
	}

	open, err = repo.List(ctx, true, 10)
	if err != nil || len(open) != 0 {
		t.Errorf("List(open) after resolve = %d rows, %v; want 0", len(open), err)
	}
	all, err := repo.List(ctx, false, 10)
	if err != nil || len(all) != 1 {
		t.Errorf("List(all) = %d rows, %v; want 1", len(all), err)
	}
}
