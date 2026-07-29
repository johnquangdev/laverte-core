package model

import (
	"strings"
	"time"
)

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

// NormalizeVNPhone reduces a Vietnamese number to one canonical 84XXXXXXXXX form.
//
// Without it "0900000001", "+84900000001" and "84900000001" are three different
// strings, which means three different per-phone rate-limit keys and three separate
// pending-booking lookups — so one caller gets three times the intended quota and can
// hold three slots. Everything that keys on a phone must key on this.
func NormalizeVNPhone(phone string) string {
	var digits []rune
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	d := string(digits)
	switch {
	case strings.HasPrefix(d, "0"):
		return "84" + d[1:]
	case strings.HasPrefix(d, "84"):
		return d
	default:
		return d
	}
}
