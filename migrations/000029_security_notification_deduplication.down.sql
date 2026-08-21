DROP INDEX email_outbox_active_dedupe_key;
ALTER TABLE email_outbox DROP COLUMN dedupe_key;
