package gcalendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// newTestGoogleCalendar points a real *calendar.Service at a local httptest
// server instead of Google, so DeleteEvent's actual HTTP round trip — not a
// reimplementation of its logic — is what gets exercised below.
func newTestGoogleCalendar(t *testing.T, status int, body string) *googleCalendar {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	svc, err := calendar.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("calendar.NewService() error = %v", err)
	}
	return &googleCalendar{svc: svc}
}

// TestDeleteEvent404And410AreTreatedAsSuccess proves the design point that a
// second cancellation, or an event an admin already removed by hand, must
// not surface as an error: Google reports both with 404 or 410, and either
// one means the desired end state (event gone) is already reached.
func TestDeleteEvent404And410AreTreatedAsSuccess(t *testing.T) {
	notFoundBody := `{"error":{"code":404,"message":"Not Found","errors":[{"reason":"notFound"}]}}`
	goneBody := `{"error":{"code":410,"message":"Resource has been deleted","errors":[{"reason":"deleted"}]}}`
	serverErrBody := `{"error":{"code":500,"message":"Backend Error"}}`

	t.Run("404 is success", func(t *testing.T) {
		g := newTestGoogleCalendar(t, http.StatusNotFound, notFoundBody)
		if err := g.DeleteEvent(context.Background(), "cal-1", "evt-1"); err != nil {
			t.Errorf("DeleteEvent() with 404 = %v, want nil", err)
		}
	})

	t.Run("410 is success", func(t *testing.T) {
		g := newTestGoogleCalendar(t, http.StatusGone, goneBody)
		if err := g.DeleteEvent(context.Background(), "cal-1", "evt-1"); err != nil {
			t.Errorf("DeleteEvent() with 410 = %v, want nil", err)
		}
	})

	t.Run("500 is a real failure", func(t *testing.T) {
		g := newTestGoogleCalendar(t, http.StatusInternalServerError, serverErrBody)
		err := g.DeleteEvent(context.Background(), "cal-1", "evt-1")
		if err == nil {
			t.Fatal("DeleteEvent() with 500 = nil, want error")
		}
		if !strings.Contains(err.Error(), "delete event evt-1") {
			t.Errorf("DeleteEvent() error = %q, want it wrapped with our context", err.Error())
		}
	})
}
