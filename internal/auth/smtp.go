package auth

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPMailer sends plaintext email via a standard SMTP submission server
// using the credentials in config. If Host is empty (no SMTP configured,
// e.g. in local dev), Send is a no-op that returns nil so the rest of the
// password-reset flow still completes.
type SMTPMailer struct {
	Host     string
	Port     string
	Sender   string
	Password string
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	if m.Host == "" {
		return nil
	}

	msg := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, m.Sender, subject, body)

	auth := smtp.PlainAuth("", m.Sender, m.Password, m.Host)
	addr := fmt.Sprintf("%s:%s", m.Host, m.Port)

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, m.Sender, []string{to}, []byte(msg))
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("send mail: %w", err)
		}
		return nil
	}
}
