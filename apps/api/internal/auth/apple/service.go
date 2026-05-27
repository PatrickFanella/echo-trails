package apple

import (
	"context"
	"errors"
	"strings"
)

const Provider = "apple"

var (
	ErrInvalidIdentityToken = errors.New("invalid apple identity token")
	ErrUnverifiedEmail      = errors.New("apple email is not verified")
	ErrMissingSubject       = errors.New("apple subject is missing")
	ErrEmailConflict        = errors.New("email belongs to a different user")
)

type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
}

type User struct {
	ID    string
	Email string
}

type Identity struct {
	Provider string
	Subject  string
	UserID   string
	Email    string
}

type AuthResult struct {
	User     User
	Identity Identity
	Linked   bool
}

type IdentityVerifier interface {
	VerifyIdentityToken(ctx context.Context, identityToken string) (Claims, error)
}

type IdentityStore interface {
	FindIdentity(ctx context.Context, provider, subject string) (Identity, bool, error)
	FindUserByID(ctx context.Context, userID string) (User, bool, error)
	FindUserByEmail(ctx context.Context, email string) (User, bool, error)
	CreateUserWithIdentity(ctx context.Context, email string, identity Identity) (User, Identity, error)
	LinkIdentity(ctx context.Context, userID string, identity Identity) (Identity, error)
}

type Service struct {
	verifier IdentityVerifier
	store    IdentityStore
}

func NewService(verifier IdentityVerifier, store IdentityStore) *Service {
	return &Service{verifier: verifier, store: store}
}

func (s *Service) SignIn(ctx context.Context, identityToken string) (AuthResult, error) {
	claims, err := s.verifier.VerifyIdentityToken(ctx, strings.TrimSpace(identityToken))
	if err != nil {
		return AuthResult{}, err
	}
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return AuthResult{}, ErrMissingSubject
	}

	existing, ok, err := s.store.FindIdentity(ctx, Provider, subject)
	if err != nil {
		return AuthResult{}, err
	}
	if ok {
		user, found, err := s.store.FindUserByID(ctx, existing.UserID)
		if err != nil {
			return AuthResult{}, err
		}
		if !found {
			return AuthResult{}, ErrEmailConflict
		}
		return AuthResult{User: user, Identity: existing, Linked: false}, nil
	}

	email := normalizeEmail(claims.Email)
	if email == "" || !claims.EmailVerified {
		return AuthResult{}, ErrUnverifiedEmail
	}

	identity := Identity{Provider: Provider, Subject: subject, Email: email}
	user, found, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, err
	}
	if found {
		linked, err := s.store.LinkIdentity(ctx, user.ID, identity)
		if err != nil {
			return AuthResult{}, err
		}
		return AuthResult{User: user, Identity: linked, Linked: true}, nil
	}

	createdUser, createdIdentity, err := s.store.CreateUserWithIdentity(ctx, email, identity)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: createdUser, Identity: createdIdentity, Linked: true}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
