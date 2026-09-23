package presenter

import "github.com/johnquangdev/laverte-core/config"

// AdminSettingsResponse is the effective configuration, read-only. Settings
// live in the deployment's environment and are validated once at boot, so an
// edit here would either be lost on restart or skip that validation. Secrets are
// reported only as set or unset.
type AdminSettingsResponse struct {
	AppTimeZone              string               `json:"app_timezone"`
	BookingPendingTTLMinutes int                  `json:"booking_pending_ttl_minutes"`
	CheckinAlertLeadMinutes  int                  `json:"checkin_alert_lead_minutes"`
	AdminAlertEmail          string               `json:"admin_alert_email"`
	SePay                    SePaySettings        `json:"sepay"`
	Integrations             IntegrationsSettings `json:"integrations"`
}

type SePaySettings struct {
	// BankAccount is not a secret: it is printed on every guest's transfer QR.
	BankAccount      string `json:"bank_account"`
	BankCode         string `json:"bank_code"`
	TransferPrefix   string `json:"transfer_prefix"`
	WebhookSecretSet bool   `json:"webhook_secret_set"`
	WebhookPath      string `json:"webhook_path"`
}

type IntegrationsSettings struct {
	GoogleLogin    bool `json:"google_login"`
	GoogleCalendar bool `json:"google_calendar"`
	ZaloZNS        bool `json:"zalo_zns"`
	AdminEmail     bool `json:"admin_email"`
}

// sepayWebhookPath mirrors the route mounted in delivery/http; it is what gets
// pasted into the provider's dashboard.
const sepayWebhookPath = "/api/v1/webhooks/sepay"

func ToAdminSettingsResponse(cfg *config.Config) AdminSettingsResponse {
	return AdminSettingsResponse{
		AppTimeZone:              cfg.AppTimeZone,
		BookingPendingTTLMinutes: cfg.BookingPendingTTLMinutes,
		CheckinAlertLeadMinutes:  cfg.BookingCheckinAlertLeadMinutes,
		AdminAlertEmail:          cfg.AdminAlertEmail,
		SePay: SePaySettings{
			BankAccount:      cfg.SePayBankAccount,
			BankCode:         cfg.SePayBankCode,
			TransferPrefix:   cfg.SePayTransferPrefix,
			WebhookSecretSet: cfg.SePayWebhookSecret != "",
			WebhookPath:      sepayWebhookPath,
		},
		Integrations: IntegrationsSettings{
			GoogleLogin:    cfg.GoogleLoginConfigured(),
			GoogleCalendar: cfg.GoogleCalendarConfigured(),
			ZaloZNS:        cfg.ZNSConfigured(),
			AdminEmail:     cfg.SMTPConfigured(),
		},
	}
}
