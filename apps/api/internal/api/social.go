package api

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ReactionKindEcho = "echo"
	ReactionKindSeen = "seen"

	ReportTargetEcho  = "echo"
	ReportTargetReply = "reply"
	ReportTargetUser  = "user"

	NotificationReaction = "reaction"
	NotificationReply    = "reply"
	NotificationStitch   = "stitch"
)

var ErrSocialInvalid = errors.New("invalid social request")
var ErrSocialNotFound = errors.New("social target not found")
var ErrSocialHidden = errors.New("social target hidden")

type SocialStore struct {
	mu            sync.RWMutex
	echoAuthors   map[string]string
	reactions     map[string]Reaction
	replies       map[string][]Reply
	stitches      map[string][]Stitch
	reports       []Report
	hides         map[string]map[string]bool
	blocks        map[string]map[string]bool
	notifications []Notification
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
	return &SocialStore{echoAuthors: authors, reactions: map[string]Reaction{}, replies: map[string][]Reply{}, stitches: map[string][]Stitch{}, hides: map[string]map[string]bool{}, blocks: map[string]map[string]bool{}}
}

type ReactionRequest struct {
	UserID string `json:"user_id"`
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
	UserID  string `json:"user_id"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

type Reply struct {
	ID       string    `json:"id"`
	EchoID   string    `json:"echo_id"`
	UserID   string    `json:"user_id"`
	Body     string    `json:"body"`
	ReplyTo  string    `json:"reply_to,omitempty"`
	Edited   bool      `json:"edited"`
	Hidden   bool      `json:"hidden,omitempty"`
	PostedAt time.Time `json:"posted_at"`
}

type StitchRequest struct {
	UserID       string `json:"user_id"`
	SourceEchoID string `json:"source_echo_id"`
	Title        string `json:"title"`
	Body         string `json:"body"`
}

type Stitch struct {
	ID           string    `json:"id"`
	EchoID       string    `json:"echo_id"`
	UserID       string    `json:"user_id"`
	SourceEchoID string    `json:"source_echo_id"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
}

type ReportRequest struct {
	ReporterID string `json:"reporter_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason"`
}

type Report struct {
	ID         string    `json:"id"`
	ReporterID string    `json:"reporter_id"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Reason     string    `json:"reason"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type VisibilityRequest struct {
	UserID   string `json:"user_id"`
	TargetID string `json:"target_id"`
}

type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	TargetID  string    `json:"target_id"`
	Preview   string    `json:"preview"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *SocialStore) PutReaction(echoID string, req ReactionRequest, now time.Time) (ReactionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validSocialID(echoID) || !validSocialID(req.UserID) || (req.Kind != ReactionKindEcho && req.Kind != ReactionKindSeen) || s.hidden(req.UserID, echoID) {
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
		return ReactionSummary{EchoID: echoID, Totals: map[string]int{ReactionKindEcho: 0, ReactionKindSeen: 0}}
	}
	return s.reactionSummaryLocked(echoID)
}

func (s *SocialStore) EchoKnown(echoID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return validSocialID(echoID) && s.echoAuthors[echoID] != ""
}

func (s *SocialStore) reactionSummaryLocked(echoID string) ReactionSummary {
	totals := map[string]int{ReactionKindEcho: 0, ReactionKindSeen: 0}
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
	reply := Reply{ID: nextSocialID("reply", len(s.replies[echoID])+1), EchoID: echoID, UserID: req.UserID, Body: body, ReplyTo: req.ReplyTo, PostedAt: now.UTC()}
	s.replies[echoID] = append(s.replies[echoID], reply)
	s.notifyLocked(Notification{UserID: authorID, Kind: NotificationReply, ActorID: req.UserID, TargetID: echoID, Preview: "Someone replied to your Echo.", CreatedAt: now.UTC()})
	return reply, nil
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
	if !validSocialID(req.ReporterID) || !validSocialID(req.TargetID) || !validReportTarget(req.TargetType) || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 {
		return Report{}, ErrSocialInvalid
	}
	report := Report{ID: nextSocialID("report", len(s.reports)+1), ReporterID: req.ReporterID, TargetType: req.TargetType, TargetID: req.TargetID, Reason: strings.TrimSpace(req.Reason), Status: "pending_review", CreatedAt: now.UTC()}
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

func nextSocialID(prefix string, n int) string { return prefix + "-" + strconv.Itoa(n) }
