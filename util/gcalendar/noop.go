package gcalendar

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type noopCalendar struct{}

// NewNoop returns an ICalendar that does nothing, so a caller can depend on
// the interface before a real calendar adapter is wired in.
func NewNoop() ICalendar { return noopCalendar{} }

// TODO(Google service-account calendar adapter): CreateEvent never actually
// creates a calendar event; it exists only to satisfy ICalendar until the
// real adapter is implemented.
func (noopCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	return "", nil
}

func (noopCalendar) DeleteEvent(context.Context, string, string) error { return nil }
