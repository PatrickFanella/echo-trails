-- Device delivery state, APNs retries, and Activity read tracking.

CREATE TABLE notification_deliveries (
    notification_id uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'sent', 'retry', 'invalid_token', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    apns_id text,
    last_error text,
    sent_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (notification_id, device_id)
);

CREATE INDEX notification_deliveries_due_idx ON notification_deliveries(status, next_attempt_at)
    WHERE status IN ('queued', 'retry');

ALTER TABLE notifications ADD COLUMN collapse_key text;
