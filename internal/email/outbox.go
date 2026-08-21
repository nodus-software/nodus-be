package email

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"nodus-health/pkg/utility"
)

// Execer is implemented by pgx pools and transactions.
type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Message is a fully rendered transactional email ready for durable delivery.
type Message struct {
	ID        string
	TenantID  *string
	Kind      Kind
	To        string
	Subject   string
	Text      string
	HTML      string
	ExpiresAt *time.Time
	DedupeKey string
}

// Enqueue inserts a message using the supplied transaction or connection.
func Enqueue(ctx context.Context, db Execer, message Message) error {
	if message.ID == "" {
		id, err := utility.GenerateUUID()
		if err != nil {
			return err
		}
		message.ID = id
	}
	recipient := strings.ToLower(strings.TrimSpace(message.To))
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(recipient)))
	_, err := db.Exec(ctx, `INSERT INTO email_outbox
		(id,tenant_id,kind,recipient,recipient_hash,subject,text_body,html_body,expires_at,dedupe_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')) ON CONFLICT DO NOTHING`, message.ID, message.TenantID, string(message.Kind),
		recipient, hash, message.Subject, message.Text, message.HTML, message.ExpiresAt, message.DedupeKey)
	return err
}
