-- Webhook payload format (ADR-0010). 'nebula' is the signed Nebula JSON event;
-- 'slack' is a Slack/Mattermost incoming-webhook message. For 'slack' the URL
-- itself is the credential: it is kept only as AES-256-GCM ciphertext in
-- url_encrypted, and the url column holds a masked form for display.

-- +goose Up
ALTER TABLE webhooks
    ADD COLUMN format text NOT NULL DEFAULT 'nebula' CHECK (format IN ('nebula', 'slack')),
    ADD COLUMN url_encrypted bytea,
    ADD CONSTRAINT webhooks_url_encrypted_check CHECK ((format = 'slack') = (url_encrypted IS NOT NULL));

-- +goose Down
-- The plain URL of a 'slack' webhook is not stored, so after a rollback it
-- cannot be delivered: disable it instead of sending Nebula JSON to the masked URL.
UPDATE webhooks SET is_active = false, disabled_reason = 'payload format removed by a migration rollback'
    WHERE format = 'slack';
ALTER TABLE webhooks
    DROP CONSTRAINT webhooks_url_encrypted_check,
    DROP COLUMN url_encrypted,
    DROP COLUMN format;
