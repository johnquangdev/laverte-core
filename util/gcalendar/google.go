package gcalendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type googleCalendar struct {
	svc *calendar.Service
	cfg *config.Config
}

// NewGoogle authenticates with a service-account key so the backend can push
// events unattended — no consent screen, no user refresh token to expire. The
// error return (rather than a panic) is deliberate: the caller degrades to the
// noop implementation because Calendar push is best-effort and must never
// block booking confirmation.
func NewGoogle(cfg *config.Config) (ICalendar, error) {
	if strings.TrimSpace(cfg.GoogleCalendarCredentialsJSON) == "" {
		return nil, errors.New("gcalendar: GOOGLE_CALENDAR_CREDENTIALS_JSON is empty")
	}

	// context.Background rather than a request context: the service outlives
	// any single request and uses this context to refresh its access token.
	// CredentialsFromJSONWithType (rather than the untyped CredentialsFromJSON)
	// rejects a config whose type isn't service_account, since this JSON comes
	// from an env var an operator controls, never from an external caller.
	ctx := context.Background()
	creds, err := google.CredentialsFromJSONWithType(ctx, []byte(cfg.GoogleCalendarCredentialsJSON), google.ServiceAccount, calendar.CalendarScope)
	if err != nil {
		return nil, fmt.Errorf("gcalendar: parse service account credentials: %w", err)
	}

	svc, err := calendar.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("gcalendar: new calendar service: %w", err)
	}
	return &googleCalendar{svc: svc, cfg: cfg}, nil
}

func (g *googleCalendar) CreateEvent(ctx context.Context, calendarID string, b *model.Booking) (string, error) {
	if calendarID == "" {
		return "", fmt.Errorf("gcalendar: home of booking %d has no google_calendar_id", b.ID)
	}

	created, err := g.svc.Events.Insert(calendarID, buildEvent(g.cfg, b)).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gcalendar: insert event for booking %d: %w", b.ID, err)
	}
	return created.Id, nil
}

func (g *googleCalendar) DeleteEvent(ctx context.Context, calendarID, eventID string) error {
	if calendarID == "" || eventID == "" {
		return nil
	}

	err := g.svc.Events.Delete(calendarID, eventID).Context(ctx).Do()
	if err == nil {
		return nil
	}

	// 404/410 means the event is already gone — a second cancel, or an admin
	// who deleted it by hand. The desired end state is reached, so surfacing
	// this as a failure would make cancellation non-idempotent.
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && (apiErr.Code == http.StatusNotFound || apiErr.Code == http.StatusGone) {
		return nil
	}
	return fmt.Errorf("gcalendar: delete event %s: %w", eventID, err)
}

// buildEvent is split out of CreateEvent so the summary/description/timezone
// formatting can be asserted without a Google API round-trip.
func buildEvent(cfg *config.Config, b *model.Booking) *calendar.Event {
	return &calendar.Event{
		// The summary is the only field visible in month and agenda views, and in
		// previews, without opening the event — and these calendars get shared with
		// cleaning and maintenance staff. Guest name and phone stay in the
		// description so a share for scheduling does not hand out contact details.
		Summary: fmt.Sprintf("Booking #%d — %s", b.ID, b.BookingType),
		Description: fmt.Sprintf("Khach: %s\nSDT: %s\nGia: %d VND",
			b.CustomerName, b.CustomerPhone, b.ComputedPrice),
		Start: &calendar.EventDateTime{
			DateTime: b.StartTime.Format(time.RFC3339),
			TimeZone: cfg.GoogleCalendarTimeZone,
		},
		End: &calendar.EventDateTime{
			DateTime: b.EndTime.Format(time.RFC3339),
			TimeZone: cfg.GoogleCalendarTimeZone,
		},
	}
}
