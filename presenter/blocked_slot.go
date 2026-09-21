package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

type BlockedSlotResponse struct {
	ID               uint      `json:"id"`
	HomeID           uint      `json:"home_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Reason           string    `json:"reason"`
	CreatedByAdminID *uint     `json:"created_by_admin_id"`
}

func ToBlockedSlotResponse(s *model.BlockedSlot) BlockedSlotResponse {
	return BlockedSlotResponse{
		ID: s.ID, HomeID: s.HomeID, StartTime: s.StartTime, EndTime: s.EndTime,
		Reason: s.Reason, CreatedByAdminID: s.CreatedByAdminID,
	}
}
