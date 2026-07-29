package payload

import "time"

type CreateBlockedSlotRequest struct {
	HomeID    uint      `json:"home_id" validate:"required"`
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`
	Reason    string    `json:"reason"`
}
