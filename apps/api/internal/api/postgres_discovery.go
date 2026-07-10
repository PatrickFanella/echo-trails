package api

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

func (s *PostgresStore) SearchDiscovery(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || len([]rune(request.Query)) > 200 {
		return SearchResponse{}, ErrDiscoveryInvalid
	}
	if request.Limit <= 0 || request.Limit > 50 {
		request.Limit = 20
	}
	args := []any{request.Query, request.ViewerID, request.Limit + 1}
	whereScope := ""
	switch request.Scope {
	case "", "city":
	case "place":
		if request.PlaceID == "" {
			return SearchResponse{}, ErrDiscoveryInvalid
		}
		whereScope = " AND e.place_id = $4"
		args = append(args, request.PlaceID)
	case "nearby":
		if request.Latitude == nil || request.Longitude == nil || request.RadiusMeters < 10 || request.RadiusMeters > 10000 {
			return SearchResponse{}, ErrDiscoveryInvalid
		}
		whereScope = " AND ST_DWithin(e.public_location, ST_SetSRID(ST_MakePoint($5, $4), 4326)::geography, $6)"
		args = append(args, *request.Latitude, *request.Longitude, request.RadiusMeters)
	default:
		return SearchResponse{}, ErrDiscoveryInvalid
	}
	cursorPredicate := ""
	if request.Cursor != "" {
		cursor, decodeErr := pagination.Decode(request.Cursor)
		if decodeErr != nil {
			return SearchResponse{}, decodeErr
		}
		cursorScore, _, scoreErr := s.echoScore(ctx, cursor.ID.String(), request.Query)
		if scoreErr != nil {
			return SearchResponse{}, scoreErr
		}
		cursorPredicate = fmt.Sprintf(" WHERE (score, published_at, id) < ($%d, $%d, $%d::uuid)", len(args)+1, len(args)+2, len(args)+3)
		args = append(args, cursorScore, cursor.After, cursor.ID.String())
	}
	searchContext, cancelSearch := context.WithCancel(ctx)
	defer cancelSearch()
	type placeSearchResult struct {
		places []Place
		err    error
	}
	placeResults := make(chan placeSearchResult, 1)
	go func() {
		places, searchErr := s.SearchPlaces(searchContext, request.Query, false, request.Latitude, request.Longitude)
		if len(places) > request.Limit {
			places = places[:request.Limit]
		}
		select {
		case placeResults <- placeSearchResult{places: places, err: searchErr}:
		case <-searchContext.Done():
		}
	}()
	waitForPlaces := func() ([]Place, error) {
		select {
		case result := <-placeResults:
			return result.places, result.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Full-text search uses bounded recent and notable candidate sets for both
	// Echo text and approved transcripts before applying
	// the exact 60/25/15 score. Recent candidates preserve fresh local results;
	// notability candidates preserve older high-signal results. Both scans can
	// use existing published-at and notability indexes, avoiding an exhaustive
	// query-time rank/sort across every matching Echo.
	query := `WITH candidate_ids AS (
		(SELECT e.id FROM echoes e
		 WHERE e.status = 'published' AND e.deleted_at IS NULL
		 AND e.search_document @@ websearch_to_tsquery('simple', unaccent($1))` + whereScope + `
		 ORDER BY e.published_at DESC, e.id DESC LIMIT 500)
		UNION
		(SELECT e.id FROM echo_rank_snapshots candidate_rank
		 JOIN echoes e ON e.id = candidate_rank.echo_id
		 WHERE e.status = 'published' AND e.deleted_at IS NULL
		 AND e.search_document @@ websearch_to_tsquery('simple', unaccent($1))` + whereScope + `
		 ORDER BY candidate_rank.notability_score DESC, candidate_rank.echo_id DESC LIMIT 500)
		UNION
		(SELECT e.id FROM media_transcripts transcript
		 JOIN media_assets asset ON asset.id = transcript.media_asset_id
		 JOIN echoes e ON e.id = asset.echo_id
		 WHERE transcript.status = 'succeeded' AND asset.status = 'approved' AND asset.deleted_at IS NULL
		 AND e.status = 'published' AND e.deleted_at IS NULL
		 AND transcript.search_document @@ websearch_to_tsquery('simple', unaccent($1))` + whereScope + `
		 ORDER BY e.published_at DESC, e.id DESC LIMIT 500)
		UNION
		(SELECT e.id FROM media_transcripts transcript
		 JOIN media_assets asset ON asset.id = transcript.media_asset_id
		 JOIN echoes e ON e.id = asset.echo_id
		 JOIN echo_rank_snapshots candidate_rank ON candidate_rank.echo_id = e.id
		 WHERE transcript.status = 'succeeded' AND asset.status = 'approved' AND asset.deleted_at IS NULL
		 AND e.status = 'published' AND e.deleted_at IS NULL
		 AND transcript.search_document @@ websearch_to_tsquery('simple', unaccent($1))` + whereScope + `
		 ORDER BY candidate_rank.notability_score DESC, e.published_at DESC, e.id DESC LIMIT 500)
	), ranked_results AS (
	SELECT e.id,
		e.published_at,
		greatest(0, least(1, 0.60 * greatest(ts_rank_cd(e.search_document, websearch_to_tsquery('simple', unaccent($1))), coalesce(transcript_rank.score, 0)) +
			0.25 * coalesce(rank.notability_score, 0) +
			0.15 * exp(-ln(2) * extract(epoch from (now() - e.published_at)) / 172800)))::float8 AS score,
		coalesce(rank.ranking_reason, 'recent') AS reason
		FROM candidate_ids candidate JOIN echoes e ON e.id = candidate.id
		LEFT JOIN echo_rank_snapshots rank ON rank.echo_id = e.id
		LEFT JOIN LATERAL (
			SELECT max(ts_rank_cd(transcript.search_document, websearch_to_tsquery('simple', unaccent($1)))) AS score
			FROM media_assets asset JOIN media_transcripts transcript ON transcript.media_asset_id = asset.id
			WHERE asset.echo_id = e.id AND asset.status = 'approved' AND asset.deleted_at IS NULL AND transcript.status = 'succeeded'
		) transcript_rank ON true
		WHERE e.status = 'published' AND e.deleted_at IS NULL
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM hidden_targets h WHERE h.user_id = nullif($2, '')::uuid AND h.target_type = 'echo' AND h.target_id = e.id))
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM blocks b WHERE
			(b.blocker_user_id = nullif($2, '')::uuid AND b.blocked_user_id = e.author_user_id) OR
			(b.blocker_user_id = e.author_user_id AND b.blocked_user_id = nullif($2, '')::uuid)))
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM notification_mutes m WHERE m.user_id = nullif($2, '')::uuid
			AND (m.echo_id = e.id OR m.place_id = e.place_id) AND (m.muted_until IS NULL OR m.muted_until > now())))
	)
	SELECT id::text, score, reason, published_at FROM ranked_results` + cursorPredicate + `
		ORDER BY score DESC, published_at DESC, id DESC LIMIT $3`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return SearchResponse{}, err
	}
	type rankedEcho struct {
		ID          string
		Score       float64
		Reason      string
		PublishedAt time.Time
	}
	ranked := make([]rankedEcho, 0, request.Limit+1)
	for rows.Next() {
		var item rankedEcho
		if err = rows.Scan(&item.ID, &item.Score, &item.Reason, &item.PublishedAt); err != nil {
			rows.Close()
			return SearchResponse{}, err
		}
		ranked = append(ranked, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return SearchResponse{}, err
	}
	rows.Close()
	nextCursor := ""
	if len(ranked) > request.Limit {
		ranked = ranked[:request.Limit]
		last := ranked[len(ranked)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return SearchResponse{}, parseErr
		}
		nextCursor, err = pagination.Encode(pagination.Cursor{After: last.PublishedAt, ID: id})
		if err != nil {
			return SearchResponse{}, err
		}
	}
	ids := make([]string, 0, len(ranked))
	for _, item := range ranked {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		places, placeErr := waitForPlaces()
		if placeErr != nil {
			return SearchResponse{}, placeErr
		}
		return SearchResponse{Places: places, Echoes: []EchoDiscoveryResult{}, NextCursor: ""}, nil
	}
	detailRows, err := s.pool.Query(ctx, echoSelect+` WHERE e.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return SearchResponse{}, err
	}
	records := make(map[string]EchoRecord, len(ids))
	for detailRows.Next() {
		record, scanErr := scanEcho(detailRows)
		if scanErr != nil {
			detailRows.Close()
			return SearchResponse{}, scanErr
		}
		records[record.ID] = record
	}
	if err = detailRows.Err(); err != nil {
		detailRows.Close()
		return SearchResponse{}, err
	}
	detailRows.Close()
	results := make([]EchoDiscoveryResult, 0, len(ranked))
	for _, item := range ranked {
		if record, ok := records[item.ID]; ok {
			results = append(results, EchoDiscoveryResult{Echo: toEchoResponse(record, request.ViewerID), Score: item.Score, Label: rankingLabel(item.Reason)})
		}
	}
	places, placeErr := waitForPlaces()
	if placeErr != nil {
		return SearchResponse{}, placeErr
	}
	return SearchResponse{Places: places, Echoes: results, NextCursor: nextCursor}, nil
}

func (s *PostgresStore) NotableEchoes(ctx context.Context, placeID string, latitude, longitude *float64, radiusMeters int, viewerID string, limit int) ([]EchoDiscoveryResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	args := []any{viewerID, limit}
	where := ""
	if placeID != "" {
		where = " AND e.place_id = $3"
		args = append(args, placeID)
	} else if latitude != nil && longitude != nil {
		where = " AND ST_DWithin(e.public_location, ST_SetSRID(ST_MakePoint($4, $3), 4326)::geography, $5)"
		args = append(args, *latitude, *longitude, radiusMeters)
	}
	rows, err := s.pool.Query(ctx, echoSelect+`
		JOIN echo_rank_snapshots rank ON rank.echo_id = e.id
		WHERE e.status = 'published' AND rank.notability_score > 0`+where+`
		AND (NOT rank.evergreen_eligible OR e.id IN (
			SELECT evergreen.echo_id FROM echo_rank_snapshots evergreen JOIN echoes old_echo ON old_echo.id = evergreen.echo_id
			WHERE evergreen.evergreen_eligible AND old_echo.status = 'published' AND old_echo.deleted_at IS NULL
			ORDER BY evergreen.notability_score DESC, old_echo.published_at DESC
			LIMIT (($2::integer + 19) / 20)))
		AND ($1 = '' OR NOT EXISTS (SELECT 1 FROM hidden_targets h WHERE h.user_id = nullif($1, '')::uuid AND h.target_type = 'echo' AND h.target_id = e.id))
		AND ($1 = '' OR NOT EXISTS (SELECT 1 FROM blocks b WHERE
			(b.blocker_user_id = nullif($1, '')::uuid AND b.blocked_user_id = e.author_user_id) OR
			(b.blocker_user_id = e.author_user_id AND b.blocked_user_id = nullif($1, '')::uuid)))
		AND ($1 = '' OR NOT EXISTS (SELECT 1 FROM notification_mutes m WHERE m.user_id = nullif($1, '')::uuid
			AND (m.echo_id = e.id OR m.place_id = e.place_id) AND (m.muted_until IS NULL OR m.muted_until > now())))
		ORDER BY rank.notability_score DESC, e.published_at DESC, e.id DESC LIMIT $2`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []EchoDiscoveryResult{}
	for rows.Next() {
		record, scanErr := scanEcho(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		var score float64
		var label string
		if err = s.pool.QueryRow(ctx, `SELECT notability_score::float8, ranking_reason FROM echo_rank_snapshots WHERE echo_id = $1`, record.ID).Scan(&score, &label); err != nil {
			return nil, err
		}
		results = append(results, EchoDiscoveryResult{Echo: toEchoResponse(record, viewerID), Score: score, Label: rankingLabel(label)})
	}
	return results, rows.Err()
}

func (s *PostgresStore) echoScore(ctx context.Context, echoID, query string) (float64, string, error) {
	var textScore, notability, recency float64
	var reason string
	err := s.pool.QueryRow(ctx, `SELECT greatest(ts_rank_cd(e.search_document, websearch_to_tsquery('simple', unaccent($2))), coalesce((
		SELECT max(ts_rank_cd(transcript.search_document, websearch_to_tsquery('simple', unaccent($2))))
		FROM media_assets asset JOIN media_transcripts transcript ON transcript.media_asset_id = asset.id
		WHERE asset.echo_id = e.id AND asset.status = 'approved' AND asset.deleted_at IS NULL AND transcript.status = 'succeeded'
	), 0))::float8,
		coalesce(r.notability_score, 0)::float8,
		exp(-ln(2) * extract(epoch from (now() - e.published_at)) / 172800)::float8,
		coalesce(r.ranking_reason, 'recent') FROM echoes e LEFT JOIN echo_rank_snapshots r ON r.echo_id = e.id WHERE e.id = $1`, echoID, query).
		Scan(&textScore, &notability, &recency, &reason)
	if err != nil {
		return 0, "", err
	}
	return math.Max(0, math.Min(1, .60*textScore+.25*notability+.15*recency)), rankingLabel(reason), nil
}

func rankingLabel(reason string) string {
	switch reason {
	case "popular_nearby":
		return "Popular nearby"
	case "subscribed_place":
		return "From a subscribed Place"
	case "notable":
		return "Notable"
	default:
		return "Recent"
	}
}
