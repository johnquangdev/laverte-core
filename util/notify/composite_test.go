package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/johnquangdev/laverte-home/model"
)

type recordingNotifier struct {
	confirmed  int
	lockCode   int
	adminAlert int
	err        error
}

func (r *recordingNotifier) BookingConfirmed(context.Context, *model.Booking) error {
	r.confirmed++
	return r.err
}

func (r *recordingNotifier) LockCode(context.Context, *model.Booking, string) error {
	r.lockCode++
	return r.err
}

func (r *recordingNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	r.adminAlert++
	return r.err
}

func TestCompositeSendsCustomerMessagesOnlyToCustomerChannel(t *testing.T) {
	customer, admin := &recordingNotifier{}, &recordingNotifier{}
	n := NewComposite(customer, admin)
	ctx := context.Background()
	b := &model.Booking{ID: 1}

	if err := n.BookingConfirmed(ctx, b); err != nil {
		t.Fatalf("BookingConfirmed() error = %v", err)
	}
	if err := n.LockCode(ctx, b, "1234"); err != nil {
		t.Fatalf("LockCode() error = %v", err)
	}

	if customer.confirmed != 1 || customer.lockCode != 1 {
		t.Errorf("customer channel = %+v, want confirmed=1 lockCode=1", customer)
	}
	if admin.confirmed+admin.lockCode+admin.adminAlert != 0 {
		t.Errorf("admin channel = %+v, want no calls", admin)
	}
}

func TestCompositeSendsAdminAlertOnlyToAdminChannel(t *testing.T) {
	customer, admin := &recordingNotifier{}, &recordingNotifier{}
	n := NewComposite(customer, admin)

	if err := n.AdminLockCodeMissing(context.Background(), &model.Booking{ID: 1}); err != nil {
		t.Fatalf("AdminLockCodeMissing() error = %v", err)
	}

	if admin.adminAlert != 1 {
		t.Errorf("admin channel adminAlert = %d, want 1", admin.adminAlert)
	}
	if customer.confirmed+customer.lockCode+customer.adminAlert != 0 {
		t.Errorf("customer channel = %+v, want no calls", customer)
	}
}

func TestCompositePropagatesDelegateError(t *testing.T) {
	wantErr := errors.New("zns unreachable")
	n := NewComposite(&recordingNotifier{err: wantErr}, &recordingNotifier{})

	if err := n.BookingConfirmed(context.Background(), &model.Booking{ID: 1}); !errors.Is(err, wantErr) {
		t.Errorf("BookingConfirmed() error = %v, want %v", err, wantErr)
	}
}
