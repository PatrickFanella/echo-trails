package api

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/auth/apple"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) FindIdentity(ctx context.Context, provider, subject string) (apple.Identity, bool, error) {
	var identity apple.Identity
	err := s.pool.QueryRow(ctx, `SELECT provider, provider_subject, user_id::text, coalesce(email, '')
		FROM auth_identities WHERE provider = $1 AND provider_subject = $2`, provider, subject).
		Scan(&identity.Provider, &identity.Subject, &identity.UserID, &identity.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return apple.Identity{}, false, nil
	}
	return identity, err == nil, err
}

func (s *PostgresStore) FindUserByID(ctx context.Context, userID string) (apple.User, bool, error) {
	return s.findAppleUser(ctx, `id = $1`, userID)
}

func (s *PostgresStore) FindUserByEmail(ctx context.Context, email string) (apple.User, bool, error) {
	return s.findAppleUser(ctx, `email = $1`, email)
}

func (s *PostgresStore) findAppleUser(ctx context.Context, predicate, value string) (apple.User, bool, error) {
	var user apple.User
	err := s.pool.QueryRow(ctx, `SELECT id::text, coalesce(email, '') FROM users WHERE `+predicate+` AND status <> 'deleted'`, value).Scan(&user.ID, &user.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return apple.User{}, false, nil
	}
	return user, err == nil, err
}

func (s *PostgresStore) CreateUserWithIdentity(ctx context.Context, email string, identity apple.Identity) (apple.User, apple.Identity, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	userID, err := uuidv7.New()
	if err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, userID.String(), email); err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	identityID, err := uuidv7.New()
	if err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_identities (id, user_id, provider, provider_subject, email, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, identityID.String(), userID.String(), identity.Provider, identity.Subject, email, time.Now().UTC()); err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return apple.User{}, apple.Identity{}, err
	}
	identity.UserID = userID.String()
	identity.Email = email
	return apple.User{ID: userID.String(), Email: email}, identity, nil
}

func (s *PostgresStore) LinkIdentity(ctx context.Context, userID string, identity apple.Identity) (apple.Identity, error) {
	id, err := uuidv7.New()
	if err != nil {
		return apple.Identity{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO auth_identities (id, user_id, provider, provider_subject, email, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, id.String(), userID, identity.Provider, identity.Subject, identity.Email, time.Now().UTC())
	if err != nil {
		return apple.Identity{}, err
	}
	identity.UserID = userID
	return identity, nil
}
