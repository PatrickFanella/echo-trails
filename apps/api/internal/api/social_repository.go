package api

import (
	"context"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

type SocialRepository interface {
	PutReaction(context.Context, string, ReactionRequest, time.Time) (ReactionSummary, error)
	ReactionSummary(context.Context, string) (ReactionSummary, error)
	EchoKnown(context.Context, string) (bool, error)
	RegisterEcho(context.Context, string, string) error
	AddReply(context.Context, string, ReplyRequest, time.Time) (Reply, error)
	ListReplies(context.Context, string, string) ([]Reply, error)
	ListReplyPage(context.Context, string, string, string, int) (ReplyPage, error)
	UpdateReply(context.Context, string, string, string, time.Time) (Reply, error)
	DeleteReply(context.Context, string, string, time.Time) error
	ReplyHistory(context.Context, string, string) ([]ReplyRevision, error)
	AddStitch(context.Context, StitchRequest, time.Time) (Stitch, error)
	ValidateStitchSource(context.Context, string, string, string, bool) error
	LinkStitch(context.Context, string, string, string, string, time.Time) error
	AddReport(context.Context, ReportRequest, time.Time) (Report, error)
	Hide(context.Context, VisibilityRequest) error
	Block(context.Context, VisibilityRequest) error
	Notifications(context.Context, string) ([]Notification, error)
	NotificationPage(context.Context, string, string, int) (NotificationPage, error)
	RecordDetailView(context.Context, string, string, time.Time) error
}

type MemorySocialRepository struct{ store *SocialStore }

func (m *MemorySocialRepository) RecordDetailView(_ context.Context, echoID, _ string, _ time.Time) error {
	if !m.store.EchoKnown(echoID) {
		return ErrSocialNotFound
	}
	return nil
}

func NewMemorySocialRepository(store *SocialStore) *MemorySocialRepository {
	if store == nil {
		store = NewSocialStore()
	}
	return &MemorySocialRepository{store: store}
}

func (m *MemorySocialRepository) PutReaction(_ context.Context, echoID string, request ReactionRequest, now time.Time) (ReactionSummary, error) {
	return m.store.PutReaction(echoID, request, now)
}
func (m *MemorySocialRepository) ReactionSummary(_ context.Context, echoID string) (ReactionSummary, error) {
	return m.store.ReactionSummary(echoID), nil
}
func (m *MemorySocialRepository) EchoKnown(_ context.Context, echoID string) (bool, error) {
	return m.store.EchoKnown(echoID), nil
}
func (m *MemorySocialRepository) RegisterEcho(_ context.Context, echoID, authorID string) error {
	m.store.RegisterEcho(echoID, authorID)
	return nil
}
func (m *MemorySocialRepository) AddReply(_ context.Context, echoID string, request ReplyRequest, now time.Time) (Reply, error) {
	return m.store.AddReply(echoID, request, now)
}
func (m *MemorySocialRepository) ListReplies(_ context.Context, echoID, viewerID string) ([]Reply, error) {
	return m.store.ListReplies(echoID, viewerID), nil
}
func (m *MemorySocialRepository) ListReplyPage(_ context.Context, echoID, viewerID, cursorValue string, limit int) (ReplyPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var cursor *pagination.Cursor
	if cursorValue != "" {
		decoded, err := pagination.Decode(cursorValue)
		if err != nil {
			return ReplyPage{}, err
		}
		cursor = &decoded
	}
	replies := m.store.ListReplies(echoID, viewerID)
	result := make([]Reply, 0, limit+1)
	for _, reply := range replies {
		if cursor != nil && (reply.PostedAt.Before(cursor.After) || (reply.PostedAt.Equal(cursor.After) && reply.ID <= cursor.ID.String())) {
			continue
		}
		result = append(result, reply)
		if len(result) == limit+1 {
			break
		}
	}
	page := ReplyPage{Replies: result}
	if len(result) > limit {
		page.Replies = result[:limit]
		last := page.Replies[len(page.Replies)-1]
		id, err := uuidv7.Parse(last.ID)
		if err != nil {
			return ReplyPage{}, err
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.PostedAt, ID: id})
		if err != nil {
			return ReplyPage{}, err
		}
	}
	return page, nil
}
func (m *MemorySocialRepository) UpdateReply(_ context.Context, replyID, userID, body string, now time.Time) (Reply, error) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 500 {
		return Reply{}, ErrSocialInvalid
	}
	m.store.mu.Lock()
	defer m.store.mu.Unlock()
	for echoID, replies := range m.store.replies {
		for index := range replies {
			if replies[index].ID != replyID {
				continue
			}
			if replies[index].UserID != userID {
				return Reply{}, ErrSocialForbidden
			}
			replies[index].Body = body
			replies[index].Edited = true
			m.store.replies[echoID] = replies
			revisions := m.store.replyRevisions[replyID]
			m.store.replyRevisions[replyID] = append(revisions, ReplyRevision{RevisionNumber: len(revisions) + 1, Body: body, CreatedAt: now.UTC()})
			m.store.syncReplyMentionsLocked(replies[index], body, nil, now)
			return replies[index], nil
		}
	}
	return Reply{}, ErrSocialNotFound
}
func (m *MemorySocialRepository) DeleteReply(_ context.Context, replyID, userID string, _ time.Time) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()
	for echoID, replies := range m.store.replies {
		for index := range replies {
			if replies[index].ID != replyID {
				continue
			}
			if replies[index].UserID != userID {
				return ErrSocialForbidden
			}
			replies[index].Body = "Reply deleted"
			replies[index].Hidden = true
			m.store.replies[echoID] = replies
			delete(m.store.replyMentions, replyID)
			return nil
		}
	}
	return ErrSocialNotFound
}
func (m *MemorySocialRepository) ReplyHistory(_ context.Context, replyID, userID string) ([]ReplyRevision, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	for _, replies := range m.store.replies {
		for _, reply := range replies {
			if reply.ID == replyID {
				if reply.UserID != userID {
					return nil, ErrSocialForbidden
				}
				return append([]ReplyRevision(nil), m.store.replyRevisions[replyID]...), nil
			}
		}
	}
	return nil, ErrSocialNotFound
}
func (m *MemorySocialRepository) AddStitch(_ context.Context, request StitchRequest, now time.Time) (Stitch, error) {
	return m.store.AddStitch(request, now)
}
func (m *MemorySocialRepository) ValidateStitchSource(_ context.Context, userID, sourceEchoID, quotedReplyID string, acknowledged bool) error {
	if !m.store.EchoKnown(sourceEchoID) {
		return ErrSocialNotFound
	}
	if quotedReplyID != "" && !acknowledged {
		return ErrSocialInvalid
	}
	return nil
}
func (m *MemorySocialRepository) LinkStitch(_ context.Context, sourceEchoID, targetEchoID, userID, quotedReplyID string, now time.Time) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()
	authorID, ok := m.store.echoAuthors[sourceEchoID]
	if !ok {
		return ErrSocialNotFound
	}
	m.store.echoAuthors[targetEchoID] = userID
	m.store.stitches[sourceEchoID] = append(m.store.stitches[sourceEchoID], Stitch{ID: targetEchoID, EchoID: targetEchoID, UserID: userID, SourceEchoID: sourceEchoID, QuotedReplyID: quotedReplyID, CreatedAt: now.UTC(), Status: EchoStatusDraft})
	m.store.notifyLocked(Notification{UserID: authorID, Kind: NotificationStitch, ActorID: userID, TargetID: sourceEchoID, Preview: "Someone stitched your Echo.", CreatedAt: now.UTC()})
	return nil
}
func (m *MemorySocialRepository) AddReport(_ context.Context, request ReportRequest, now time.Time) (Report, error) {
	return m.store.AddReport(request, now)
}
func (m *MemorySocialRepository) Hide(_ context.Context, request VisibilityRequest) error {
	return m.store.Hide(request)
}
func (m *MemorySocialRepository) Block(_ context.Context, request VisibilityRequest) error {
	return m.store.Block(request)
}
func (m *MemorySocialRepository) Notifications(_ context.Context, userID string) ([]Notification, error) {
	return m.store.Notifications(userID), nil
}
func (m *MemorySocialRepository) NotificationPage(_ context.Context, userID, _ string, limit int) (NotificationPage, error) {
	notifications := m.store.Notifications(userID)
	if limit > 0 && len(notifications) > limit {
		notifications = notifications[:limit]
	}
	return NotificationPage{Notifications: notifications}, nil
}
