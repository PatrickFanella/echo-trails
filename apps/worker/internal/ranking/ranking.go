package ranking

import (
	"context"
	"errors"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/identifier"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const AlgorithmVersion = "notability-v2"

type Engine struct{ pool *pgxpool.Pool }

func NewEngine(pool *pgxpool.Pool) (*Engine, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &Engine{pool: pool}, nil
}

func (e *Engine) RecomputeAll(ctx context.Context) error {
	rows, err := e.pool.Query(ctx, `SELECT id::text FROM echoes WHERE status = 'published' AND deleted_at IS NULL ORDER BY published_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err = e.RecomputeEcho(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) RecomputeEcho(ctx context.Context, echoID string) error {
	result, err := e.pool.Exec(ctx, `WITH components AS (
		SELECT e.id,
			exp(-ln(2) * extract(epoch from (now() - e.published_at)) / 172800)::numeric AS recency,
			least(1, coalesce((SELECT sum(per_user.capped_weight * CASE
					WHEN actor.status = 'banned' OR actor.status = 'deleted' THEN 0
					WHEN actor.status = 'restricted' OR actor.status = 'suspended' THEN least(0.25, coalesce(trust.weight, 0.25))
					WHEN actor.created_at > now() - interval '30 days' THEN least(0.5, coalesce(trust.weight, 0.5))
					ELSE least(1.0, coalesce(trust.weight, 1.0)) END) / 20
				FROM (SELECT event.actor_user_id, least(10, sum(event.event_weight)) AS capped_weight
					FROM echo_engagement_events event WHERE event.echo_id = e.id
					AND event.occurred_at > now() - interval '24 hours' GROUP BY event.actor_user_id) per_user
				JOIN users actor ON actor.id = per_user.actor_user_id
				LEFT JOIN user_trust_scores trust ON trust.user_id = per_user.actor_user_id), 0))::numeric AS engagement,
			least(1, 0.25 + coalesce((SELECT count(DISTINCT local_event.actor_user_id)::numeric / 10
				FROM echo_engagement_events local_event
				WHERE local_event.echo_id = e.id AND local_event.occurred_at > now() - interval '24 hours'
				AND (EXISTS (SELECT 1 FROM echoes local_echo WHERE local_echo.author_user_id = local_event.actor_user_id
					AND local_echo.place_id = e.place_id AND local_echo.status = 'published' AND local_echo.deleted_at IS NULL)
				OR EXISTS (SELECT 1 FROM replies local_reply JOIN echoes thread ON thread.id = local_reply.echo_id
					WHERE local_reply.author_user_id = local_event.actor_user_id AND local_reply.status = 'published'
					AND thread.place_id = e.place_id AND thread.status = 'published' AND thread.deleted_at IS NULL))), 0))::numeric AS locality,
			least(1, (CASE WHEN length(coalesce(e.body_text, e.caption_text, '')) >= 40 THEN 0.5 ELSE 0.25 END +
				CASE WHEN e.content_mode = 'text' OR EXISTS (SELECT 1 FROM media_assets asset WHERE asset.echo_id = e.id AND asset.status = 'approved') THEN 0.5 ELSE 0 END))::numeric AS quality,
			least(1, (SELECT count(*)::numeric / 20 FROM place_subscriptions subscription WHERE subscription.place_id = e.place_id)) AS subscription,
			CASE WHEN coalesce(e.sensitive_label, 'none') <> 'none' THEN 0.35 ELSE 0 END::numeric AS risk
		FROM echoes e WHERE e.id = $1 AND e.status = 'published' AND e.deleted_at IS NULL
	), scored AS (
		SELECT *, greatest(0, least(1, 0.35*recency + 0.30*engagement + 0.15*locality + 0.15*quality + 0.05*subscription - risk)) AS notability
		FROM components
	)
	INSERT INTO echo_rank_snapshots (echo_id, algorithm_version, recency_score, engagement_score, locality_score,
		quality_score, subscription_score, risk_penalty, notability_score, ranking_reason, evergreen_eligible, computed_at)
	SELECT id, $2, recency, engagement, locality, quality, subscription, risk, notability,
		CASE WHEN subscription >= 0.5 THEN 'subscribed_place' WHEN engagement >= 0.5 THEN 'popular_nearby'
			WHEN recency >= 0.7 THEN 'recent' ELSE 'notable' END,
		(quality >= 0.75 AND risk = 0 AND recency < 0.12 AND notability >= 0.45), now() FROM scored
	ON CONFLICT (echo_id) DO UPDATE SET algorithm_version = EXCLUDED.algorithm_version, recency_score = EXCLUDED.recency_score,
		engagement_score = EXCLUDED.engagement_score, locality_score = EXCLUDED.locality_score, quality_score = EXCLUDED.quality_score,
		subscription_score = EXCLUDED.subscription_score, risk_penalty = EXCLUDED.risk_penalty,
		notability_score = EXCLUDED.notability_score, ranking_reason = EXCLUDED.ranking_reason,
		evergreen_eligible = EXCLUDED.evergreen_eligible, computed_at = EXCLUDED.computed_at`, echoID, AlgorithmVersion)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	return e.GeneratePlaceAlerts(ctx, echoID)
}

func (e *Engine) GeneratePlaceAlerts(ctx context.Context, echoID string) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var placeID string
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT e.place_id::text,
		(rank.notability_score >= 0.70 AND e.published_at <= now() - interval '15 minutes'
		AND coalesce(e.sensitive_label, 'none') = 'none'
		AND NOT EXISTS (SELECT 1 FROM echo_links link WHERE link.target_echo_id = e.id))
		FROM echoes e JOIN echo_rank_snapshots rank ON rank.echo_id = e.id WHERE e.id = $1 AND e.status = 'published'`, echoID).Scan(&placeID, &eligible)
	if errors.Is(err, pgx.ErrNoRows) || !eligible {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT subscription.user_id::text,
		NOT EXISTS (SELECT 1 FROM place_alert_deliveries delivery WHERE delivery.user_id = subscription.user_id
			AND delivery.place_id = subscription.place_id AND delivery.delivered_at > now() - interval '6 hours') AS push_eligible
		FROM place_subscriptions subscription
		LEFT JOIN notification_preferences preference ON preference.user_id = subscription.user_id
		WHERE subscription.place_id = $1 AND subscription.notifications_enabled AND coalesce(preference.place_alerts_enabled, true)
		AND NOT EXISTS (SELECT 1 FROM notifications existing WHERE existing.user_id = subscription.user_id AND existing.echo_id = $2 AND existing.type = 'place_alert')
		AND NOT EXISTS (SELECT 1 FROM notification_mutes mute WHERE mute.user_id = subscription.user_id AND mute.place_id = subscription.place_id
			AND (mute.muted_until IS NULL OR mute.muted_until > now()))`, placeID, echoID)
	if err != nil {
		return err
	}
	type subscriber struct {
		userID string
		push   bool
	}
	subscribers := []subscriber{}
	for rows.Next() {
		var value subscriber
		if err = rows.Scan(&value.userID, &value.push); err != nil {
			rows.Close()
			return err
		}
		subscribers = append(subscribers, value)
	}
	rows.Close()
	for _, subscriber := range subscribers {
		notificationID, idErr := randomUUID()
		if idErr != nil {
			return idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO notifications (id, user_id, type, echo_id, place_id, payload, expires_at, collapse_key)
			VALUES ($1, $2, 'place_alert', $3, $4, '{"preview":"A subscribed Place has a notable new Echo."}', now() + interval '90 days', $5)`,
			notificationID, subscriber.userID, echoID, placeID, "place:"+placeID)
		if err != nil {
			return err
		}
		if !subscriber.push {
			continue
		}
		_, err = tx.Exec(ctx, `INSERT INTO place_alert_deliveries (user_id, place_id, echo_id, notification_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, subscriber.userID, placeID, echoID, notificationID)
		if err != nil {
			return err
		}
		deliveryResult, err := tx.Exec(ctx, `INSERT INTO notification_deliveries (notification_id, device_id)
			SELECT $1, id FROM devices WHERE user_id = $2 AND disabled_at IS NULL AND push_token_ciphertext IS NOT NULL ON CONFLICT DO NOTHING`, notificationID, subscriber.userID)
		if err != nil {
			return err
		}
		if deliveryResult.RowsAffected() > 0 {
			jobID, idErr := randomUUID()
			if idErr != nil {
				return idErr
			}
			if _, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts) VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5)`, jobID, notificationID); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func randomUUID() (string, error) {
	return identifier.NewUUIDv7()
}
