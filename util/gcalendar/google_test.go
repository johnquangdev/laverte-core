package gcalendar

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

func TestNewGoogleRejectsEmptyCredentials(t *testing.T) {
	if _, err := NewGoogle(&config.Config{}); err == nil {
		t.Fatal("NewGoogle() with empty GoogleCalendarCredentialsJSON = nil error, want error")
	}
}

func TestNewGoogleRejectsMalformedCredentials(t *testing.T) {
	cfg := &config.Config{GoogleCalendarCredentialsJSON: `{"type": "service_account"`}
	if _, err := NewGoogle(cfg); err == nil {
		t.Fatal("NewGoogle() with malformed JSON = nil error, want error")
	}
}

// The noop must be a usable fallback so cmd/main.go boots with no credentials.
func TestNoopIsUsableWithoutCredentials(t *testing.T) {
	cal := NewNoop()
	ctx := context.Background()

	eventID, err := cal.CreateEvent(ctx, "cal-1", &model.Booking{ID: 7})
	if err != nil {
		t.Fatalf("noop CreateEvent() error = %v, want nil", err)
	}
	if eventID != "" {
		t.Errorf("noop CreateEvent() id = %q, want empty", eventID)
	}
	if err := cal.DeleteEvent(ctx, "cal-1", "evt-1"); err != nil {
		t.Errorf("noop DeleteEvent() error = %v, want nil", err)
	}
}

func TestBuildEvent(t *testing.T) {
	cfg := &config.Config{GoogleCalendarTimeZone: "Asia/Ho_Chi_Minh"}
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	b := &model.Booking{
		ID:            42,
		HomeID:        7,
		CustomerName:  "Nguyen Van A",
		CustomerPhone: "0901234567",
		StartTime:     start,
		EndTime:       start.Add(3 * time.Hour),
		BookingType:   model.BookingTypeHourly,
		ComputedPrice: 450000,
	}

	ev := buildEvent(cfg, b)

	wantSummary := "Booking #42 — Nguyen Van A (0901234567)"
	if ev.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", ev.Summary, wantSummary)
	}
	if ev.Start.TimeZone != "Asia/Ho_Chi_Minh" {
		t.Errorf("Start.TimeZone = %q, want Asia/Ho_Chi_Minh", ev.Start.TimeZone)
	}
	if ev.End.TimeZone != "Asia/Ho_Chi_Minh" {
		t.Errorf("End.TimeZone = %q, want Asia/Ho_Chi_Minh", ev.End.TimeZone)
	}
	if ev.Start.DateTime != "2026-08-01T14:00:00Z" {
		t.Errorf("Start.DateTime = %q, want 2026-08-01T14:00:00Z", ev.Start.DateTime)
	}
	if ev.End.DateTime != "2026-08-01T17:00:00Z" {
		t.Errorf("End.DateTime = %q, want 2026-08-01T17:00:00Z", ev.End.DateTime)
	}
	if !strings.Contains(ev.Description, "0901234567") {
		t.Errorf("Description = %q, want it to contain the customer phone", ev.Description)
	}
	if !strings.Contains(ev.Description, "450000") {
		t.Errorf("Description = %q, want it to contain the computed price", ev.Description)
	}
	if !strings.Contains(ev.Description, model.BookingTypeHourly) {
		t.Errorf("Description = %q, want it to contain the booking type", ev.Description)
	}
}
