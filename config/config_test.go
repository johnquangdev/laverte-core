package config

import "testing"

func TestGetConfigDefaults(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	cfg = nil // reset singleton so this test controls the env it reads

	c := GetConfig()

	if c.Port != "14000" {
		t.Errorf("Port = %q, want 14000", c.Port)
	}
	if c.BookingPendingTTLMinutes != 15 {
		t.Errorf("BookingPendingTTLMinutes = %d, want 15", c.BookingPendingTTLMinutes)
	}
	if c.BookingCheckinAlertLeadMinutes != 30 {
		t.Errorf("BookingCheckinAlertLeadMinutes = %d, want 30", c.BookingCheckinAlertLeadMinutes)
	}
}

func TestDatabaseURL(t *testing.T) {
	c := &Config{PostgresHost: "h", PostgresPort: "5432", PostgresUser: "u", PostgresPassword: "p", PostgresDB: "d"}
	want := "host=h port=5432 user=u password=p dbname=d sslmode=disable"
	if got := c.DatabaseURL(); got != want {
		t.Errorf("DatabaseURL() = %q, want %q", got, want)
	}
}
