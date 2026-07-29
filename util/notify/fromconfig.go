package notify

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

// znsTimeout bounds a ZNS call. Notification is best-effort and called inline from
// the webhook, so a hung provider must not hold the request open.
const znsTimeout = 10 * time.Second

// FromConfig returns the real two-channel notifier, or the no-op when either
// channel is unconfigured.
//
// Both or neither, deliberately. A half-configured environment — guests messaged
// but the admin alert going nowhere, or the reverse — hides a misconfiguration
// behind apparently-working behaviour, and the lock-code flow depends on the admin
// alert arriving. Returning the no-op makes the gap visible in the boot log instead.
func FromConfig(cfg *config.Config, log *zap.Logger) INotifier {
	znsReady := cfg.ZNSAccessToken != ""
	smtpReady := cfg.SMTPHost != ""
	if znsReady && smtpReady {
		return NewComposite(NewZNS(cfg, &http.Client{Timeout: znsTimeout}), NewSMTPAdmin(cfg))
	}
	log.Warn("notifications disabled",
		zap.Bool("zns_configured", znsReady),
		zap.Bool("smtp_configured", smtpReady))
	return NewNoop()
}
