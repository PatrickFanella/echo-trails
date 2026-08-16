package api

import (
	"crypto/rand"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

const (
	ReactionKindHeart     = "heart"
	ReactionKindFire      = "fire"
	ReactionKindSad       = "sad"
	ReactionKindLaugh     = "laugh"
	ReactionKindClap      = "clap"
	ReactionKindMindBlown = "mind_blown"

	ReportTargetEcho  = "echo"
	ReportTargetReply = "reply"
	ReportTargetUser  = "user"

	NotificationReaction = "reaction"
	NotificationReply    = "reply"
	NotificationStitch   = "stitch"
	NotificationMention  = "mention"
)

var replyMentionPattern = regexp.MustCompile(`(?i)(?:^|[^\pL\pN_])@([a-z][a-z0-9_]{2,29})(?:\b|$)`)

var ErrSocialInvalid = errors.New("invalid social request")
var ErrSocialNotFound = errors.New("social target not found")
var ErrSocialHidden = errors.New("social target hidden")
var ErrSocialForbidden = errors.New("social action forbidden")

type SocialStore struct {
	mu             sync.RWMutex
	echoAuthors    map[string]string
	reactions      map[string]Reaction
	replies        map[string][]Reply
	replyRevisions map[string][]ReplyRevision
	stitches       map[string][]Stitch
	reports        []Report
	hides          map[string]map[string]bool
	blocks         map[string]map[string]bool
	notifications  []Notification
	userByHandle   map[string]string
	handleByUser   map[string]string
	replyMentions  map[string]map[string]bool
}

func NewSocialStore() *SocialStore {
	return NewSocialStoreWithEchoAuthors(map[string]string{"echo-active-1": "user-seed-1"})
}

func NewSocialStoreWithEchoAuthors(echoAuthors map[string]string) *SocialStore {
	authors := map[string]string{}
	for echoID, authorID := range echoAuthors {
		if validSocialID(echoID) && validSocialID(authorID) {
			authors[echoID] = authorID
		}
	}
	return &SocialStore{echoAuthors: authors, reactions: map[string]Reaction{}, replies: map[string][]Reply{}, replyRevisions: map[string][]ReplyRevision{}, stitches: map[string][]Stitch{}, hides: map[string]map[string]bool{}, blocks: map[string]map[string]bool{}, userByHandle: map[string]string{}, handleByUser: map[string]string{}, replyMentions: map[string]map[string]bool{}}
}

// RegisterParticipantHandle supplies the local-memory adapter with the same
// handle directory Postgres gets from user_profiles. Production never trusts a
// handle from a Reply request itself.
func (s *SocialStore) RegisterParticipantHandle(userID, handle string) {
	handle, err := ValidateHandle(handle)
	if err != nil || !validSocialID(userID) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userByHandle[handle] = userID
	s.handleByUser[userID] = handle
}

func parseReplyMentionHandles(body string) []string {
	matches := replyMentionPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		handle := NormalizeHandle(match[1])
		if _, exists := seen[handle]; exists {
			continue
		}
		seen[handle] = struct{}{}
		result = append(result, handle)
	}
	return result
}

type ReactionRequest struct {
	UserID string `json:"-"`
	Kind   string `json:"kind"`
}

type Reaction struct {
	EchoID    string    `json:"echo_id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

type ReactionSummary struct {
	EchoID string         `json:"echo_id"`
	Totals map[string]int `json:"totals"`
}

type ReplyRequest struct {
	UserID  string `json:"-"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

type Reply struct {
	ID           string    `json:"id"`
	EchoID       string    `json:"echo_id"`
	UserID       string    `json:"user_id"`
	AuthorHandle string    `json:"author_handle"`
	Body         string    `json:"body"`
	ReplyTo      string    `json:"reply_to,omitempty"`
	Edited       bool      `json:"edited"`
	Hidden       bool      `json:"hidden,omitempty"`
	PostedAt     time.Time `json:"posted_at"`
}

type ReplyRevision struct {
	RevisionNumber int       `json:"revision_number"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReplyPage struct {
	Replies    []Reply `json:"replies"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type StitchRequest struct {
	UserID                   string          `json:"-"`
	SourceEchoID             string          `json:"source_echo_id"`
	PlaceID                  string          `json:"place_id,omitempty"`
	ContentMode              EchoContentMode `json:"content_mode,omitempty"`
	Title                    string          `json:"title"`
	Body                     string          `json:"body,omitempty"`
	CaptionText              string          `json:"caption_text,omitempty"`
	CreationToken            string          `json:"creation_token,omitempty"`
	QuotedReplyID            string          `json:"quoted_reply_id,omitempty"`
	QuoteWarningAcknowledged bool            `json:"quote_warning_acknowledged,omitempty"`
}

type Stitch struct {
	ID            string    `json:"id"`
	EchoID        string    `json:"echo_id"`
	UserID        string    `json:"user_id"`
	SourceEchoID  string    `json:"source_echo_id"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status,omitempty"`
	QuotedReplyID string    `json:"quoted_reply_id,omitempty"`
}

type ReportRequest struct {
	ReporterID   string `json:"-"`
	TargetType   string `json:"target_type"`
	TargetID     string `json:"target_id,omitempty"`
	TargetHandle string `json:"target_handle,omitempty"`
	Category     string `json:"category"`
	Details      string `json:"details,omitempty"`
	Reason       string `json:"reason,omitempty"` // accepted for older beta clients
}

type Report struct {
	ID         string    `json:"id"`
	ReporterID string    `json:"reporter_id"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Category   string    `json:"category"`
	Details    string    `json:"details,omitempty"`
	Reason     string    `json:"reason"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type VisibilityRequest struct {
	UserID   string `json:"-"`
	TargetID string `json:"target_id"`
}

type BlockRequest struct {
	TargetID     string `json:"target_id,omitempty"`
	TargetHandle string `json:"target_handle,omitempty"`
}

type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Kind      string     `json:"kind"`
	ActorID   string     `json:"actor_id"`
	TargetID  string     `json:"target_id"`
	Preview   string     `json:"preview"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

type NotificationPage struct {
	Notifications []Notification `json:"notifications"`
	NextCursor    string         `json:"next_cursor,omitempty"`
}

func (s *SocialStore) PutReaction(echoID string, req ReactionRequest, now time.Time) (ReactionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validSocialID(echoID) || !validSocialID(req.UserID) || !validReactionKind(req.Kind) || s.hidden(req.UserID, echoID) {
		return ReactionSummary{}, ErrSocialInvalid
	}
	authorID, ok := s.echoAuthors[echoID]
	if !ok {
		return ReactionSummary{}, ErrSocialNotFound
	}
	if s.blocked(authorID, req.UserID) {
		return ReactionSummary{}, ErrSocialHidden
	}
	s.reactions[echoID+":"+req.UserID] = Reaction{EchoID: echoID, UserID: req.UserID, Kind: req.Kind, CreatedAt: now.UTC()}
	s.notifyLocked(Notification{UserID: authorID, Kind: NotificationReaction, ActorID: req.UserID, TargetID: echoID, Preview: "Someone reacted to your Echo.", CreatedAt: now.UTC()})
	return s.reactionSummaryLocked(echoID), nil
}

func (s *SocialStore) ReactionSummary(echoID string) ReactionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !validSocialID(echoID) || s.echoAuthors[echoID] == "" {
		return ReactionSummary{EchoID: echoID, Totals: emptyReactionTotals()}
	}
	return s.reactionSummaryLocked(echoID)
}

func (s *SocialStore) EchoKnown(echoID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return validSocialID(echoID) && s.echoAuthors[echoID] != ""
}

func (s *SocialStore) RegisterEcho(echoID, authorID string) {
	if !validSocialID(echoID) || !validSocialID(authorID) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.echoAuthors[echoID] = authorID
}

func (s *SocialStore) reactionSummaryLocked(echoID string) ReactionSummary {
	totals := emptyReactionTotals()
	for _, reaction := range s.reactions {
		if reaction.EchoID == echoID {
			totals[reaction.Kind]++
		}
	}
	return ReactionSummary{EchoID: echoID, Totals: totals}
}

func (s *SocialStore) AddReply(echoID string, req ReplyRequest, now time.Time) (Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body := strings.TrimSpace(req.Body)
	if !validSocialID(echoID) || !validSocialID(req.UserID) || body == "" || len(body) > 500 || s.hidden(req.UserID, echoID) {
		return Reply{}, ErrSocialInvalid
	}
	authorID, ok := s.echoAuthors[echoID]
	if !ok {
		return Reply{}, ErrSocialNotFound
	}
	if s.blocked(authorID, req.UserID) {
		return Reply{}, ErrSocialHidden
	}
	if req.ReplyTo != "" && !s.replyExists(echoID, req.ReplyTo) {
		return Reply{}, ErrSocialInvalid
	}
	replyID, err := uuidv7.NewAt(now, rand.Reader)
	if err != nil {
		return Reply{}, err
	}
	handle := s.handleByUser[req.UserID]
	if handle == "" {
		// Memory repositories support older unit fixtures without account setup.
		// Production always resolves this field from user_profiles.
		handle = req.UserID
	}
	reply := Reply{ID: replyID.String(), EchoID: echoID, UserID: req.UserID, AuthorHandle: handle, Body: body, ReplyTo: req.ReplyTo, PostedAt: now.UTC()}
	s.replies[echoID] = append(s.replies[echoID], reply)
	s.replyRevisions[reply.ID] = []ReplyRevision{{RevisionNumber: 1, Body: body, CreatedAt: now.UTC()}}
	s.notifyLocked(Notification{UserID: authorID, Kind: NotificationReply, ActorID: req.UserID, TargetID: echoID, Preview: "Someone replied to your Echo.", CreatedAt: now.UTC()})
	s.syncReplyMentionsLocked(reply, body, map[string]bool{authorID: true}, now)
	return reply, nil
}

func (s *SocialStore) syncReplyMentionsLocked(reply Reply, body string, alreadyNotified map[string]bool, now time.Time) {
	participants := map[string]bool{s.echoAuthors[reply.EchoID]: true}
	for _, existingReply := range s.replies[reply.EchoID] {
		participants[existingReply.UserID] = true
	}
	previous := s.replyMentions[reply.ID]
	current := map[string]bool{}
	for _, handle := range parseReplyMentionHandles(body) {
		mentionedUserID := s.userByHandle[handle]
		if mentionedUserID == "" || mentionedUserID == reply.UserID || !participants[mentionedUserID] ||
			s.blocked(mentionedUserID, reply.UserID) || s.blocked(reply.UserID, mentionedUserID) {
			continue
		}
		current[mentionedUserID] = true
		if !previous[mentionedUserID] && !alreadyNotified[mentionedUserID] {
			s.notifyLocked(Notification{UserID: mentionedUserID, Kind: NotificationMention, ActorID: reply.UserID, TargetID: reply.EchoID, Preview: "Someone mentioned you in a Reply.", CreatedAt: now.UTC()})
		}
	}
	s.replyMentions[reply.ID] = current
}

func (s *SocialStore) ListReplies(echoID, viewerID string) []Reply {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !validSocialID(echoID) || s.echoAuthors[echoID] == "" || s.hidden(viewerID, echoID) {
		return nil
	}
	replies := append([]Reply(nil), s.replies[echoID]...)
	out := replies[:0]
	for _, reply := range replies {
		if s.blocked(viewerID, reply.UserID) || s.hidden(viewerID, reply.ID) {
			continue
		}
		out = append(out, reply)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PostedAt.Before(out[j].PostedAt) })
	return out
}

func (s *SocialStore) AddStitch(req StitchRequest, now time.Time) (Stitch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validSocialID(req.UserID) || !validSocialID(req.SourceEchoID) || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" || len(req.Title) > 80 || len(req.Body) > 2000 || s.hidden(req.UserID, req.SourceEchoID) {
		return Stitch{}, ErrSocialInvalid
	}
	authorID, ok := s.echoAuthors[req.SourceEchoID]
	if !ok {
		return Stitch{}, ErrSocialNotFound
	}
	if s.blocked(authorID, req.UserID) {
		return Stitch{}, ErrSocialHidden
	}
	stitch := Stitch{ID: nextSocialID("stitch", len(s.stitches[req.SourceEchoID])+1), EchoID: nextSocialID("echo-stitch", len(s.stitches[req.SourceEchoID])+1), UserID: req.UserID, SourceEchoID: req.SourceEchoID, Title: strings.TrimSpace(req.Title), Body: strings.TrimSpace(req.Body), CreatedAt: now.UTC()}
	s.stitches[req.SourceEchoID] = append(s.stitches[req.SourceEchoID], stitch)
	s.echoAuthors[stitch.EchoID] = req.UserID
	s.notifyLocked(Notification{UserID: authorID, Kind: NotificationStitch, ActorID: req.UserID, TargetID: req.SourceEchoID, Preview: "Someone stitched your Echo.", CreatedAt: now.UTC()})
	return stitch, nil
}

func (s *SocialStore) AddReport(req ReportRequest, now time.Time) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	category, details, valid := normalizeReport(req)
	if req.TargetType == ReportTargetUser && req.TargetID == "" {
		handle, err := ValidateHandle(req.TargetHandle)
		if err == nil {
			req.TargetID = s.userByHandle[handle]
		}
	}
	if !validSocialID(req.ReporterID) || !validSocialID(req.TargetID) || req.ReporterID == req.TargetID || !validReportTarget(req.TargetType) || !valid {
		return Report{}, ErrSocialInvalid
	}
	report := Report{ID: nextSocialID("report", len(s.reports)+1), ReporterID: req.ReporterID, TargetType: req.TargetType, TargetID: req.TargetID, Category: category, Details: details, Reason: details, Status: "pending_review", CreatedAt: now.UTC()}
	s.reports = append(s.reports, report)
	return report, nil
}

func (s *SocialStore) Hide(req VisibilityRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validSocialID(req.UserID) || !validSocialID(req.TargetID) {
		return ErrSocialInvalid
	}
	if s.hides[req.UserID] == nil {
		s.hides[req.UserID] = map[string]bool{}
	}
	s.hides[req.UserID][req.TargetID] = true
	return nil
}

func (s *SocialStore) Block(req VisibilityRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validSocialID(req.UserID) || !validSocialID(req.TargetID) {
		return ErrSocialInvalid
	}
	if s.blocks[req.UserID] == nil {
		s.blocks[req.UserID] = map[string]bool{}
	}
	s.blocks[req.UserID][req.TargetID] = true
	return nil
}

func (s *SocialStore) Notifications(userID string) []Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Notification{}
	for _, notification := range s.notifications {
		if notification.UserID == userID && !s.blocked(userID, notification.ActorID) {
			out = append(out, notification)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *SocialStore) notifyLocked(notification Notification) {
	if s.blocked(notification.UserID, notification.ActorID) {
		return
	}
	notification.ID = nextSocialID("notif", len(s.notifications)+1)
	s.notifications = append(s.notifications, notification)
}

func (s *SocialStore) replyExists(echoID, replyID string) bool {
	for _, reply := range s.replies[echoID] {
		if reply.ID == replyID && reply.ReplyTo == "" {
			return true
		}
	}
	return false
}

func (s *SocialStore) hidden(userID, targetID string) bool {
	return s.hides[userID] != nil && s.hides[userID][targetID]
}
func (s *SocialStore) blocked(userID, actorID string) bool {
	return s.blocks[userID] != nil && s.blocks[userID][actorID]
}

func validSocialID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") || len(id) > 128 {
		return false
	}
	return true
}

func validReportTarget(target string) bool {
	return target == ReportTargetEcho || target == ReportTargetReply || target == ReportTargetUser
}

func normalizeReport(request ReportRequest) (string, string, bool) {
	category := strings.TrimSpace(request.Category)
	details := strings.TrimSpace(request.Details)
	if category == "" && strings.TrimSpace(request.Reason) != "" {
		category, details = "other", strings.TrimSpace(request.Reason)
	}
	validCategories := map[string]bool{
		"harassment": true, "hate": true, "sexual_content": true,
		"violence_threat": true, "illegal_dangerous": true,
		"privacy_location_safety": true, "spam_scam": true,
		"misinformation": true, "sensitive_place": true,
		"intellectual_property": true, "other": true,
	}
	return category, details, validCategories[category] && len([]rune(details)) <= 500
}

func validReactionKind(kind string) bool {
	switch kind {
	case ReactionKindHeart, ReactionKindFire, ReactionKindSad, ReactionKindLaugh, ReactionKindClap, ReactionKindMindBlown:
		return true
	default:
		return false
	}
}

func emptyReactionTotals() map[string]int {
	return map[string]int{ReactionKindHeart: 0, ReactionKindFire: 0, ReactionKindSad: 0, ReactionKindLaugh: 0, ReactionKindClap: 0, ReactionKindMindBlown: 0}
}

func nextSocialID(prefix string, n int) string { return prefix + "-" + strconv.Itoa(n) }
