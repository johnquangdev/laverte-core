package notify

import (
	"context"

	"github.com/johnquangdev/laverte-core/model"
)

type noopNotifier struct{}

// NewNoop returns an INotifier that does nothing, so a caller can depend on
// the interface before a real delivery channel is wired in.
func NewNoop() INotifier { return noopNotifier{} }

// TODO(ZNS + admin-email adapter): none of these methods send anything yet;
// they exist only to satisfy INotifier until the real adapter is implemented.
func (noopNotifier) BookingConfirmed(context.Context, *model.Booking) error     { return nil }
func (noopNotifier) LockCode(context.Context, *model.Booking, string) error     { return nil }
func (noopNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }
func (noopNotifier) AdminUnmatchedTransfer(context.Context, *model.UnmatchedTransfer) error {
	return nil
}
