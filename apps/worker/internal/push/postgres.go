package push

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
	aead cipher.AEAD
}

func NewPostgresRepository(pool *pgxpool.Pool, encryptionKey []byte) (*PostgresRepository, error) {
	if pool == nil || len(encryptionKey) < 32 {
		return nil, errors.New("Postgres and a 32-byte push encryption key are required")
	}
	digest := sha256.Sum256(encryptionKey)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &PostgresRepository{pool: pool, aead: aead}, nil
}

func (r *PostgresRepository) Pending(ctx context.Context, notificationID string) ([]Notification, error) {
	rows, err := r.pool.Query(ctx, `SELECT n.id::text, d.id::text, d.push_token_ciphertext, d.push_environment,
		n.type, coalesce(n.payload->>'preview', ''), coalesce(n.echo_id::text, ''),
		coalesce(n.payload->>'moderation_action_id', ''), coalesce(n.collapse_key, '')
		FROM notification_deliveries delivery
		JOIN notifications n ON n.id = delivery.notification_id
		JOIN devices d ON d.id = delivery.device_id
		WHERE delivery.notification_id = $1 AND delivery.status IN ('queued', 'retry')
		AND delivery.next_attempt_at <= now() AND d.disabled_at IS NULL`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Notification{}
	for rows.Next() {
		var notification Notification
		var ciphertext []byte
		if err = rows.Scan(&notification.NotificationID, &notification.DeviceID, &ciphertext, &notification.Environment,
			&notification.Kind, &notification.Preview, &notification.EchoID, &notification.ActionID, &notification.CollapseKey); err != nil {
			return nil, err
		}
		token, decryptErr := r.decrypt(ciphertext)
		if decryptErr != nil {
			return nil, decryptErr
		}
		notification.Token = string(token)
		result = append(result, notification)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) MarkSent(ctx context.Context, notification Notification, apnsID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notification_deliveries SET status = 'sent', attempts = attempts + 1,
		apns_id = $3, sent_at = now(), updated_at = now(), last_error = NULL
		WHERE notification_id = $1 AND device_id = $2`, notification.NotificationID, notification.DeviceID, apnsID)
	return err
}

func (r *PostgresRepository) MarkRetry(ctx context.Context, notification Notification, sendErr error) error {
	reason := safeError(sendErr)
	_, err := r.pool.Exec(ctx, `UPDATE notification_deliveries SET status = CASE WHEN attempts + 1 >= 5 THEN 'failed' ELSE 'retry' END,
		attempts = attempts + 1, next_attempt_at = now() + make_interval(secs => least(300, power(2, attempts)::integer)),
		last_error = $3, updated_at = now() WHERE notification_id = $1 AND device_id = $2`, notification.NotificationID, notification.DeviceID, reason)
	return err
}

func (r *PostgresRepository) DisableInvalidToken(ctx context.Context, notification Notification, sendErr error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE notification_deliveries SET status = 'invalid_token', attempts = attempts + 1,
		last_error = $3, updated_at = now() WHERE notification_id = $1 AND device_id = $2`, notification.NotificationID, notification.DeviceID, safeError(sendErr)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE devices SET disabled_at = now(), push_token_ciphertext = NULL WHERE id = $1`, notification.DeviceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := r.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("invalid push token ciphertext")
	}
	return r.aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

func safeError(err error) string {
	if err == nil {
		return "unknown APNs error"
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
