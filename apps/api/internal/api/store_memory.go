package api

import (
	"context"
	"errors"
	"sync"
	"time"
)

type MemoryStore struct {
	mu                    sync.Mutex
	users                 map[string]User
	usersByEmail          map[string]string
	authRequests          map[string]AuthRequest
	sessions              map[string]Session
	sessionsByRefreshHash map[string]string
	refreshReplays        map[string]Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: map[string]User{}, usersByEmail: map[string]string{}, authRequests: map[string]AuthRequest{}, sessions: map[string]Session{}, sessionsByRefreshHash: map[string]string{}, refreshReplays: map[string]Session{}}
}
func (m *MemoryStore) RotateSessionRefreshTokenIdempotent(ctx context.Context, currentHash, key, newRefreshHash, newAccessHash string, refreshExpiry, accessExpiry, now time.Time) (Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	replayKey := currentHash + ":" + key
	if session, ok := m.refreshReplays[replayKey]; ok {
		return session, true, nil
	}
	sessionID, ok := m.sessionsByRefreshHash[currentHash]
	if !ok {
		return Session{}, false, nil
	}
	session := m.sessions[sessionID]
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
		return Session{}, false, nil
	}
	delete(m.sessionsByRefreshHash, currentHash)
	session.RefreshTokenHash = newRefreshHash
	session.AccessTokenHash = newAccessHash
	session.ExpiresAt = refreshExpiry
	session.AccessExpiresAt = accessExpiry
	m.sessions[sessionID] = session
	m.sessionsByRefreshHash[newRefreshHash] = sessionID
	m.refreshReplays[replayKey] = session
	return session, true, nil
}

func (m *MemoryStore) CreateAuthRequest(_ context.Context, req AuthRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authRequests[req.ID] = req
	return nil
}
func (m *MemoryStore) GetAuthRequest(_ context.Context, id string) (AuthRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.authRequests[id]
	return v, ok
}
func (m *MemoryStore) ConsumeAuthRequest(_ context.Context, id string, submittedHash string, usedAt time.Time) (AuthRequest, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.authRequests[id]
	if !ok {
		return AuthRequest{}, false, nil
	}
	if v.UsedAt != nil || !usedAt.Before(v.ExpiresAt) || v.Attempts >= 5 {
		return AuthRequest{}, false, nil
	}
	if !constantTimeEqualHex(v.CodeHash, submittedHash) {
		v.Attempts++
		m.authRequests[id] = v
		return AuthRequest{}, false, nil
	}
	v.UsedAt = &usedAt
	m.authRequests[id] = v
	return v, true, nil
}
func (m *MemoryStore) FindOrCreateUserByEmail(_ context.Context, email string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.usersByEmail[email]; ok {
		return m.users[id], nil
	}
	id, err := newID("usr_")
	if err != nil {
		return User{}, err
	}
	u := User{ID: id, Email: email, Role: "user", Status: "active"}
	m.users[id] = u
	m.usersByEmail[email] = id
	return u, nil
}
func (m *MemoryStore) CreateSession(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	m.sessionsByRefreshHash[s.RefreshTokenHash] = s.ID
	return nil
}
func (m *MemoryStore) AuthenticateAccessToken(_ context.Context, hash string, now time.Time) (User, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, session := range m.sessions {
		if session.AccessTokenHash == hash && session.RevokedAt == nil && now.Before(session.AccessExpiresAt) {
			user, ok := m.users[session.UserID]
			return user, ok, nil
		}
	}
	return User{}, false, nil
}
func (m *MemoryStore) RotateSessionRefreshToken(_ context.Context, currentHash, newRefreshHash, newAccessHash string, newRefreshExpiresAt, newAccessExpiresAt, now time.Time) (Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessionID, ok := m.sessionsByRefreshHash[currentHash]
	if !ok {
		return Session{}, false, nil
	}
	s, ok := m.sessions[sessionID]
	if !ok {
		return Session{}, false, errors.New("not found")
	}
	if s.RevokedAt != nil || !now.Before(s.ExpiresAt) {
		return Session{}, false, nil
	}
	delete(m.sessionsByRefreshHash, currentHash)
	s.RefreshTokenHash = newRefreshHash
	s.AccessTokenHash = newAccessHash
	s.ExpiresAt = newRefreshExpiresAt
	s.AccessExpiresAt = newAccessExpiresAt
	m.sessions[sessionID] = s
	m.sessionsByRefreshHash[newRefreshHash] = sessionID
	return s, true, nil
}
func (m *MemoryStore) RevokeSessionByRefreshTokenHash(_ context.Context, hash string, revokedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.sessionsByRefreshHash[hash]
	if !ok {
		return false, nil
	}
	s := m.sessions[id]
	s.RevokedAt = &revokedAt
	m.sessions[id] = s
	delete(m.sessionsByRefreshHash, hash)
	return true, nil
}
