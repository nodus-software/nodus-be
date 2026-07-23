package auth

import "context"

// Mailer delivers out-of-band messages (password reset links, etc). A
// dedicated interface keeps the service testable and delivery-mechanism
// agnostic — the SMTP implementation lives in smtp.go.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
