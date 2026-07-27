package auth

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
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
	msg := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", to, m.Sender, subject, body)
	return m.send(ctx, to, []byte(msg))
}

// SendHTML sends a multipart/alternative email. The plain-text part remains
// available to accessibility tools and clients that do not render HTML.
func (m *SMTPMailer) SendHTML(ctx context.Context, to, subject, textBody, htmlBody string) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	textHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	textPart, err := w.CreatePart(textHeader)
	if err != nil {
		return fmt.Errorf("create text email part: %w", err)
	}
	textEncoder := quotedprintable.NewWriter(textPart)
	if _, err := textEncoder.Write([]byte(textBody)); err != nil {
		return fmt.Errorf("encode text email part: %w", err)
	}
	if err := textEncoder.Close(); err != nil {
		return fmt.Errorf("close text email part: %w", err)
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	htmlPart, err := w.CreatePart(htmlHeader)
	if err != nil {
		return fmt.Errorf("create HTML email part: %w", err)
	}
	htmlEncoder := quotedprintable.NewWriter(htmlPart)
	if _, err := htmlEncoder.Write([]byte(htmlBody)); err != nil {
		return fmt.Errorf("encode HTML email part: %w", err)
	}
	if err := htmlEncoder.Close(); err != nil {
		return fmt.Errorf("close HTML email part: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close email body: %w", err)
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "To: %s\r\nFrom: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", to, m.Sender, subject, w.Boundary())
	msg.Write(body.Bytes())
	return m.send(ctx, to, msg.Bytes())
}

func (m *SMTPMailer) send(ctx context.Context, to string, msg []byte) error {
	if m.Host == "" {
		return nil
	}

	auth := smtp.PlainAuth("", m.Sender, m.Password, m.Host)
	addr := fmt.Sprintf("%s:%s", m.Host, m.Port)

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, m.Sender, []string{to}, msg)
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
