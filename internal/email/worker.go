package email

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/pkg/logger"
)

type Worker struct {
	pool     *pgxpool.Pool
	provider Provider
	log      *logger.Logger
}

func NewWorker(pool *pgxpool.Pool, provider Provider, log *logger.Logger) *Worker {
	return &Worker{pool: pool, provider: provider, log: log}
}

type queuedMessage struct {
	Delivery
	AttemptCount int
}

func (w *Worker) Run(ctx context.Context) {
	// A message rejected permanently by one provider may be valid at another.
	if _, err := w.pool.Exec(ctx, `UPDATE email_outbox SET status='pending', attempt_count=0, next_attempt_at=now(), last_error=NULL
		WHERE status='failed' AND last_provider IS DISTINCT FROM $1 AND (expires_at IS NULL OR expires_at>now())`, w.provider.Name()); err != nil {
		w.log.Error("failed to requeue email for selected provider", "provider", w.provider.Name(), "error", err.Error())
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	cleanup := time.NewTicker(24 * time.Hour)
	defer cleanup.Stop()
	for {
		if err := w.processOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.log.Error("email worker iteration failed", "provider", w.provider.Name(), "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-cleanup.C:
			_, _ = w.pool.Exec(ctx, `DELETE FROM email_outbox WHERE status IN ('sent','failed','expired') AND updated_at<now()-interval '30 days'`)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	message, err := w.claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	providerID, sendErr := w.provider.Send(ctx, message.Delivery)
	if sendErr == nil {
		_, err = w.pool.Exec(ctx, `UPDATE email_outbox SET status='sent', sent_at=now(), provider_message_id=$2,
			lease_until=NULL, last_error=NULL, recipient=NULL, subject=NULL, text_body=NULL, html_body=NULL WHERE id=$1`, message.ID, providerID)
		if err == nil {
			w.log.Info("email delivered", "email_id", message.ID, "provider", w.provider.Name())
		}
		return err
	}

	status := "pending"
	nextAttempt := time.Now().Add(retryDelay(message.AttemptCount))
	if errors.Is(sendErr, ErrPermanent) {
		status = "failed"
	}
	_, err = w.pool.Exec(ctx, `UPDATE email_outbox SET status=$2, next_attempt_at=$3, lease_until=NULL, last_error=$4 WHERE id=$1`,
		message.ID, status, nextAttempt, truncateError(sendErr.Error()))
	if err == nil {
		w.log.Error("email delivery failed", "email_id", message.ID, "provider", w.provider.Name(), "retryable", status == "pending", "error", sendErr.Error())
	}
	return err
}

func (w *Worker) claim(ctx context.Context) (*queuedMessage, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var message queuedMessage
	err = tx.QueryRow(ctx, `SELECT id::text,recipient,subject,text_body,html_body,attempt_count
		FROM email_outbox
		WHERE status IN ('pending','processing')
		  AND next_attempt_at<=now()
		  AND (lease_until IS NULL OR lease_until<now())
		  AND (expires_at IS NULL OR expires_at>now())
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&message.ID, &message.To, &message.Subject, &message.Text, &message.HTML, &message.AttemptCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, expireErr := tx.Exec(ctx, `UPDATE email_outbox SET status='expired', lease_until=NULL
				WHERE status IN ('pending','processing') AND expires_at IS NOT NULL AND expires_at<=now()`); expireErr != nil {
				return nil, expireErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return nil, commitErr
			}
		}
		return nil, err
	}
	message.AttemptCount++
	if _, err = tx.Exec(ctx, `UPDATE email_outbox SET status='processing', attempt_count=$2, lease_until=now()+interval '1 minute', last_provider=$3 WHERE id=$1`,
		message.ID, message.AttemptCount, w.provider.Name()); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &message, nil
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 10 * time.Second
	for i := 1; i < attempt && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func truncateError(value string) string {
	const limit = 2000
	if len(value) <= limit {
		return value
	}
	return fmt.Sprintf("%s…", value[:limit-3])
}
