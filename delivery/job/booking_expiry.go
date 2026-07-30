package job

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// jobTimeout bounds one tick: the expiry and lock-code sweeps run every minute,
// so a hung run must be cut loose before the next one is due.
const jobTimeout = 30 * time.Second

func (j *Job) expirePendingBookings() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.ExpirePendingBookings(ctx); err != nil {
		j.log.Error("expire pending bookings job failed", zap.Error(err))
	}
}
