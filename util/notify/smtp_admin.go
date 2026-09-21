package notify

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/model"
)

// SMTPAdminNotifier is the admin channel. It carries one message: a confirmed
// booking is about to start and nobody has entered the door lock code yet.
type SMTPAdminNotifier struct {
	host       string
	port       string
	username   string
	password   string
	from       string
	adminEmail string
}

func NewSMTPAdmin(cfg *config.Config) *SMTPAdminNotifier {
	return &SMTPAdminNotifier{
		host:       cfg.SMTPHost,
		port:       cfg.SMTPPort,
		username:   cfg.SMTPUsername,
		password:   cfg.SMTPPassword,
		from:       cfg.SMTPFrom,
		adminEmail: cfg.AdminAlertEmail,
	}
}

// BookingConfirmed and LockCode are intentionally inert: guests are reached
// over ZNS, and this type has no customer address to mail.
func (s *SMTPAdminNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }

func (s *SMTPAdminNotifier) LockCode(context.Context, *model.Booking, string) error { return nil }

func (s *SMTPAdminNotifier) AdminLockCodeMissing(_ context.Context, b *model.Booking) error {
	if s.host == "" || s.adminEmail == "" {
		return errors.New("notify/smtp: SMTP host or admin alert email not configured")
	}

	subject := fmt.Sprintf("[laverte-home] Booking #%d chua co ma khoa cua", b.ID)
	body := fmt.Sprintf(
		"Booking #%d tai home %d bat dau luc %s va chua co ma khoa cua.\r\n"+
			"Khach: %s - %s\r\n"+
			"Vui long nhap ma khoa cua qua PATCH /api/v1/admin/bookings/%d/lock-code.\r\n",
		b.ID, b.HomeID, b.StartTime.Format(vnTimeLayout), b.CustomerName, b.CustomerPhone, b.ID)

	msg := "From: " + s.from + "\r\n" +
		"To: " + s.adminEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" + body

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := smtp.SendMail(addr, auth, s.from, []string{s.adminEmail}, []byte(msg)); err != nil {
		return fmt.Errorf("notify/smtp: send admin alert for booking %d: %w", b.ID, err)
	}
	return nil
}
