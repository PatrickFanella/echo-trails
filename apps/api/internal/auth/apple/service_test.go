package apple

import (
	"context"
	"errors"
	"testing"
)

func TestSignInWithAppleCreatesUserAndLinksIdentity(t *testing.T) {
	verifier := NewTestVerifier()
	verifier.AddToken("token-1", Claims{Subject: "apple-subject-1", Email: "Test@Example.com", EmailVerified: true})
	service := NewService(verifier, NewMemoryStore())

	result, err := service.SignIn(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if !result.Linked {
		t.Fatal("expected new identity to be linked")
	}
	if result.User.Email != "test@example.com" {
		t.Fatalf("email = %q, want normalized email", result.User.Email)
	}
	if result.Identity.Provider != Provider || result.Identity.Subject != "apple-subject-1" || result.Identity.UserID != result.User.ID {
		t.Fatalf("unexpected identity: %+v", result.Identity)
	}
}

func TestSignInWithAppleLinksExistingEmailUser(t *testing.T) {
	store := NewMemoryStore()
	seedUser, _, err := store.CreateUserWithIdentity(context.Background(), "person@example.com", Identity{Provider: "email", Subject: "person@example.com", Email: "person@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	verifier := NewTestVerifier()
	verifier.AddToken("token-2", Claims{Subject: "apple-subject-2", Email: "person@example.com", EmailVerified: true})
	service := NewService(verifier, store)

	result, err := service.SignIn(context.Background(), "token-2")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if !result.Linked || result.User.ID != seedUser.ID || result.Identity.UserID != seedUser.ID {
		t.Fatalf("expected Apple identity linked to existing user, got %+v", result)
	}
}

func TestSignInWithAppleReusesExistingIdentity(t *testing.T) {
	verifier := NewTestVerifier()
	verifier.AddToken("token-3", Claims{Subject: "apple-subject-3", Email: "repeat@example.com", EmailVerified: true})
	service := NewService(verifier, NewMemoryStore())

	first, err := service.SignIn(context.Background(), "token-3")
	if err != nil {
		t.Fatalf("first SignIn: %v", err)
	}
	second, err := service.SignIn(context.Background(), "token-3")
	if err != nil {
		t.Fatalf("second SignIn: %v", err)
	}
	if second.Linked {
		t.Fatal("existing identity should not be reported as newly linked")
	}
	if second.User.ID != first.User.ID || second.Identity.UserID != first.User.ID {
		t.Fatalf("expected same user on replay, first=%+v second=%+v", first, second)
	}
}

func TestSignInWithAppleReusesExistingIdentityWhenAppleOmitsEmail(t *testing.T) {
	verifier := NewTestVerifier()
	verifier.AddToken("initial-token", Claims{Subject: "apple-subject-no-email-replay", Email: "replay@example.com", EmailVerified: true})
	verifier.AddToken("replay-token", Claims{Subject: "apple-subject-no-email-replay"})
	service := NewService(verifier, NewMemoryStore())

	first, err := service.SignIn(context.Background(), "initial-token")
	if err != nil {
		t.Fatalf("initial SignIn: %v", err)
	}
	replay, err := service.SignIn(context.Background(), "replay-token")
	if err != nil {
		t.Fatalf("replay SignIn: %v", err)
	}
	if replay.Linked || replay.User.ID != first.User.ID || replay.Identity.UserID != first.User.ID {
		t.Fatalf("expected existing identity replay without relinking, first=%+v replay=%+v", first, replay)
	}
}

func TestSignInWithAppleFailurePaths(t *testing.T) {
	verifier := NewTestVerifier()
	verifier.AddFailure("bad-token", ErrInvalidIdentityToken)
	verifier.AddToken("missing-subject", Claims{Email: "user@example.com", EmailVerified: true})
	verifier.AddToken("unverified-email", Claims{Subject: "apple-subject-4", Email: "user@example.com", EmailVerified: false})
	service := NewService(verifier, NewMemoryStore())

	tests := []struct {
		name    string
		token   string
		wantErr error
	}{
		{name: "invalid token", token: "bad-token", wantErr: ErrInvalidIdentityToken},
		{name: "missing subject", token: "missing-subject", wantErr: ErrMissingSubject},
		{name: "unverified email", token: "unverified-email", wantErr: ErrUnverifiedEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.SignIn(context.Background(), tt.token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SignIn error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignInWithAppleStoreFailurePaths(t *testing.T) {
	storeErr := errors.New("store failed")
	verifier := NewTestVerifier()
	verifier.AddToken("token", Claims{Subject: "apple-subject", Email: "user@example.com", EmailVerified: true})

	t.Run("find identity error propagates", func(t *testing.T) {
		service := NewService(verifier, failingStore{err: storeErr})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, storeErr) {
			t.Fatalf("SignIn error = %v, want store error", err)
		}
	})

	t.Run("existing identity without linked user is conflict", func(t *testing.T) {
		service := NewService(verifier, missingLinkedUserStore{})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, ErrEmailConflict) {
			t.Fatalf("SignIn error = %v, want email conflict", err)
		}
	})

	t.Run("find linked user error propagates", func(t *testing.T) {
		service := NewService(verifier, existingIdentityFindUserFailStore{err: storeErr})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, storeErr) {
			t.Fatalf("SignIn error = %v, want store error", err)
		}
	})

	t.Run("find user by email error propagates", func(t *testing.T) {
		service := NewService(verifier, findEmailFailStore{err: storeErr})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, storeErr) {
			t.Fatalf("SignIn error = %v, want store error", err)
		}
	})

	t.Run("create user with identity error propagates", func(t *testing.T) {
		service := NewService(verifier, createFailStore{err: storeErr})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, storeErr) {
			t.Fatalf("SignIn error = %v, want store error", err)
		}
	})

	t.Run("link identity error propagates", func(t *testing.T) {
		service := NewService(verifier, linkFailStore{err: storeErr})
		_, err := service.SignIn(context.Background(), "token")
		if !errors.Is(err, storeErr) {
			t.Fatalf("SignIn error = %v, want store error", err)
		}
	})
}

func TestLoadProductionConfigFromEnvRequiresSecretNames(t *testing.T) {
	t.Setenv(EnvTeamID, "TEAMID")
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvKeyID, "KEYID")
	t.Setenv(EnvPrivateKey, "PRIVATEKEY")

	if _, err := LoadProductionConfigFromEnv(); !errors.Is(err, ErrMissingProductionConfig) {
		t.Fatalf("LoadProductionConfigFromEnv error = %v, want missing config", err)
	}

	t.Setenv(EnvClientID, "com.example.app")
	cfg, err := LoadProductionConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadProductionConfigFromEnv: %v", err)
	}
	if cfg.TeamID == "" || cfg.ClientID == "" || cfg.KeyID == "" || cfg.PrivateKey == "" {
		t.Fatalf("expected all production config fields, got %+v", cfg)
	}
}

type failingStore struct{ err error }

func (s failingStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{}, false, s.err
}
func (s failingStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, s.err
}
func (s failingStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{}, false, s.err
}
func (s failingStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, s.err
}
func (s failingStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, s.err
}

type missingLinkedUserStore struct{}

func (missingLinkedUserStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{Provider: Provider, Subject: "apple-subject", UserID: "usr_missing", Email: "old@example.com"}, true, nil
}
func (missingLinkedUserStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (missingLinkedUserStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (missingLinkedUserStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, nil
}
func (missingLinkedUserStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, nil
}

type existingIdentityFindUserFailStore struct{ err error }

func (existingIdentityFindUserFailStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{Provider: Provider, Subject: "apple-subject", UserID: "usr_existing", Email: "user@example.com"}, true, nil
}
func (s existingIdentityFindUserFailStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, s.err
}
func (existingIdentityFindUserFailStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (existingIdentityFindUserFailStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, nil
}
func (existingIdentityFindUserFailStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, nil
}

type findEmailFailStore struct{ err error }

func (findEmailFailStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{}, false, nil
}
func (findEmailFailStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (s findEmailFailStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{}, false, s.err
}
func (findEmailFailStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, nil
}
func (findEmailFailStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, nil
}

type createFailStore struct{ err error }

func (createFailStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{}, false, nil
}
func (createFailStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (createFailStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (s createFailStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, s.err
}
func (createFailStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, nil
}

type linkFailStore struct{ err error }

func (linkFailStore) FindIdentity(context.Context, string, string) (Identity, bool, error) {
	return Identity{}, false, nil
}
func (linkFailStore) FindUserByID(context.Context, string) (User, bool, error) {
	return User{}, false, nil
}
func (linkFailStore) FindUserByEmail(context.Context, string) (User, bool, error) {
	return User{ID: "usr_existing", Email: "user@example.com"}, true, nil
}
func (s linkFailStore) CreateUserWithIdentity(context.Context, string, Identity) (User, Identity, error) {
	return User{}, Identity{}, s.err
}
func (s linkFailStore) LinkIdentity(context.Context, string, Identity) (Identity, error) {
	return Identity{}, s.err
}
