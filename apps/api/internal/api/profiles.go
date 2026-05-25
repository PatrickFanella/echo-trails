package api

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

const handleChangeCooldown = 30 * 24 * time.Hour

var (
	ErrInvalidHandle       = errors.New("invalid handle")
	ErrHandleTaken         = errors.New("handle is already taken")
	ErrHandleChangeTooSoon = errors.New("handle change cooldown is active")
	ErrProfileNotFound     = errors.New("profile not found")

	handlePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{2,29}$`)
	reservedHandles = map[string]struct{}{
		"admin": {}, "api": {}, "auth": {}, "echo": {}, "echotrails": {},
		"me": {}, "moderator": {}, "root": {}, "support": {}, "users": {},
	}
)

type Profile struct {
	UserID          string
	Email           string
	Handle          string
	DisplayName     string
	Bio             string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	HandleChangedAt *time.Time
}

type MeProfile struct {
	UserID                  string     `json:"user_id"`
	Email                   string     `json:"email"`
	Handle                  string     `json:"handle"`
	DisplayName             string     `json:"display_name"`
	Bio                     string     `json:"bio,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	HandleChangeAvailableAt *time.Time `json:"handle_change_available_at,omitempty"`
}

type PublicProfile struct {
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type UpsertProfileRequest struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio,omitempty"`
}

type ProfileStore interface {
	GetProfileByUserID(ctx context.Context, userID string) (Profile, bool, error)
	GetProfileByHandle(ctx context.Context, handle string) (Profile, bool, error)
	UpsertProfile(ctx context.Context, userID, email string, req UpsertProfileRequest, now time.Time) (Profile, error)
}

type MemoryProfileStore struct {
	mu             sync.Mutex
	byUserID       map[string]Profile
	userIDByHandle map[string]string
}

func NewMemoryProfileStore() *MemoryProfileStore {
	return &MemoryProfileStore{byUserID: map[string]Profile{}, userIDByHandle: map[string]string{}}
}

func (s *MemoryProfileStore) GetProfileByUserID(_ context.Context, userID string) (Profile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.byUserID[userID]
	return profile, ok, nil
}

func (s *MemoryProfileStore) GetProfileByHandle(_ context.Context, handle string) (Profile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userIDByHandle[NormalizeHandle(handle)]
	if !ok {
		return Profile{}, false, nil
	}
	profile, ok := s.byUserID[userID]
	return profile, ok, nil
}

func (s *MemoryProfileStore) UpsertProfile(_ context.Context, userID, email string, req UpsertProfileRequest, now time.Time) (Profile, error) {
	handle, err := ValidateHandle(req.Handle)
	if err != nil {
		return Profile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingUserID, ok := s.userIDByHandle[handle]; ok && existingUserID != userID {
		return Profile{}, ErrHandleTaken
	}

	displayName := normalizeDisplayName(req.DisplayName, handle)
	bio := normalizeBio(req.Bio)
	profile, exists := s.byUserID[userID]
	if !exists {
		profile = Profile{UserID: userID, Email: strings.TrimSpace(email), CreatedAt: now}
	}

	if profile.Handle != "" && profile.Handle != handle {
		if profile.HandleChangedAt != nil && now.Before(profile.HandleChangedAt.Add(handleChangeCooldown)) {
			return Profile{}, ErrHandleChangeTooSoon
		}
		delete(s.userIDByHandle, profile.Handle)
		changedAt := now
		profile.HandleChangedAt = &changedAt
	}
	if profile.Handle == "" {
		changedAt := now
		profile.HandleChangedAt = &changedAt
	}

	profile.Email = strings.TrimSpace(email)
	profile.Handle = handle
	profile.DisplayName = displayName
	profile.Bio = bio
	profile.UpdatedAt = now
	s.byUserID[userID] = profile
	s.userIDByHandle[handle] = userID
	return profile, nil
}

func NormalizeHandle(input string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(input)), "@")
}

func ValidateHandle(input string) (string, error) {
	handle := NormalizeHandle(input)
	if !handlePattern.MatchString(handle) {
		return "", ErrInvalidHandle
	}
	if _, reserved := reservedHandles[handle]; reserved {
		return "", ErrInvalidHandle
	}
	return handle, nil
}

func toMeProfile(profile Profile, now time.Time) MeProfile {
	var availableAt *time.Time
	if profile.HandleChangedAt != nil {
		next := profile.HandleChangedAt.Add(handleChangeCooldown)
		if now.Before(next) {
			availableAt = &next
		}
	}
	return MeProfile{
		UserID: profile.UserID, Email: profile.Email, Handle: profile.Handle,
		DisplayName: profile.DisplayName, Bio: profile.Bio, CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt, HandleChangeAvailableAt: availableAt,
	}
}

func toPublicProfile(profile Profile) PublicProfile {
	return PublicProfile{Handle: profile.Handle, DisplayName: profile.DisplayName, Bio: profile.Bio, CreatedAt: profile.CreatedAt}
}

func normalizeDisplayName(input, fallbackHandle string) string {
	name := strings.Join(strings.Fields(input), " ")
	if name == "" {
		return fallbackHandle
	}
	if len(name) > 80 {
		return name[:80]
	}
	return name
}

func normalizeBio(input string) string {
	bio := strings.TrimSpace(input)
	if len(bio) > 160 {
		return bio[:160]
	}
	return bio
}
