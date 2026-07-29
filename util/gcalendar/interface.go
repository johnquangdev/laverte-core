package gcalendar

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

// ICalendar pushes and removes a booking's event on a home's shared calendar.
type ICalendar interface {
	CreateEvent(ctx context.Context, calendarID string, b *model.Booking) (string, error)
	DeleteEvent(ctx context.Context, calendarID, eventID string) error
}
