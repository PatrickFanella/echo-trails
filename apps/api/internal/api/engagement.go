package api

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

var ErrEngagementInvalid = errors.New("invalid engagement request")
var ErrEngagementNotFound = errors.New("engagement target not found")

type PlaceSubscription struct {
	PlaceID              string    `json:"place_id"`
	NotificationsEnabled bool      `json:"notifications_enabled"`
	CreatedAt            time.Time `json:"created_at"`
}

type PlaceSubscriptionPage struct {
	Subscriptions []PlaceSubscription `json:"subscriptions"`
	NextCursor    string              `json:"next_cursor,omitempty"`
}

type NotificationPreferences struct {
	DirectActivityEnabled bool      `json:"direct_activity_enabled"`
	PlaceAlertsEnabled    bool      `json:"place_alerts_enabled"`
	ModerationEnabled     bool      `json:"moderation_enabled"`
	UploadStatusEnabled   bool      `json:"upload_status_enabled"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type NotificationPreferencesPatch struct {
	DirectActivityEnabled *bool `json:"direct_activity_enabled,omitempty"`
	PlaceAlertsEnabled    *bool `json:"place_alerts_enabled,omitempty"`
	ModerationEnabled     *bool `json:"moderation_enabled,omitempty"`
	UploadStatusEnabled   *bool `json:"upload_status_enabled,omitempty"`
}

type DeviceRegistrationRequest struct {
	Token       string `json:"token"`
	Environment string `json:"environment"`
	AppVersion  string `json:"app_version,omitempty"`
	Locale      string `json:"locale,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
}

type DeviceRegistration struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	AppVersion  string    `json:"app_version,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type EngagementRepository interface {
	PutSubscription(context.Context, string, string, bool, time.Time) (PlaceSubscription, error)
	DeleteSubscription(context.Context, string, string) error
	ListSubscriptions(context.Context, string) ([]PlaceSubscription, error)
	ListSubscriptionPage(context.Context, string, string, int) (PlaceSubscriptionPage, error)
	GetNotificationPreferences(context.Context, string, time.Time) (NotificationPreferences, error)
	PatchNotificationPreferences(context.Context, string, NotificationPreferencesPatch, time.Time) (NotificationPreferences, error)
	RegisterDevice(context.Context, string, DeviceRegistrationRequest, time.Time) (DeviceRegistration, error)
	DeleteDevice(context.Context, string, string) error
	PutMute(context.Context, string, string, string, *time.Time, time.Time) error
	DeleteMute(context.Context, string, string, string) error
	MarkNotificationsRead(context.Context, string, []string, bool, time.Time) error
}

type MemoryEngagementRepository struct {
	mu            sync.Mutex
	subscriptions map[string]map[string]PlaceSubscription
	preferences   map[string]NotificationPreferences
	devices       map[string]map[string]DeviceRegistration
	mutes         map[string]map[string]bool
}

func NewMemoryEngagementRepository() *MemoryEngagementRepository {
	return &MemoryEngagementRepository{subscriptions: map[string]map[string]PlaceSubscription{}, preferences: map[string]NotificationPreferences{}, devices: map[string]map[string]DeviceRegistration{}, mutes: map[string]map[string]bool{}}
}

func (m *MemoryEngagementRepository) PutSubscription(_ context.Context, userID, placeID string, enabled bool, now time.Time) (PlaceSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subscriptions[userID] == nil {
		m.subscriptions[userID] = map[string]PlaceSubscription{}
	}
	subscription := PlaceSubscription{PlaceID: placeID, NotificationsEnabled: enabled, CreatedAt: now.UTC()}
	m.subscriptions[userID][placeID] = subscription
	return subscription, nil
}
func (m *MemoryEngagementRepository) DeleteSubscription(_ context.Context, userID, placeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscriptions[userID], placeID)
	return nil
}
func (m *MemoryEngagementRepository) ListSubscriptions(_ context.Context, userID string) ([]PlaceSubscription, error) {
	page, err := m.ListSubscriptionPage(context.Background(), userID, "", 50)
	return page.Subscriptions, err
}
func (m *MemoryEngagementRepository) ListSubscriptionPage(_ context.Context, userID, cursorValue string, limit int) (PlaceSubscriptionPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var cursor *pagination.Cursor
	if cursorValue != "" {
		decoded, err := pagination.Decode(cursorValue)
		if err != nil {
			return PlaceSubscriptionPage{}, err
		}
		cursor = &decoded
	}
	result := []PlaceSubscription{}
	for _, subscription := range m.subscriptions[userID] {
		result = append(result, subscription)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].PlaceID > result[j].PlaceID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	pageItems := make([]PlaceSubscription, 0, limit+1)
	for _, subscription := range result {
		if cursor != nil && (subscription.CreatedAt.After(cursor.After) || (subscription.CreatedAt.Equal(cursor.After) && subscription.PlaceID >= cursor.ID.String())) {
			continue
		}
		pageItems = append(pageItems, subscription)
		if len(pageItems) == limit+1 {
			break
		}
	}
	page := PlaceSubscriptionPage{Subscriptions: pageItems}
	if len(pageItems) > limit {
		page.Subscriptions = pageItems[:limit]
		last := page.Subscriptions[len(page.Subscriptions)-1]
		id, err := uuidv7.Parse(last.PlaceID)
		if err != nil {
			return PlaceSubscriptionPage{}, err
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
		if err != nil {
			return PlaceSubscriptionPage{}, err
		}
	}
	return page, nil
}
func (m *MemoryEngagementRepository) PatchNotificationPreferences(_ context.Context, userID string, patch NotificationPreferencesPatch, now time.Time) (NotificationPreferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	preferences := m.preferences[userID]
	if preferences.UpdatedAt.IsZero() {
		preferences = NotificationPreferences{DirectActivityEnabled: true, PlaceAlertsEnabled: true, ModerationEnabled: true, UploadStatusEnabled: true}
	}
	applyPreferencesPatch(&preferences, patch)
	preferences.UpdatedAt = now.UTC()
	m.preferences[userID] = preferences
	return preferences, nil
}
func (m *MemoryEngagementRepository) GetNotificationPreferences(_ context.Context, userID string, now time.Time) (NotificationPreferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	preferences, found := m.preferences[userID]
	if !found {
		preferences = NotificationPreferences{DirectActivityEnabled: true, PlaceAlertsEnabled: true, ModerationEnabled: true, UploadStatusEnabled: true, UpdatedAt: now.UTC()}
	}
	return preferences, nil
}
func (m *MemoryEngagementRepository) RegisterDevice(_ context.Context, userID string, request DeviceRegistrationRequest, now time.Time) (DeviceRegistration, error) {
	if err := validateDeviceRegistration(request); err != nil {
		return DeviceRegistration{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.devices[userID] == nil {
		m.devices[userID] = map[string]DeviceRegistration{}
	}
	id := newOpaqueID("device")
	device := DeviceRegistration{ID: id, Environment: request.Environment, AppVersion: request.AppVersion, CreatedAt: now.UTC()}
	m.devices[userID][id] = device
	return device, nil
}
func (m *MemoryEngagementRepository) DeleteDevice(_ context.Context, userID, deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.devices[userID], deviceID)
	return nil
}
func (m *MemoryEngagementRepository) PutMute(_ context.Context, userID, targetType, targetID string, _ *time.Time, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mutes[userID] == nil {
		m.mutes[userID] = map[string]bool{}
	}
	m.mutes[userID][targetType+":"+targetID] = true
	return nil
}
func (m *MemoryEngagementRepository) DeleteMute(_ context.Context, userID, targetType, targetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mutes[userID], targetType+":"+targetID)
	return nil
}
func (m *MemoryEngagementRepository) MarkNotificationsRead(context.Context, string, []string, bool, time.Time) error {
	return nil
}

func applyPreferencesPatch(preferences *NotificationPreferences, patch NotificationPreferencesPatch) {
	if patch.DirectActivityEnabled != nil {
		preferences.DirectActivityEnabled = *patch.DirectActivityEnabled
	}
	if patch.PlaceAlertsEnabled != nil {
		preferences.PlaceAlertsEnabled = *patch.PlaceAlertsEnabled
	}
	if patch.ModerationEnabled != nil {
		preferences.ModerationEnabled = *patch.ModerationEnabled
	}
	if patch.UploadStatusEnabled != nil {
		preferences.UploadStatusEnabled = *patch.UploadStatusEnabled
	}
}

func validateDeviceRegistration(request DeviceRegistrationRequest) error {
	token := strings.TrimSpace(request.Token)
	if len(token) != 64 || (request.Environment != "sandbox" && request.Environment != "production") {
		return ErrEngagementInvalid
	}
	for _, character := range token {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return ErrEngagementInvalid
		}
	}
	return nil
}
