package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) PutSubscription(ctx context.Context, userID, placeID string, enabled bool, now time.Time) (PlaceSubscription, error) {
	var subscription PlaceSubscription
	err := s.pool.QueryRow(ctx, `INSERT INTO place_subscriptions (user_id, place_id, notifications_enabled, created_at)
		SELECT $1, id, $3, $4 FROM places WHERE id = $2 AND status = 'active'
		ON CONFLICT (user_id, place_id) DO UPDATE SET notifications_enabled = EXCLUDED.notifications_enabled
		RETURNING place_id::text, notifications_enabled, created_at`, userID, placeID, enabled, now.UTC()).
		Scan(&subscription.PlaceID, &subscription.NotificationsEnabled, &subscription.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlaceSubscription{}, ErrEngagementNotFound
	}
	return subscription, err
}

func (s *PostgresStore) DeleteSubscription(ctx context.Context, userID, placeID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM place_subscriptions WHERE user_id = $1 AND place_id = $2`, userID, placeID)
	return err
}

func (s *PostgresStore) ListSubscriptions(ctx context.Context, userID string) ([]PlaceSubscription, error) {
	page, err := s.ListSubscriptionPage(ctx, userID, "", 50)
	return page.Subscriptions, err
}

func (s *PostgresStore) ListSubscriptionPage(ctx context.Context, userID, cursorValue string, limit int) (PlaceSubscriptionPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT place_id::text, notifications_enabled, created_at FROM place_subscriptions WHERE user_id = $1`
	args := []any{userID, limit + 1}
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return PlaceSubscriptionPage{}, err
		}
		query += ` AND (created_at, place_id) < ($3, $4)`
		args = append(args, cursor.After, cursor.ID.String())
	}
	query += ` ORDER BY created_at DESC, place_id DESC LIMIT $2`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return PlaceSubscriptionPage{}, err
	}
	defer rows.Close()
	result := []PlaceSubscription{}
	for rows.Next() {
		var subscription PlaceSubscription
		if err = rows.Scan(&subscription.PlaceID, &subscription.NotificationsEnabled, &subscription.CreatedAt); err != nil {
			return PlaceSubscriptionPage{}, err
		}
		result = append(result, subscription)
	}
	if err = rows.Err(); err != nil {
		return PlaceSubscriptionPage{}, err
	}
	page := PlaceSubscriptionPage{Subscriptions: result}
	if len(result) > limit {
		page.Subscriptions = result[:limit]
		last := page.Subscriptions[len(page.Subscriptions)-1]
		id, parseErr := uuidv7.Parse(last.PlaceID)
		if parseErr != nil {
			return PlaceSubscriptionPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
		if err != nil {
			return PlaceSubscriptionPage{}, err
		}
	}
	return page, nil
}

func (s *PostgresStore) PatchNotificationPreferences(ctx context.Context, userID string, patch NotificationPreferencesPatch, now time.Time) (NotificationPreferences, error) {
	var preferences NotificationPreferences
	err := s.pool.QueryRow(ctx, `INSERT INTO notification_preferences
		(user_id, direct_activity_enabled, place_alerts_enabled, moderation_enabled, upload_status_enabled, updated_at)
		VALUES ($1, coalesce($2, true), coalesce($3, true), coalesce($4, true), coalesce($5, true), $6)
		ON CONFLICT (user_id) DO UPDATE SET
		direct_activity_enabled = coalesce($2, notification_preferences.direct_activity_enabled),
		place_alerts_enabled = coalesce($3, notification_preferences.place_alerts_enabled),
		moderation_enabled = coalesce($4, notification_preferences.moderation_enabled),
		upload_status_enabled = coalesce($5, notification_preferences.upload_status_enabled), updated_at = $6
		RETURNING direct_activity_enabled, place_alerts_enabled, moderation_enabled, upload_status_enabled, updated_at`,
		userID, nullableBool(patch.DirectActivityEnabled), nullableBool(patch.PlaceAlertsEnabled), nullableBool(patch.ModerationEnabled), nullableBool(patch.UploadStatusEnabled), now.UTC()).
		Scan(&preferences.DirectActivityEnabled, &preferences.PlaceAlertsEnabled, &preferences.ModerationEnabled, &preferences.UploadStatusEnabled, &preferences.UpdatedAt)
	return preferences, err
}

func (s *PostgresStore) GetNotificationPreferences(ctx context.Context, userID string, now time.Time) (NotificationPreferences, error) {
	var preferences NotificationPreferences
	err := s.pool.QueryRow(ctx, `SELECT
		coalesce(preference.direct_activity_enabled, true), coalesce(preference.place_alerts_enabled, true),
		coalesce(preference.moderation_enabled, true), coalesce(preference.upload_status_enabled, true),
		coalesce(preference.updated_at, $2)
		FROM users account LEFT JOIN notification_preferences preference ON preference.user_id = account.id
		WHERE account.id = $1`, userID, now.UTC()).Scan(
		&preferences.DirectActivityEnabled, &preferences.PlaceAlertsEnabled,
		&preferences.ModerationEnabled, &preferences.UploadStatusEnabled, &preferences.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationPreferences{}, ErrEngagementNotFound
	}
	return preferences, err
}

func (s *PostgresStore) RegisterDevice(ctx context.Context, userID string, request DeviceRegistrationRequest, now time.Time) (DeviceRegistration, error) {
	if err := validateDeviceRegistration(request); err != nil {
		return DeviceRegistration{}, err
	}
	token := strings.ToLower(strings.TrimSpace(request.Token))
	hash := sha256.Sum256([]byte(token))
	ciphertext, err := encryptBytes(s.pushTokens, []byte(token))
	if err != nil {
		return DeviceRegistration{}, err
	}
	id, err := uuidv7.New()
	if err != nil {
		return DeviceRegistration{}, err
	}
	var registration DeviceRegistration
	err = s.pool.QueryRow(ctx, `INSERT INTO devices
		(id, user_id, platform, push_token_hash, push_token_ciphertext, push_environment, app_version, locale, timezone, notifications_authorized_at, created_at, last_seen_at)
		VALUES ($1, $2, 'ios', $3, $4, $5, $6, $7, $8, $9, $9, $9)
		ON CONFLICT (push_token_hash) DO UPDATE SET user_id = EXCLUDED.user_id, push_token_ciphertext = EXCLUDED.push_token_ciphertext,
		push_environment = EXCLUDED.push_environment, app_version = EXCLUDED.app_version, locale = EXCLUDED.locale,
		timezone = EXCLUDED.timezone, notifications_authorized_at = EXCLUDED.notifications_authorized_at,
		last_seen_at = EXCLUDED.last_seen_at, disabled_at = NULL
		RETURNING id::text, push_environment, coalesce(app_version, ''), created_at`, id.String(), userID, hash[:], ciphertext,
		request.Environment, strings.TrimSpace(request.AppVersion), strings.TrimSpace(request.Locale), strings.TrimSpace(request.Timezone), now.UTC()).
		Scan(&registration.ID, &registration.Environment, &registration.AppVersion, &registration.CreatedAt)
	return registration, err
}

func (s *PostgresStore) DeleteDevice(ctx context.Context, userID, deviceID string) error {
	result, err := s.pool.Exec(ctx, `UPDATE devices SET disabled_at = now(), push_token_ciphertext = NULL WHERE id = $1 AND user_id = $2 AND disabled_at IS NULL`, deviceID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrEngagementNotFound
	}
	return nil
}

func (s *PostgresStore) PutMute(ctx context.Context, userID, targetType, targetID string, mutedUntil *time.Time, now time.Time) error {
	if targetType != "place" && targetType != "echo" {
		return ErrEngagementInvalid
	}
	placeID, echoID := any(nil), any(nil)
	if targetType == "place" {
		placeID = targetID
	} else {
		echoID = targetID
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO notification_mutes (user_id, place_id, echo_id, muted_until, created_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (user_id, place_id, echo_id) DO UPDATE SET muted_until = EXCLUDED.muted_until`,
		userID, placeID, echoID, mutedUntil, now.UTC())
	return err
}

func (s *PostgresStore) DeleteMute(ctx context.Context, userID, targetType, targetID string) error {
	if targetType == "place" {
		_, err := s.pool.Exec(ctx, `DELETE FROM notification_mutes WHERE user_id = $1 AND place_id = $2`, userID, targetID)
		return err
	}
	if targetType == "echo" {
		_, err := s.pool.Exec(ctx, `DELETE FROM notification_mutes WHERE user_id = $1 AND echo_id = $2`, userID, targetID)
		return err
	}
	return ErrEngagementInvalid
}

func (s *PostgresStore) MarkNotificationsRead(ctx context.Context, userID string, notificationIDs []string, all bool, now time.Time) error {
	if all {
		_, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = coalesce(read_at, $2) WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > $2)`, userID, now.UTC())
		return err
	}
	if len(notificationIDs) == 0 || len(notificationIDs) > 100 {
		return ErrEngagementInvalid
	}
	sort.Strings(notificationIDs)
	for _, notificationID := range notificationIDs {
		if _, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = coalesce(read_at, $3) WHERE id = $1 AND user_id = $2`, notificationID, userID, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func encryptBytes(aead interface {
	NonceSize() int
	Seal([]byte, []byte, []byte, []byte) []byte
}, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}
