package apple

import (
	"context"
	"strconv"
	"sync"
)

type TestVerifier struct {
	mu     sync.Mutex
	claims map[string]Claims
	errs   map[string]error
}

func NewTestVerifier() *TestVerifier {
	return &TestVerifier{claims: map[string]Claims{}, errs: map[string]error{}}
}

func (v *TestVerifier) AddToken(token string, claims Claims) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.claims[token] = claims
}

func (v *TestVerifier) AddFailure(token string, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.errs[token] = err
}

func (v *TestVerifier) VerifyIdentityToken(_ context.Context, token string) (Claims, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err, ok := v.errs[token]; ok {
		return Claims{}, err
	}
	claims, ok := v.claims[token]
	if !ok {
		return Claims{}, ErrInvalidIdentityToken
	}
	return claims, nil
}

type MemoryStore struct {
	mu         sync.Mutex
	nextID     int
	users      map[string]User
	usersEmail map[string]string
	identities map[string]Identity
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: map[string]User{}, usersEmail: map[string]string{}, identities: map[string]Identity{}}
}

func (s *MemoryStore) FindIdentity(_ context.Context, provider, subject string) (Identity, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, ok := s.identities[identityKey(provider, subject)]
	return identity, ok, nil
}

func (s *MemoryStore) FindUserByID(_ context.Context, userID string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	return user, ok, nil
}

func (s *MemoryStore) FindUserByEmail(_ context.Context, email string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.usersEmail[normalizeEmail(email)]
	if !ok {
		return User{}, false, nil
	}
	return s.users[userID], true, nil
}

func (s *MemoryStore) CreateUserWithIdentity(_ context.Context, email string, identity Identity) (User, Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizedEmail := normalizeEmail(email)
	if _, ok := s.usersEmail[normalizedEmail]; ok {
		return User{}, Identity{}, ErrEmailConflict
	}
	if _, ok := s.identities[identityKey(identity.Provider, identity.Subject)]; ok {
		return User{}, Identity{}, ErrEmailConflict
	}
	s.nextID++
	user := User{ID: "usr_test_" + strconv.Itoa(s.nextID), Email: normalizedEmail}
	identity.UserID = user.ID
	identity.Email = normalizedEmail
	s.users[user.ID] = user
	s.usersEmail[normalizedEmail] = user.ID
	s.identities[identityKey(identity.Provider, identity.Subject)] = identity
	return user, identity, nil
}

func (s *MemoryStore) LinkIdentity(_ context.Context, userID string, identity Identity) (Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return Identity{}, ErrEmailConflict
	}
	if existing, ok := s.identities[identityKey(identity.Provider, identity.Subject)]; ok && existing.UserID != userID {
		return Identity{}, ErrEmailConflict
	}
	identity.UserID = userID
	identity.Email = normalizeEmail(identity.Email)
	s.identities[identityKey(identity.Provider, identity.Subject)] = identity
	return identity, nil
}

func identityKey(provider, subject string) string { return provider + ":" + subject }
