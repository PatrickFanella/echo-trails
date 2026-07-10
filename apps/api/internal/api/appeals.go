package api

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrAppealInvalid = errors.New("invalid appeal")

type AppealRequest struct {
	ModerationActionID string `json:"moderation_action_id"`
	Body               string `json:"body"`
}

type Appeal struct {
	ID                 string    `json:"id"`
	ModerationActionID string    `json:"moderation_action_id"`
	Body               string    `json:"body"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}

type AppealPage struct {
	Appeals    []Appeal `json:"appeals"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type AppealRepository interface {
	CreateAppeal(context.Context, string, AppealRequest, string, time.Time) (Appeal, error)
	ListAppeals(context.Context, string, string, int) (AppealPage, error)
	UpdateAppeal(context.Context, string, string, string, string, time.Time) (Appeal, error)
}

type MemoryAppealRepository struct {
	mu    sync.Mutex
	items map[string][]Appeal
}

func NewMemoryAppealRepository() *MemoryAppealRepository {
	return &MemoryAppealRepository{items: map[string][]Appeal{}}
}
func (m *MemoryAppealRepository) CreateAppeal(_ context.Context, userID string, request AppealRequest, _ string, now time.Time) (Appeal, error) {
	if request.ModerationActionID == "" || request.Body == "" {
		return Appeal{}, ErrAppealInvalid
	}
	appeal := Appeal{ID: newOpaqueID("appeal"), ModerationActionID: request.ModerationActionID, Body: request.Body, Status: "open", CreatedAt: now.UTC()}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[userID] = append(m.items[userID], appeal)
	return appeal, nil
}
func (m *MemoryAppealRepository) ListAppeals(_ context.Context, userID, _ string, _ int) (AppealPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return AppealPage{Appeals: append([]Appeal(nil), m.items[userID]...)}, nil
}
func (m *MemoryAppealRepository) UpdateAppeal(_ context.Context, userID, appealID, body, _ string, _ time.Time) (Appeal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.items[userID] {
		if m.items[userID][index].ID == appealID && m.items[userID][index].Status == "open" {
			m.items[userID][index].Body = body
			return m.items[userID][index], nil
		}
	}
	return Appeal{}, ErrSocialNotFound
}
