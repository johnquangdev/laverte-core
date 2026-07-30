package job

import (
	"context"

	"go.uber.org/zap"
)

func (j *Job) alertMissingLockCodes() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.AlertMissingLockCodes(ctx); err != nil {
		j.log.Error("lock-code alert job failed", zap.Error(err))
	}
}

func (j *Job) sendDueLockCodes() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.SendDueLockCodes(ctx); err != nil {
		j.log.Error("lock-code send job failed", zap.Error(err))
	}
}
