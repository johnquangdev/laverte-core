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

// NormalizeVNPhone reduces a Vietnamese mobile number to one canonical
// 84XXXXXXXXX form, or "" if it cannot be one.
//
// Everything that keys on a phone must key on this. Without it "0900000001",
// "+84900000001", "84900000001", "084900000001" and "0084900000001" are five
// different strings, so they are five per-phone rate-limit keys and five separate
// pending-booking lookups — one caller gets five times the intended quota and can
// hold five slots at once.
//
// Prefixes are only stripped while the remainder is longer than the 9-digit
// subscriber number. That length guard is what keeps a real 084x number intact:
// 0842345678 is an assigned Vinaphone number whose subscriber part genuinely is
// 842345678, so blindly stripping a leading "84" would corrupt it.
func NormalizeVNPhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()

	const subscriberDigits = 9
	for len(d) > subscriberDigits {
		switch {
		case strings.HasPrefix(d, "00"): // international dialling prefix
			d = d[2:]
		case strings.HasPrefix(d, "84"): // country code
			d = d[2:]
		case strings.HasPrefix(d, "0"): // national trunk prefix
			d = d[1:]
		default:
			return ""
		}
	}
	if len(d) != subscriberDigits {
		return ""
	}
	return "84" + d
}
