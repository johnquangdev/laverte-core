package job

import (
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	bookingjobsuc "github.com/johnquangdev/laverte-home/usecase/bookingjobs"
)

type Job struct {
	cron *cron.Cron
	uc   bookingjobsuc.IUseCase
	log  *zap.Logger
}

func New(uc bookingjobsuc.IUseCase, log *zap.Logger) *Job {
	c := cron.New()
	j := &Job{cron: c, uc: uc, log: log}
	// AddFunc only fails on a malformed spec, and these specs are constants.
	_, _ = c.AddFunc("* * * * *", j.expirePendingBookings)
	_, _ = c.AddFunc("*/5 * * * *", j.alertMissingLockCodes)
	_, _ = c.AddFunc("* * * * *", j.sendDueLockCodes)
	return j
}

func (j *Job) Start() { j.cron.Start() }
func (j *Job) Stop()  { j.cron.Stop() }
