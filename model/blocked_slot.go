package model

import "time"

type BlockedSlot struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	HomeID           uint      `gorm:"not null;index" json:"home_id"`
	StartTime        time.Time `gorm:"not null" json:"start_time"`
	EndTime          time.Time `gorm:"not null" json:"end_time"`
	Reason           string    `json:"reason"`
	CreatedByAdminID *uint     `json:"created_by_admin_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func (BlockedSlot) TableName() string { return "blocked_slots" }
