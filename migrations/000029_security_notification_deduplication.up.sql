ALTER TABLE email_outbox ADD COLUMN dedupe_key TEXT;
CREATE UNIQUE INDEX email_outbox_active_dedupe_key
    ON email_outbox (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
