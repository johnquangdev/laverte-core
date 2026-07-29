package notify

import (
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

func TestFromConfig(t *testing.T) {
	// base is a fully-configured environment; each case knocks out exactly one
	// field so a failure pinpoints which readiness clause let it through.
	base := config.Config{
		ZNSAccessToken:                "tok",
		ZNSBookingConfirmedTemplateID: "tpl-confirmed",
		ZNSLockCodeTemplateID:         "tpl-lock",
		SMTPHost:                      "smtp.example.com",
		AdminAlertEmail:               "admin@example.com",
	}

	cases := []struct {
		name          string
		mutate        func(cfg *config.Config)
		wantComposite bool
	}{
		{name: "both configured", mutate: func(*config.Config) {}, wantComposite: true},
		{name: "only ZNS configured (no SMTP host)", mutate: func(cfg *config.Config) { cfg.SMTPHost = "" }, wantComposite: false},
		{name: "only SMTP configured (no ZNS token)", mutate: func(cfg *config.Config) { cfg.ZNSAccessToken = "" }, wantComposite: false},
		{name: "neither configured", mutate: func(cfg *config.Config) { cfg.ZNSAccessToken = ""; cfg.SMTPHost = "" }, wantComposite: false},
		{name: "SMTP host set but no admin alert email", mutate: func(cfg *config.Config) { cfg.AdminAlertEmail = "" }, wantComposite: false},
		{name: "ZNS token set but confirmed template id empty", mutate: func(cfg *config.Config) { cfg.ZNSBookingConfirmedTemplateID = "" }, wantComposite: false},
		{name: "ZNS token set but lock code template id empty", mutate: func(cfg *config.Config) { cfg.ZNSLockCodeTemplateID = "" }, wantComposite: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			c.mutate(&cfg)
			n := FromConfig(&cfg, zap.NewNop())

			_, isNoop := n.(noopNotifier)
			if c.wantComposite && isNoop {
				t.Errorf("FromConfig() = noop, want composite")
			}
			if !c.wantComposite && !isNoop {
				t.Errorf("FromConfig() = %T, want noop", n)
			}
		})
	}
}
