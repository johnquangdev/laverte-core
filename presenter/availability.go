package presenter

import "time"

// BusyRange is one window the home cannot be booked for. It carries no reason,
// no customer and no booking id: this is served to anyone who asks, and the only
// thing a guest needs in order to pick another slot is when the property is taken.
type BusyRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// AvailabilityResponse lists busy windows, not free ones. Free time is whatever
// the ranges leave over, and leaving the subtraction to the caller keeps this from
// having to guess a slot granularity that the hourly pricing rule (base hours plus
// whole extra hours) does not have.
//
// Ranges are sorted by start and may overlap each other — a blocked slot can cover
// a booking that already exists.
type AvailabilityResponse struct {
	HomeID uint        `json:"home_id"`
	From   time.Time   `json:"from"`
	To     time.Time   `json:"to"`
	Busy   []BusyRange `json:"busy"`
}
