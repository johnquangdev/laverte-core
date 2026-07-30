package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port        string `envconfig:"PORT" default:"14000"`
	AppName     string `envconfig:"APP_NAME" default:"laverte-home-backend"`
	Environment string `envconfig:"ENVIRONMENT" default:"development"`

	// AppTimeZone is the business's own clock, and every ?date=YYYY-MM-DD an admin
	// sends means a day in it. Nothing pins the container's zone, so parsing those
	// dates in the host's location silently shifts the window by the UTC offset —
	// seven hours at both ends of a revenue range, with nothing to signal it.
	// Distinct from GoogleCalendarTimeZone, which is a field in a Calendar API
	// payload rather than a parsing default.
	AppTimeZone string `envconfig:"APP_TIMEZONE" default:"Asia/Ho_Chi_Minh"`

	PostgresHost     string `envconfig:"POSTGRES_HOST" default:"localhost"`
	PostgresPort     string `envconfig:"POSTGRES_PORT" default:"5432"`
	PostgresUser     string `envconfig:"POSTGRES_USER" default:"laverte"`
	PostgresPassword string `envconfig:"POSTGRES_PASSWORD" default:"laverte"`
	PostgresDB       string `envconfig:"POSTGRES_DB" default:"laverte"`

	JWTAccessSecret     string `envconfig:"JWT_ACCESS_SECRET" required:"true"`
	JWTRefreshSecret    string `envconfig:"JWT_REFRESH_SECRET" required:"true"`
	JWTAccessTTLMinutes int    `envconfig:"JWT_ACCESS_TTL_MINUTES" default:"15"`
	JWTRefreshTTLDays   int    `envconfig:"JWT_REFRESH_TTL_DAYS" default:"30"`

	GoogleClientID     string `envconfig:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `envconfig:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURI  string `envconfig:"GOOGLE_REDIRECT_URI"`

	FrontendURL string `envconfig:"FRONTEND_URL" default:"http://localhost:3000"`
	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"http://localhost:3000"`

	RedisURL string `envconfig:"REDIS_URL" default:"redis://localhost:6379"`

	AdminUserIDs []uint `envconfig:"ADMIN_USER_IDS"`

	SePayAPIKey         string `envconfig:"SEPAY_API_KEY"`
	SePayBankAccount    string `envconfig:"SEPAY_BANK_ACCOUNT"`
	SePayBankCode       string `envconfig:"SEPAY_BANK_CODE"`
	SePayWebhookSecret  string `envconfig:"SEPAY_WEBHOOK_SECRET"`
	SePayTransferPrefix string `envconfig:"SEPAY_TRANSFER_PREFIX" default:"LAVERTE"`

	BookingPendingTTLMinutes       int `envconfig:"BOOKING_PENDING_TTL_MINUTES" default:"15"`
	BookingCheckinAlertLeadMinutes int `envconfig:"BOOKING_CHECKIN_ALERT_LEAD_MINUTES" default:"30"`

	GoogleCalendarCredentialsJSON string `envconfig:"GOOGLE_CALENDAR_CREDENTIALS_JSON"`
	GoogleCalendarTimeZone        string `envconfig:"GOOGLE_CALENDAR_TIMEZONE" default:"Asia/Ho_Chi_Minh"`

	ZNSAccessToken                string `envconfig:"ZNS_ACCESS_TOKEN"`
	ZNSBookingConfirmedTemplateID string `envconfig:"ZNS_BOOKING_CONFIRMED_TEMPLATE_ID"`
	ZNSLockCodeTemplateID         string `envconfig:"ZNS_LOCK_CODE_TEMPLATE_ID"`
	ZNSEndpoint                   string `envconfig:"ZNS_ENDPOINT" default:"https://business.openapi.zalo.me/message/template"`

	SMTPHost        string `envconfig:"SMTP_HOST"`
	SMTPPort        string `envconfig:"SMTP_PORT" default:"587"`
	SMTPUsername    string `envconfig:"SMTP_USERNAME"`
	SMTPPassword    string `envconfig:"SMTP_PASSWORD"`
	SMTPFrom        string `envconfig:"SMTP_FROM" default:"no-reply@laverte-home.vn"`
	AdminAlertEmail string `envconfig:"ADMIN_ALERT_EMAIL"`

	RateLimitBookingPerMinIP    int `envconfig:"RATE_LIMIT_BOOKING_PER_MIN_IP" default:"10"`
	RateLimitBookingPerMinPhone int `envconfig:"RATE_LIMIT_BOOKING_PER_MIN_PHONE" default:"5"`
	RateLimitWebhookPerMin      int `envconfig:"RATE_LIMIT_WEBHOOK_PER_MIN" default:"120"`
	RateLimitPublicPerMin       int `envconfig:"RATE_LIMIT_PUBLIC_PER_MIN" default:"20"`
	RateLimitAuthedPerMin       int `envconfig:"RATE_LIMIT_AUTHED_PER_MIN" default:"120"`
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB)
}

var cfg *Config

// GetConfig loads env once (godotenv then envconfig) and caches the result.
// Validate reports the settings the money path cannot degrade without. The notifier
// and Calendar adapters deliberately warn and fall back to a no-op when half
// configured, but a missing SePay setting has no safe degraded mode: without the bank
// details every guest booking 500s at CreateQR *after* its row is already committed and
// holding a slot, and without the webhook secret every callback is rejected as
// unsigned, so no transfer ever settles and every booking expires unpaid. Both look
// like a healthy process from the outside.
func (c *Config) Validate() error {
	var missing []string
	for _, f := range []struct {
		name  string
		value string
	}{
		{"SEPAY_BANK_ACCOUNT", c.SePayBankAccount},
		{"SEPAY_BANK_CODE", c.SePayBankCode},
		{"SEPAY_WEBHOOK_SECRET", c.SePayWebhookSecret},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required settings: %s", strings.Join(missing, ", "))
	}
	return nil
}

func GetConfig() *Config {
	if cfg != nil {
		return cfg
	}
	_ = godotenv.Load()
	cfg = &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}
