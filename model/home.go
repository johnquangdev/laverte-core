package model

import "time"

const (
	HomeCategoryHome = "home"
	HomeCategoryNest = "nest"
)

type Home struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string    `gorm:"not null" json:"name"`
	Category         string    `gorm:"not null" json:"category"`
	Address          string    `json:"address"`
	Description      string    `json:"description"`
	GoogleCalendarID string    `gorm:"column:google_calendar_id" json:"google_calendar_id"`
	IsActive         bool      `gorm:"default:true" json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Home) TableName() string { return "homes" }

func IsValidHomeCategory(c string) bool {
	return c == HomeCategoryHome || c == HomeCategoryNest
}
