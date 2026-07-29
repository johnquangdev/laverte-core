package notify

import (
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

func TestFromConfig(t *testing.T) {
	cases := []struct {
		name          string
		znsToken      string
		smtpHost      string
		wantComposite bool
	}{
		{name: "both configured", znsToken: "tok", smtpHost: "smtp.example.com", wantComposite: true},
		{name: "only ZNS configured", znsToken: "tok", smtpHost: "", wantComposite: false},
		{name: "only SMTP configured", znsToken: "", smtpHost: "smtp.example.com", wantComposite: false},
		{name: "neither configured", znsToken: "", smtpHost: "", wantComposite: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &config.Config{ZNSAccessToken: c.znsToken, SMTPHost: c.smtpHost}
			n := FromConfig(cfg, zap.NewNop())

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
