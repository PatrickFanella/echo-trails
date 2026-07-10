package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the authoritative production repository for authentication,
// profiles, creation authorization, and Echo lifecycle state.
type PostgresStore struct {
	pool          *pgxpool.Pool
	exactLocation cipher.AEAD
	pushTokens    cipher.AEAD
}

func NewPostgresStore(pool *pgxpool.Pool, exactLocationKey, pushTokenKey []byte) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	if len(exactLocationKey) < 32 {
		return nil, errors.New("exact location encryption key must be at least 32 bytes")
	}
	if len(pushTokenKey) < 32 {
		return nil, errors.New("push token encryption key must be at least 32 bytes")
	}
	aead, err := newAEAD(exactLocationKey)
	if err != nil {
		return nil, err
	}
	pushAEAD, err := newAEAD(pushTokenKey)
	if err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool, exactLocation: aead, pushTokens: pushAEAD}, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	digest := sha256.Sum256(key)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *PostgresStore) CreateAuthRequest(ctx context.Context, request AuthRequest) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO email_auth_requests (id, email, code_hash, attempts, expires_at, used_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, request.ID, request.Email, decodeHex(request.CodeHash), request.Attempts, request.ExpiresAt.UTC(), request.UsedAt)
	return err
}

func (s *PostgresStore) GetAuthRequest(ctx context.Context, id string) (AuthRequest, bool) {
	var request AuthRequest
	var codeHashBytes []byte
	err := s.pool.QueryRow(ctx, `SELECT id::text, email, code_hash, attempts, expires_at, used_at FROM email_auth_requests WHERE id = $1`, id).
		Scan(&request.ID, &request.Email, &codeHashBytes, &request.Attempts, &request.ExpiresAt, &request.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthRequest{}, false
	}
	if err != nil {
		return AuthRequest{}, false
	}
	request.CodeHash = hex.EncodeToString(codeHashBytes)
	return request, true
}

func (s *PostgresStore) ConsumeAuthRequest(ctx context.Context, id, submittedHash string, now time.Time) (AuthRequest, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AuthRequest{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var request AuthRequest
	var storedHash []byte
	err = tx.QueryRow(ctx, `SELECT id::text, email, code_hash, attempts, expires_at, used_at
		FROM email_auth_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&request.ID, &request.Email, &storedHash, &request.Attempts, &request.ExpiresAt, &request.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthRequest{}, false, nil
	}
	if err != nil {
		return AuthRequest{}, false, err
	}
	request.CodeHash = hex.EncodeToString(storedHash)
	if request.UsedAt != nil || !now.Before(request.ExpiresAt) || request.Attempts >= 5 {
		return AuthRequest{}, false, nil
	}
	if !constantTimeEqualHex(request.CodeHash, submittedHash) {
		_, err = tx.Exec(ctx, `UPDATE email_auth_requests SET attempts = attempts + 1 WHERE id = $1`, id)
		if err != nil {
			return AuthRequest{}, false, err
		}
		return AuthRequest{}, false, tx.Commit(ctx)
	}
	usedAt := now.UTC()
	if _, err = tx.Exec(ctx, `UPDATE email_auth_requests SET used_at = $2 WHERE id = $1`, id, usedAt); err != nil {
		return AuthRequest{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AuthRequest{}, false, err
	}
	request.UsedAt = &usedAt
	return request, true, nil
}

func (s *PostgresStore) FindOrCreateUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `SELECT id::text, email FROM users WHERE email = $1 AND status <> 'deleted'`, email).Scan(&user.ID, &user.Email)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	id, err := uuidv7.New()
	if err != nil {
		return User{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email RETURNING id::text, email, role, status`, id.String(), email).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
	return user, err
}

func (s *PostgresStore) CreateSession(ctx context.Context, session Session) error {
	accessHash := decodeHex(session.AccessTokenHash)
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions
		(id, user_id, token_hash, access_token_hash, refresh_token_hash, device_name, access_expires_at, expires_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6, $7)`, session.ID, session.UserID, accessHash,
		decodeHex(session.RefreshTokenHash), session.DeviceName, session.AccessExpiresAt.UTC(), session.ExpiresAt.UTC())
	return err
}

func (s *PostgresStore) AuthenticateAccessToken(ctx context.Context, hash string, now time.Time) (User, bool, error) {
	var user User
	err := s.pool.QueryRow(ctx, `SELECT u.id::text, coalesce(u.email, ''), u.role, u.status FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.access_token_hash = $1 AND s.revoked_at IS NULL AND s.access_expires_at > $2
		AND u.status IN ('active', 'restricted')`, decodeHex(hash), now.UTC()).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	return user, err == nil, err
}

func (s *PostgresStore) RotateSessionRefreshToken(ctx context.Context, currentHash, newRefreshHash, newAccessHash string, newRefreshExpiresAt, newAccessExpiresAt, now time.Time) (Session, bool, error) {
	var session Session
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `UPDATE sessions s SET refresh_token_hash = $2, access_token_hash = $3,
		token_hash = $3, expires_at = $4, access_expires_at = $5, last_seen_at = $6
		FROM users u WHERE s.user_id = u.id AND s.refresh_token_hash = $1 AND s.revoked_at IS NULL
		AND s.expires_at > $6 AND u.status IN ('active', 'restricted')
		RETURNING s.id::text, s.user_id::text, s.device_name, s.expires_at, s.access_expires_at, s.revoked_at`,
		decodeHex(currentHash), decodeHex(newRefreshHash), decodeHex(newAccessHash), newRefreshExpiresAt.UTC(), newAccessExpiresAt.UTC(), now.UTC()).
		Scan(&session.ID, &session.UserID, &session.DeviceName, &session.ExpiresAt, &session.AccessExpiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	session.RefreshTokenHash = newRefreshHash
	session.AccessTokenHash = newAccessHash
	session.RevokedAt = revokedAt
	return session, true, nil
}

func (s *PostgresStore) RotateSessionRefreshTokenIdempotent(ctx context.Context, currentHash, key, newRefreshHash, newAccessHash string, refreshExpiry, accessExpiry, now time.Time) (Session, bool, error) {
	if strings.TrimSpace(key) == "" {
		return Session{}, false, nil
	}
	requestHash := decodeHex(currentHash)
	loadReplay := func() (Session, bool, error) {
		var session Session
		err := s.pool.QueryRow(ctx, `SELECT s.id::text, s.user_id::text, s.device_name, r.refresh_expires_at, r.access_expires_at, s.revoked_at
			FROM session_refresh_replays r JOIN sessions s ON s.id = r.session_id
			WHERE r.request_refresh_hash = $1 AND r.idempotency_key = $2 AND r.expires_at > $3`, requestHash, key, now.UTC()).
			Scan(&session.ID, &session.UserID, &session.DeviceName, &session.ExpiresAt, &session.AccessExpiresAt, &session.RevokedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, false, nil
		}
		return session, err == nil, err
	}
	if replay, found, err := loadReplay(); found || err != nil {
		return replay, found, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var session Session
	err = tx.QueryRow(ctx, `UPDATE sessions s SET refresh_token_hash = $2, access_token_hash = $3, token_hash = $3,
		expires_at = $4, access_expires_at = $5, last_seen_at = $6 FROM users u
		WHERE s.user_id = u.id AND s.refresh_token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $6
		AND u.status IN ('active', 'restricted') RETURNING s.id::text, s.user_id::text, s.device_name, s.expires_at, s.access_expires_at, s.revoked_at`,
		requestHash, decodeHex(newRefreshHash), decodeHex(newAccessHash), refreshExpiry.UTC(), accessExpiry.UTC(), now.UTC()).
		Scan(&session.ID, &session.UserID, &session.DeviceName, &session.ExpiresAt, &session.AccessExpiresAt, &session.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return loadReplay()
	}
	if err != nil {
		return Session{}, false, err
	}
	id, err := uuidv7.New()
	if err != nil {
		return Session{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO session_refresh_replays
		(id, session_id, request_refresh_hash, idempotency_key, access_expires_at, refresh_expires_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id.String(), session.ID, requestHash, key, accessExpiry.UTC(), refreshExpiry.UTC(), now.Add(24*time.Hour).UTC())
	if err != nil {
		return Session{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, false, err
	}
	return session, true, nil
}

func (s *PostgresStore) RevokeSessionByRefreshTokenHash(ctx context.Context, hash string, revokedAt time.Time) (bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE refresh_token_hash = $1 AND revoked_at IS NULL`, decodeHex(hash), revokedAt.UTC())
	return result.RowsAffected() > 0, err
}

func (s *PostgresStore) GetProfileByUserID(ctx context.Context, userID string) (Profile, bool, error) {
	return s.getProfile(ctx, `WHERE u.id = $1`, userID)
}

func (s *PostgresStore) GetProfileByHandle(ctx context.Context, handle string) (Profile, bool, error) {
	return s.getProfile(ctx, `WHERE p.handle = $1 AND u.status NOT IN ('banned', 'deleted')`, NormalizeHandle(handle))
}

func (s *PostgresStore) getProfile(ctx context.Context, predicate string, value string) (Profile, bool, error) {
	var profile Profile
	err := s.pool.QueryRow(ctx, `SELECT u.id::text, coalesce(u.email, ''), p.handle::text, coalesce(p.display_name, ''),
		coalesce(p.bio, ''), p.created_at, p.updated_at, p.handle_changed_at, coalesce(p.avatar_media_asset_id::text, '') FROM user_profiles p JOIN users u ON u.id = p.user_id `+predicate, value).
		Scan(&profile.UserID, &profile.Email, &profile.Handle, &profile.DisplayName, &profile.Bio, &profile.CreatedAt, &profile.UpdatedAt, &profile.HandleChangedAt, &profile.AvatarMediaAssetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, nil
	}
	return profile, err == nil, err
}

func (s *PostgresStore) SetAvatar(ctx context.Context, userID, mediaAssetID string, now time.Time) (Profile, error) {
	command, err := s.pool.Exec(ctx, `UPDATE user_profiles profile
		SET avatar_media_asset_id = asset.id, updated_at = $3
		FROM media_assets asset
		WHERE profile.user_id = $1 AND asset.id = $2 AND asset.owner_user_id = $1
		AND asset.media_type = 'image' AND asset.status = 'approved' AND asset.deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM media_derivatives derivative WHERE derivative.media_asset_id = asset.id
			AND derivative.derivative_type IN ('preview', 'thumbnail'))`, userID, mediaAssetID, now.UTC())
	if err != nil {
		return Profile{}, err
	}
	if command.RowsAffected() != 1 {
		return Profile{}, ErrAvatarNotApproved
	}
	profile, found, err := s.GetProfileByUserID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (s *PostgresStore) ClearAvatar(ctx context.Context, userID string, now time.Time) (Profile, error) {
	command, err := s.pool.Exec(ctx, `UPDATE user_profiles SET avatar_media_asset_id = NULL, updated_at = $2 WHERE user_id = $1`, userID, now.UTC())
	if err != nil {
		return Profile{}, err
	}
	if command.RowsAffected() != 1 {
		return Profile{}, ErrProfileNotFound
	}
	profile, found, err := s.GetProfileByUserID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (s *PostgresStore) UpsertProfile(ctx context.Context, userID, email string, request UpsertProfileRequest, now time.Time) (Profile, error) {
	handle, err := ValidateHandle(request.Handle)
	if err != nil {
		return Profile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentHandle string
	var changedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT handle::text, handle_changed_at FROM user_profiles WHERE user_id = $1 FOR UPDATE`, userID).Scan(&currentHandle, &changedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, err
	}
	if currentHandle != "" && currentHandle != handle && changedAt != nil && now.Before(changedAt.Add(handleChangeCooldown)) {
		return Profile{}, ErrHandleChangeTooSoon
	}
	if currentHandle == "" || currentHandle != handle {
		changed := now.UTC()
		changedAt = &changed
	}
	displayName := normalizeDisplayName(request.DisplayName, handle)
	bio := normalizeBio(request.Bio)
	_, err = tx.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name, bio, handle_changed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (user_id) DO UPDATE SET handle = EXCLUDED.handle, display_name = EXCLUDED.display_name,
		bio = EXCLUDED.bio, handle_changed_at = EXCLUDED.handle_changed_at, updated_at = EXCLUDED.updated_at`,
		userID, handle, displayName, bio, changedAt, now.UTC())
	if isUniqueViolation(err) {
		return Profile{}, ErrHandleTaken
	}
	if err != nil {
		return Profile{}, err
	}
	if email != "" {
		if _, err = tx.Exec(ctx, `UPDATE users SET email = $2, updated_at = $3 WHERE id = $1`, userID, strings.TrimSpace(email), now.UTC()); err != nil {
			return Profile{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	profile, found, err := s.GetProfileByUserID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (s *PostgresStore) Save(ctx context.Context, authorization CreationAuthorization) (CreationAuthorization, error) {
	if authorization.ID == "" {
		id, err := uuidv7.New()
		if err != nil {
			return CreationAuthorization{}, err
		}
		authorization.ID = id.String()
	}
	ciphertext, err := s.encryptCoordinate(authorization.Coordinate)
	if err != nil {
		return CreationAuthorization{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO creation_authorizations
		(id, token_hash, user_id, place_id, exact_location_ciphertext, proof_captured_at, accuracy_meters, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, authorization.ID, decodeHex(authorization.TokenHash), authorization.UserID,
		authorization.PlaceID, ciphertext, authorization.CapturedAt.UTC(), authorization.AccuracyMeters, authorization.ExpiresAt.UTC())
	return authorization, err
}

func (s *PostgresStore) Consume(ctx context.Context, token, userID, placeID, idempotencyKey string, now time.Time) (CreationAuthorization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreationAuthorization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var authorization CreationAuthorization
	var tokenHashBytes, ciphertext []byte
	err = tx.QueryRow(ctx, `SELECT id::text, token_hash, user_id::text, place_id::text, exact_location_ciphertext,
		proof_captured_at, accuracy_meters, expires_at, coalesce(consumed_idempotency_key, '') FROM creation_authorizations
		WHERE token_hash = $1 FOR UPDATE`, decodeHex(hashCreationToken(token))).Scan(&authorization.ID, &tokenHashBytes,
		&authorization.UserID, &authorization.PlaceID, &ciphertext, &authorization.CapturedAt, &authorization.AccuracyMeters, &authorization.ExpiresAt, &authorization.ConsumedIdempotencyKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreationAuthorization{}, ErrCreationProofMissing
	}
	if err != nil {
		return CreationAuthorization{}, err
	}
	if authorization.UserID != userID || authorization.PlaceID != placeID || !authorization.ExpiresAt.After(now.UTC()) {
		return CreationAuthorization{}, ErrCreationProofMissing
	}
	if authorization.ConsumedIdempotencyKey != "" && authorization.ConsumedIdempotencyKey != idempotencyKey {
		return CreationAuthorization{}, ErrCreationProofUsed
	}
	authorization.TokenHash = hex.EncodeToString(tokenHashBytes)
	authorization.Coordinate, err = s.decryptCoordinate(ciphertext)
	if err != nil {
		return CreationAuthorization{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE creation_authorizations SET consumed_idempotency_key = $2, consumed_at = coalesce(consumed_at, $3) WHERE id = $1`, authorization.ID, idempotencyKey, now.UTC()); err != nil {
		return CreationAuthorization{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CreationAuthorization{}, err
	}
	authorization.ConsumedIdempotencyKey = idempotencyKey
	return authorization, nil
}

func (s *PostgresStore) CreateDraft(ctx context.Context, input EchoCreateInput) (EchoRecord, error) {
	fingerprint := decodeHex(echoFingerprint(input))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EchoRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservationID, err := uuidv7.New()
	if err != nil {
		return EchoRecord{}, err
	}
	result, err := tx.Exec(ctx, `INSERT INTO idempotency_keys (id, user_id, key, request_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (user_id, key) DO NOTHING`, reservationID.String(), input.AuthorUserID,
		input.IdempotencyKey, fingerprint, input.Now.Add(24*time.Hour).UTC())
	if err != nil {
		return EchoRecord{}, err
	}
	if result.RowsAffected() == 0 {
		var storedFingerprint []byte
		var responseBody []byte
		if err = tx.QueryRow(ctx, `SELECT request_hash, response_body FROM idempotency_keys WHERE user_id = $1 AND key = $2 FOR UPDATE`, input.AuthorUserID, input.IdempotencyKey).Scan(&storedFingerprint, &responseBody); err != nil {
			return EchoRecord{}, err
		}
		if !constantTimeEqualHex(hex.EncodeToString(storedFingerprint), hex.EncodeToString(fingerprint)) {
			return EchoRecord{}, ErrEchoConflict
		}
		var response struct {
			EchoID string `json:"echo_id"`
		}
		if err = json.Unmarshal(responseBody, &response); err != nil || response.EchoID == "" {
			return EchoRecord{}, ErrEchoConflict
		}
		record, err := getEchoWith(ctx, tx, response.EchoID)
		if err != nil {
			return EchoRecord{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return EchoRecord{}, err
		}
		return record, nil
	}
	echoID, err := uuidv7.NewAt(input.Now, rand.Reader)
	if err != nil {
		return EchoRecord{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, caption_text, status, public_location,
		location_proof_captured_at, creation_token_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, nullif($6, ''), nullif($7, ''), 'draft',
		ST_SetSRID(ST_MakePoint($8, $9), 4326)::geography, $10, $11, $12, $12)`, echoID.String(), input.AuthorUserID,
		input.Request.PlaceID, input.Request.Title, input.Request.ContentMode, input.Request.BodyText, input.Request.CaptionText,
		input.PublicLocation.Lon, input.PublicLocation.Lat, input.Now.UTC(), input.CreationAuthorizationID, input.Now.UTC())
	if err != nil {
		return EchoRecord{}, err
	}
	responseBody, _ := json.Marshal(map[string]string{"echo_id": echoID.String()})
	if _, err = tx.Exec(ctx, `UPDATE idempotency_keys SET response_status = 201, response_body = $2 WHERE id = $1`, reservationID.String(), responseBody); err != nil {
		return EchoRecord{}, err
	}
	record, err := getEchoWith(ctx, tx, echoID.String())
	if err != nil {
		return EchoRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EchoRecord{}, err
	}
	record.PrivateLocation = input.PrivateLocation
	return record, nil
}

func (s *PostgresStore) Get(ctx context.Context, echoID, requesterID string) (EchoRecord, error) {
	record, err := getEchoWith(ctx, s.pool, echoID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (record.Status == EchoStatusDeleted || record.Status == EchoStatusRemoved)) {
		return EchoRecord{}, ErrEchoNotFound
	}
	if err != nil {
		return EchoRecord{}, err
	}
	if record.Status != EchoStatusPublished && record.AuthorUserID != requesterID {
		return EchoRecord{}, ErrEchoForbidden
	}
	record.StitchSource, err = s.loadStitchSource(ctx, echoID)
	if err != nil {
		return EchoRecord{}, err
	}
	return record, nil
}

func (s *PostgresStore) UpdateDraft(ctx context.Context, echoID, authorID string, patch EchoPatchRequest, now time.Time) (EchoRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EchoRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	record, err := scanEcho(tx.QueryRow(ctx, echoSelect+` WHERE e.id = $1 FOR UPDATE OF e`, echoID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (record.Status == EchoStatusDeleted || record.Status == EchoStatusRemoved)) {
		return EchoRecord{}, ErrEchoNotFound
	}
	if err != nil {
		return EchoRecord{}, err
	}
	if record.AuthorUserID != authorID {
		return EchoRecord{}, ErrEchoForbidden
	}
	if record.Status != EchoStatusDraft {
		return EchoRecord{}, ErrInvalidEcho
	}
	updated, err := validateEchoPatch(record, patch)
	if err != nil {
		return EchoRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE echoes SET title = $3, body_text = nullif($4, ''), caption_text = nullif($5, ''), updated_at = $6
		WHERE id = $1 AND author_user_id = $2 AND status = 'draft'`, echoID, authorID, updated.Title, updated.BodyText, updated.CaptionText, now.UTC()); err != nil {
		return EchoRecord{}, err
	}
	updated.UpdatedAt = now.UTC()
	if err = tx.Commit(ctx); err != nil {
		return EchoRecord{}, err
	}
	return updated, nil
}

func (s *PostgresStore) Delete(ctx context.Context, echoID, authorID string, now time.Time) error {
	result, err := s.pool.Exec(ctx, `UPDATE echoes SET status = 'deleted', deleted_at = coalesce(deleted_at, $3), updated_at = $3
		WHERE id = $1 AND author_user_id = $2 AND status <> 'removed'`, echoID, authorID, now.UTC())
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		return nil
	}
	var ownerID, status string
	err = s.pool.QueryRow(ctx, `SELECT author_user_id::text, status FROM echoes WHERE id = $1`, echoID).Scan(&ownerID, &status)
	if errors.Is(err, pgx.ErrNoRows) || status == EchoStatusRemoved {
		return ErrEchoNotFound
	}
	if err != nil {
		return err
	}
	if ownerID != authorID {
		return ErrEchoForbidden
	}
	return nil
}

func (s *PostgresStore) loadStitchSource(ctx context.Context, targetEchoID string) (*StitchSourceResponse, error) {
	var source StitchSourceResponse
	var snapshotAssetID, snapshotStatus, sourceStatus string
	err := s.pool.QueryRow(ctx, `SELECT source.id::text, source.title, profile.handle::text, source.status,
		coalesce(link.quoted_reply_id::text, ''), coalesce(link.quote_snapshot_media_asset_id::text, ''), link.snapshot_status
		FROM echo_links link JOIN echoes source ON source.id = link.source_echo_id
		JOIN user_profiles profile ON profile.user_id = source.author_user_id
		WHERE link.target_echo_id = $1 AND link.link_type = 'stitch'`, targetEchoID).
		Scan(&source.EchoID, &source.Title, &source.AuthorHandle, &sourceStatus, &source.QuotedReplyID, &snapshotAssetID, &snapshotStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sourceStatus == EchoStatusRemoved || sourceStatus == EchoStatusDeleted {
		source.Title = "Source Echo unavailable"
		source.AuthorHandle = ""
	}
	if source.QuotedReplyID != "" {
		if snapshotStatus == "ready" && snapshotAssetID != "" {
			source.QuoteSnapshotURL = "/media/assets/" + snapshotAssetID
		} else if snapshotStatus == "suppressed" {
			source.QuoteUnavailable = true
		}
	}
	return &source, nil
}

func (s *PostgresStore) Submit(ctx context.Context, echoID, authorID string, now time.Time) (EchoRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EchoRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ownerID, contentMode, currentStatus string
	err = tx.QueryRow(ctx, `SELECT author_user_id::text, content_mode, status FROM echoes WHERE id = $1 FOR UPDATE`, echoID).
		Scan(&ownerID, &contentMode, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return EchoRecord{}, ErrEchoNotFound
	}
	if err != nil {
		return EchoRecord{}, err
	}
	if ownerID != authorID {
		return EchoRecord{}, ErrEchoForbidden
	}
	if currentStatus == EchoStatusPublished || currentStatus == EchoStatusUploading || currentStatus == "processing" || currentStatus == "pending_review" {
		return s.Get(ctx, echoID, authorID)
	}
	if currentStatus != EchoStatusDraft {
		return EchoRecord{}, ErrInvalidEcho
	}
	mode := EchoContentMode(contentMode)
	if mode != EchoContentText {
		var total, matching, completed, distinctOrder int
		expectedType := map[EchoContentMode]string{EchoContentPhotoSet: "image", EchoContentAudio: "audio", EchoContentVideo: "video"}[mode]
		if expectedType == "" {
			return EchoRecord{}, ErrInvalidEcho
		}
		if err = tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE media_type = $2),
			count(*) FILTER (WHERE status <> 'pending_upload'), count(DISTINCT sort_order)
			FROM media_assets WHERE echo_id = $1 AND media_type <> 'quote_snapshot'
			AND deleted_at IS NULL AND status <> 'deleted'`, echoID, expectedType).
			Scan(&total, &matching, &completed, &distinctOrder); err != nil {
			return EchoRecord{}, err
		}
		validCount := mode == EchoContentPhotoSet && total >= 1 && total <= 5
		if mode == EchoContentAudio || mode == EchoContentVideo {
			validCount = total == 1
		}
		if !validCount || matching != total || completed != total || distinctOrder != total {
			return EchoRecord{}, ErrInvalidEcho
		}
	}
	status := EchoStatusUploading
	jobType, jobQueue := "scan_echo_safety", "moderation"
	if mode == EchoContentText {
		status = "processing"
	}
	if _, err = tx.Exec(ctx, `UPDATE echoes SET status = $3, published_at = NULL, updated_at = $4 WHERE id = $1 AND author_user_id = $2 AND status = 'draft'`, echoID, authorID, status, now.UTC()); err != nil {
		return EchoRecord{}, err
	}
	if jobType != "" {
		jobID, idErr := uuidv7.New()
		if idErr != nil {
			return EchoRecord{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts) VALUES ($1, $2, $3, jsonb_build_object('echo_id', $4::text), 3)`, jobID.String(), jobQueue, jobType, echoID); err != nil {
			return EchoRecord{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return EchoRecord{}, err
	}
	return s.Get(ctx, echoID, authorID)
}

func (s *PostgresStore) ListPublishedByPlace(ctx context.Context, placeID string, limit int) ([]EchoRecord, error) {
	page, err := s.ListPublishedByPlacePage(ctx, placeID, "", limit)
	return page.Records, err
}

func (s *PostgresStore) ListPublishedByPlacePage(ctx context.Context, placeID, cursorValue string, limit int) (EchoRecordPage, error) {
	return s.ListPublishedByPlaceSortedPage(ctx, placeID, cursorValue, limit, "notable")
}

func (s *PostgresStore) ListPublishedByPlaceSortedPage(ctx context.Context, placeID, cursorValue string, limit int, order string) (EchoRecordPage, error) {
	return s.ListVisiblePublishedByPlaceSortedPage(ctx, placeID, "", cursorValue, limit, order)
}

func (s *PostgresStore) ListVisiblePublishedByPlaceSortedPage(ctx context.Context, placeID, viewerID, cursorValue string, limit int, order string) (EchoRecordPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if order != "oldest" && order != "notable" {
		order = "recent"
	}
	query := echoSelect
	if order == "notable" {
		query += ` LEFT JOIN echo_rank_snapshots rank ON rank.echo_id = e.id`
	}
	query += ` WHERE e.place_id = $1 AND e.status = 'published' AND e.deleted_at IS NULL
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM blocks block WHERE
			(block.blocker_user_id = nullif($2, '')::uuid AND block.blocked_user_id = e.author_user_id) OR
			(block.blocker_user_id = e.author_user_id AND block.blocked_user_id = nullif($2, '')::uuid)))
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM hidden_targets hidden
			WHERE hidden.user_id = nullif($2, '')::uuid AND hidden.target_type = 'echo' AND hidden.target_id = e.id))`
	args := []any{placeID, viewerID, limit + 1}
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return EchoRecordPage{}, err
		}
		switch order {
		case "oldest":
			query += ` AND (e.published_at, e.id) > ($4, $5)`
		case "notable":
			query += ` AND (coalesce(rank.notability_score, 0), e.published_at, e.id) <
				(SELECT coalesce(cursor_rank.notability_score, 0), cursor_echo.published_at, cursor_echo.id
				 FROM echoes cursor_echo LEFT JOIN echo_rank_snapshots cursor_rank ON cursor_rank.echo_id = cursor_echo.id
				 WHERE cursor_echo.id = $5 AND cursor_echo.published_at = $4)`
		default:
			query += ` AND (e.published_at, e.id) < ($4, $5)`
		}
		args = append(args, cursor.After, cursor.ID.String())
	}
	switch order {
	case "oldest":
		query += ` ORDER BY e.published_at, e.id LIMIT $3`
	case "notable":
		query += ` ORDER BY coalesce(rank.notability_score, 0) DESC, e.published_at DESC, e.id DESC LIMIT $3`
	default:
		query += ` ORDER BY e.published_at DESC, e.id DESC LIMIT $3`
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return EchoRecordPage{}, err
	}
	defer rows.Close()
	result := make([]EchoRecord, 0, limit+1)
	for rows.Next() {
		record, err := scanEcho(rows)
		if err != nil {
			return EchoRecordPage{}, err
		}
		result = append(result, record)
	}
	if err = rows.Err(); err != nil {
		return EchoRecordPage{}, err
	}
	if err = s.hydrateEchoRelations(ctx, result); err != nil {
		return EchoRecordPage{}, err
	}
	page := EchoRecordPage{Records: result}
	if len(result) > limit {
		page.Records = result[:limit]
		last := page.Records[len(page.Records)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return EchoRecordPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: echoPageTime(last), ID: id})
		if err != nil {
			return EchoRecordPage{}, err
		}
	}
	return page, nil
}

func (s *PostgresStore) ListPublishedByAuthorHandlePage(ctx context.Context, handle, cursorValue string, limit int) (EchoRecordPage, error) {
	return s.ListVisiblePublishedByAuthorHandlePage(ctx, handle, "", cursorValue, limit)
}

func (s *PostgresStore) ListVisiblePublishedByAuthorHandlePage(ctx context.Context, handle, viewerID, cursorValue string, limit int) (EchoRecordPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := echoSelect + ` WHERE p.handle = $1 AND e.status = 'published' AND e.deleted_at IS NULL
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM blocks block WHERE
			(block.blocker_user_id = nullif($2, '')::uuid AND block.blocked_user_id = e.author_user_id) OR
			(block.blocker_user_id = e.author_user_id AND block.blocked_user_id = nullif($2, '')::uuid)))
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM hidden_targets hidden
			WHERE hidden.user_id = nullif($2, '')::uuid AND hidden.target_type = 'echo' AND hidden.target_id = e.id))`
	args := []any{NormalizeHandle(handle), viewerID, limit + 1}
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return EchoRecordPage{}, err
		}
		query += ` AND (e.published_at, e.id) < ($4, $5)`
		args = append(args, cursor.After, cursor.ID.String())
	}
	query += ` ORDER BY e.published_at DESC, e.id DESC LIMIT $3`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return EchoRecordPage{}, err
	}
	defer rows.Close()
	records := make([]EchoRecord, 0, limit+1)
	for rows.Next() {
		record, scanErr := scanEcho(rows)
		if scanErr != nil {
			return EchoRecordPage{}, scanErr
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return EchoRecordPage{}, err
	}
	if err = s.hydrateEchoRelations(ctx, records); err != nil {
		return EchoRecordPage{}, err
	}
	page := EchoRecordPage{Records: records}
	if len(records) > limit {
		page.Records = records[:limit]
		last := page.Records[len(page.Records)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return EchoRecordPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: echoPageTime(last), ID: id})
		if err != nil {
			return EchoRecordPage{}, err
		}
	}
	return page, nil
}

func (s *PostgresStore) hydrateEchoRelations(ctx context.Context, records []EchoRecord) error {
	for index := range records {
		source, err := s.loadStitchSource(ctx, records[index].ID)
		if err != nil {
			return err
		}
		records[index].StitchSource = source
	}
	return nil
}

const placeSelect = `SELECT p.id::text, p.name, p.status, ST_Y(p.location::geometry), ST_X(p.location::geometry),
	p.default_radius_meters::float8, coalesce(p.category, ''), p.sensitive, coalesce(p.merged_into_place_id::text, ''),
	coalesce((SELECT jsonb_agg(a.alias ORDER BY a.alias) FROM place_aliases a WHERE a.place_id = p.id), '[]'::jsonb),
	coalesce((SELECT jsonb_agg(jsonb_build_object('provider', m.provider, 'external_id', m.provider_place_id) ORDER BY m.provider, m.provider_place_id)
		FROM place_provider_mappings m WHERE m.place_id = p.id), '[]'::jsonb)
	FROM places p`

// CreationLocationAllowed applies the imported polygon when one is available,
// including the reviewed 25-metre access buffer. Point-only Places fall back to
// their curated radius, capped by schema policy rather than client input.
func (s *PostgresStore) CreationLocationAllowed(ctx context.Context, placeID string, latitude, longitude float64) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT CASE
		WHEN p.boundary IS NOT NULL THEN ST_DWithin(
			p.boundary,
			ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography,
			25
		)
		ELSE ST_DWithin(
			p.location,
			ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography,
			p.default_radius_meters
		)
	END
	FROM places p
	WHERE p.id = $1 AND p.status = 'active' AND p.deleted_at IS NULL`, placeID, latitude, longitude).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return allowed, err
}

// ConstrainPublicLocation guarantees that the deterministic public jitter is
// covered by an imported Place polygon. Irregular polygon edges are clamped to
// the nearest covered point; point-only Places keep the reviewed radius result.
func (s *PostgresStore) ConstrainPublicLocation(ctx context.Context, placeID string, candidate Coordinate) (Coordinate, error) {
	var result Coordinate
	err := s.pool.QueryRow(ctx, `WITH selected AS (
		SELECT boundary::geometry AS boundary,
			ST_SetSRID(ST_MakePoint($3, $2), 4326) AS candidate
		FROM places WHERE id = $1 AND status = 'active' AND deleted_at IS NULL
	), constrained AS (
		SELECT CASE
			WHEN boundary IS NULL OR ST_Covers(boundary, candidate) THEN candidate
			ELSE ST_ClosestPoint(boundary, candidate)
		END AS point FROM selected
	)
	SELECT ST_Y(point), ST_X(point) FROM constrained`, placeID, candidate.Lat, candidate.Lon).Scan(&result.Lat, &result.Lon)
	if errors.Is(err, pgx.ErrNoRows) {
		return Coordinate{}, ErrEchoNotFound
	}
	return result, err
}

func (s *PostgresStore) SearchPlaces(ctx context.Context, query string, includeRestricted bool, latitude, longitude *float64) ([]Place, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		rows, err := s.pool.Query(ctx, placeSelect+`
			WHERE p.deleted_at IS NULL AND p.status IN ('active', 'restricted')
			AND ($1 OR p.status <> 'restricted') ORDER BY p.name, p.id LIMIT 50`, includeRestricted)
		if err != nil {
			return nil, err
		}
		return scanPlaces(rows)
	}
	// Generate a small indexed candidate set first. The previous similarity()
	// OR predicates forced a sequential scan of every Place and every alias,
	// which dominated the 10k/100k release-scale search gate.
	rows, err := s.pool.Query(ctx, `WITH candidate_ids AS MATERIALIZED (
		SELECT p.id FROM places p
		WHERE p.search_document @@ websearch_to_tsquery('simple', unaccent($1))
		UNION
		SELECT p.id FROM places p WHERE p.normalized_name % lower(unaccent($1)) OR p.normalized_name LIKE lower(unaccent($1)) || '%'
		UNION
		SELECT a.place_id FROM place_aliases a WHERE a.normalized_alias % lower(unaccent($1)) OR a.normalized_alias LIKE lower(unaccent($1)) || '%'
	), base_ranked AS MATERIALIZED (
		SELECT p.id,
			0.55 * ts_rank_cd(p.search_document, websearch_to_tsquery('simple', unaccent($1))) +
			0.20 * CASE WHEN p.normalized_name = lower(unaccent($1)) THEN 1 WHEN p.normalized_name LIKE lower(unaccent($1)) || '%' THEN 0.8 ELSE similarity(p.normalized_name, lower(unaccent($1))) END +
			0.15 * CASE WHEN $3::float8 IS NULL OR $4::float8 IS NULL THEN 0 ELSE greatest(0, 1 - ST_Distance(p.location, ST_SetSRID(ST_MakePoint($4, $3), 4326)::geography) / 10000) END AS base_score
		FROM places p JOIN candidate_ids candidate ON candidate.id = p.id
		WHERE p.deleted_at IS NULL AND p.status IN ('active', 'restricted') AND ($2 OR p.status <> 'restricted')
		ORDER BY base_score DESC, p.name, p.id LIMIT 200
	), recent_activity AS MATERIALIZED (
		SELECT e.place_id, count(*)::float8 AS safe_count FROM echoes e
		JOIN base_ranked candidate ON candidate.id = e.place_id
		WHERE e.status = 'published' AND e.deleted_at IS NULL AND e.published_at > now() - interval '7 days'
		GROUP BY e.place_id
	)
	`+placeSelect+` JOIN base_ranked candidate ON candidate.id = p.id
		LEFT JOIN recent_activity activity ON activity.place_id = p.id
		WHERE p.deleted_at IS NULL AND p.status IN ('active', 'restricted')
		AND ($2 OR p.status <> 'restricted')
		ORDER BY (candidate.base_score + 0.10 * least(1, coalesce(activity.safe_count, 0) / 10)) DESC, p.name, p.id
		LIMIT 50`, query, includeRestricted, latitude, longitude)
	if err != nil {
		return nil, err
	}
	return scanPlaces(rows)
}

func (s *PostgresStore) NearbyPlaces(ctx context.Context, latitude, longitude float64, radiusMeters int, includeRestricted bool) ([]Place, error) {
	rows, err := s.pool.Query(ctx, placeSelect+`
		WHERE p.deleted_at IS NULL AND p.status IN ('active', 'restricted') AND ($4 OR p.status <> 'restricted')
		AND ST_DWithin(p.location, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
		ORDER BY ST_Distance(p.location, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography), p.id LIMIT 100`,
		latitude, longitude, radiusMeters, includeRestricted)
	if err != nil {
		return nil, err
	}
	return scanPlaces(rows)
}

func (s *PostgresStore) GetPlace(ctx context.Context, id string) (Place, bool, error) {
	place, err := scanPlace(s.pool.QueryRow(ctx, placeSelect+` WHERE p.id = $1 AND p.deleted_at IS NULL`, id))
	if err == nil {
		return place, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Place{}, false, err
	}
	var canonicalID string
	err = s.pool.QueryRow(ctx, `SELECT place_id::text FROM place_aliases WHERE id = $1`, id).Scan(&canonicalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Place{}, false, nil
	}
	if err != nil {
		return Place{}, false, err
	}
	return Place{ID: id, State: PlaceStateMerged, CanonicalID: canonicalID}, true, nil
}

func (s *PostgresStore) AllPlaces(ctx context.Context) ([]Place, error) {
	rows, err := s.pool.Query(ctx, placeSelect+` WHERE p.deleted_at IS NULL ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	return scanPlaces(rows)
}

func (s *PostgresStore) CreatePlaceRequest(ctx context.Context, userID string, request PlaceRequest) (Place, error) {
	if request.Latitude == nil || request.Longitude == nil {
		return Place{}, errors.New("place request coordinates are required")
	}
	id, err := uuidv7.New()
	if err != nil {
		return Place{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Place{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, created_by_user_id)
		VALUES ($1, $2, lower($2), 'pending_review', ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography, 100, $5)`,
		id.String(), strings.TrimSpace(request.Name), *request.Longitude, *request.Latitude, userID)
	if err != nil {
		return Place{}, err
	}
	if alias := strings.TrimSpace(request.Alias); alias != "" {
		aliasID, idErr := uuidv7.New()
		if idErr != nil {
			return Place{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO place_aliases (id, place_id, alias, normalized_alias) VALUES ($1, $2, $3, lower($3))`, aliasID.String(), id.String(), alias); err != nil {
			return Place{}, err
		}
	}
	if provider, externalID := strings.TrimSpace(request.Provider), strings.TrimSpace(request.ExternalID); provider != "" && externalID != "" {
		mappingID, idErr := uuidv7.New()
		if idErr != nil {
			return Place{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO place_provider_mappings (id, place_id, provider, provider_place_id) VALUES ($1, $2, $3, $4)`, mappingID.String(), id.String(), provider, externalID); err != nil {
			return Place{}, err
		}
	}
	place, err := scanPlace(tx.QueryRow(ctx, placeSelect+` WHERE p.id = $1`, id.String()))
	if err != nil {
		return Place{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Place{}, err
	}
	return place, nil
}

func scanPlaces(rows pgx.Rows) ([]Place, error) {
	defer rows.Close()
	result := make([]Place, 0)
	for rows.Next() {
		place, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, place)
	}
	return result, rows.Err()
}

func scanPlace(row rowScanner) (Place, error) {
	var place Place
	var state string
	var latitude, longitude float64
	var aliasesJSON, mappingsJSON []byte
	err := row.Scan(&place.ID, &place.Name, &state, &latitude, &longitude, &place.RadiusMeters, &place.Category,
		&place.Sensitive, &place.CanonicalID, &aliasesJSON, &mappingsJSON)
	if err != nil {
		return Place{}, err
	}
	place.State = PlaceState(state)
	place.Latitude, place.Longitude = &latitude, &longitude
	if err = json.Unmarshal(aliasesJSON, &place.Aliases); err != nil {
		return Place{}, err
	}
	if err = json.Unmarshal(mappingsJSON, &place.ProviderMappings); err != nil {
		return Place{}, err
	}
	return place, nil
}

const echoSelect = `SELECT e.id::text, e.author_user_id::text, p.handle::text, e.place_id::text, e.content_mode,
	e.title, coalesce(e.body_text, ''), coalesce(e.caption_text, ''), e.status,
	ST_Y(e.public_location::geometry), ST_X(e.public_location::geometry), e.created_at, e.updated_at, e.published_at,
	ca.exact_location_ciphertext,
	COALESCE((SELECT jsonb_agg(jsonb_build_object(
		'id', a.id::text, 'media_type', a.media_type, 'status', a.status,
		'public_url', '/media/assets/' || a.id::text,
		'duration_seconds', CASE WHEN a.duration_ms IS NULL THEN NULL ELSE a.duration_ms / 1000.0 END,
		'transcript', (SELECT nullif(t.transcript_text, '') FROM media_transcripts t
			WHERE t.media_asset_id = a.id AND t.status = 'succeeded' ORDER BY t.updated_at DESC LIMIT 1)
	) ORDER BY a.sort_order, a.created_at) FROM media_assets a
	WHERE a.echo_id = e.id AND a.status = 'approved' AND a.deleted_at IS NULL AND a.media_type <> 'quote_snapshot'), '[]'::jsonb)
	FROM echoes e JOIN user_profiles p ON p.user_id = e.author_user_id
	LEFT JOIN creation_authorizations ca ON ca.id = e.creation_token_id`

type rowScanner interface{ Scan(...any) error }
type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getEchoWith(ctx context.Context, queryer queryRower, echoID string) (EchoRecord, error) {
	return scanEcho(queryer.QueryRow(ctx, echoSelect+` WHERE e.id = $1`, echoID))
}

func scanEcho(row rowScanner) (EchoRecord, error) {
	var record EchoRecord
	var mode string
	var ciphertext, mediaJSON []byte
	err := row.Scan(&record.ID, &record.AuthorUserID, &record.AuthorHandle, &record.PlaceID, &mode, &record.Title,
		&record.BodyText, &record.CaptionText, &record.Status, &record.PublicLocation.Lat, &record.PublicLocation.Lon,
		&record.CreatedAt, &record.UpdatedAt, &record.PublishedAt, &ciphertext, &mediaJSON)
	record.ContentMode = EchoContentMode(mode)
	if err == nil && len(mediaJSON) > 0 {
		err = json.Unmarshal(mediaJSON, &record.Media)
	}
	return record, err
}

func (s *PostgresStore) LoadExactLocation(ctx context.Context, echoID, requesterID string) (Coordinate, bool, error) {
	var ciphertext []byte
	err := s.pool.QueryRow(ctx, `SELECT ca.exact_location_ciphertext FROM echoes e
		JOIN creation_authorizations ca ON ca.id = e.creation_token_id WHERE e.id = $1 AND e.author_user_id = $2`, echoID, requesterID).Scan(&ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return Coordinate{}, false, nil
	}
	if err != nil {
		return Coordinate{}, false, err
	}
	coordinate, err := s.decryptCoordinate(ciphertext)
	return coordinate, err == nil, err
}

func (s *PostgresStore) encryptCoordinate(coordinate Coordinate) ([]byte, error) {
	plaintext, err := json.Marshal(coordinate)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.exactLocation.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.exactLocation.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *PostgresStore) decryptCoordinate(ciphertext []byte) (Coordinate, error) {
	nonceSize := s.exactLocation.NonceSize()
	if len(ciphertext) < nonceSize {
		return Coordinate{}, errors.New("invalid exact location ciphertext")
	}
	plaintext, err := s.exactLocation.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
	if err != nil {
		return Coordinate{}, err
	}
	var coordinate Coordinate
	err = json.Unmarshal(plaintext, &coordinate)
	return coordinate, err
}

func decodeHex(value string) []byte {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return []byte(value)
	}
	return decoded
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}
