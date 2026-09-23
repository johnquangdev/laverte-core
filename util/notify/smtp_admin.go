package notify

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/model"
)

// SMTPAdminNotifier is the admin channel. It carries two messages: a confirmed
// booking is about to start with no door lock code, and a bank transfer arrived
// that settled no booking.
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
	subject := fmt.Sprintf("[laverte-home] Booking #%d chua co ma khoa cua", b.ID)
	body := fmt.Sprintf(
		"Booking #%d tai home %d bat dau luc %s va chua co ma khoa cua.\r\n"+
			"Khach: %s - %s\r\n"+
			"Vui long nhap ma khoa cua qua PATCH /api/v1/admin/bookings/%d/lock-code.\r\n",
		b.ID, b.HomeID, b.StartTime.Format(vnTimeLayout), b.CustomerName, b.CustomerPhone, b.ID)
	if err := s.send(subject, body); err != nil {
		return fmt.Errorf("notify/smtp: lock code alert for booking %d: %w", b.ID, err)
	}
	return nil
}

func (s *SMTPAdminNotifier) AdminUnmatchedTransfer(_ context.Context, t *model.UnmatchedTransfer) error {
	subject := fmt.Sprintf("[laverte-home] Tien ve chua khop booking: %d VND", t.Amount)
	body := fmt.Sprintf(
		"Mot giao dich chuyen khoan da vao tai khoan nhung khong xac nhan duoc booking nao.\r\n"+
			"So tien: %d VND\r\n"+
			"Noi dung chuyen khoan: %s\r\n"+
			"Ly do: %s\r\n"+
			"Ma giao dich SePay: %s\r\n"+
			"Vui long doi soat trong man Giao dich > Can doi soat.\r\n",
		t.Amount, t.Content, t.Reason, t.SePayTransactionRef)
	if err := s.send(subject, body); err != nil {
		return fmt.Errorf("notify/smtp: unmatched transfer alert %s: %w", t.SePayTransactionRef, err)
	}
	return nil
}

func (s *SMTPAdminNotifier) send(subject, body string) error {
	if s.host == "" || s.adminEmail == "" {
		return errors.New("SMTP host or admin alert email not configured")
	}

	msg := "From: " + s.from + "\r\n" +
		"To: " + s.adminEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" + body

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	return smtp.SendMail(addr, auth, s.from, []string{s.adminEmail}, []byte(msg))
}
