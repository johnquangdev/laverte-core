package bookingjobs

import "context"

// IUseCase holds the three periodic sweeps driven by delivery/job. Each one is
// a whole-batch operation: it returns an error only when the batch could not be
// read at all.
type IUseCase interface {
	ExpirePendingBookings(ctx context.Context) error
	AlertMissingLockCodes(ctx context.Context) error
	SendDueLockCodes(ctx context.Context) error
}
