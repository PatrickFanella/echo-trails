package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseReplyMentionHandlesDeduplicatesAndIgnoresEmailAddresses(t *testing.T) {
	handles := parseReplyMentionHandles("Hi @Trail_Friend, @trail_friend! Email person@example.com and ignore @x.")
	if len(handles) != 1 || handles[0] != "trail_friend" {
		t.Fatalf("mention handles = %#v", handles)
	}
}

func TestReplyMentionsAreLimitedToThreadParticipantsAndDeduplicated(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewSocialStoreWithEchoAuthors(map[string]string{"echo-thread": "user-author"})
	store.RegisterParticipantHandle("user-author", "echo_author")
	store.RegisterParticipantHandle("user-participant", "trail_friend")
	store.RegisterParticipantHandle("user-outsider", "outside_user")
	store.RegisterParticipantHandle("user-actor", "reply_actor")

	if _, err := store.AddReply("echo-thread", ReplyRequest{UserID: "user-participant", Body: "Joining the thread."}, now); err != nil {
		t.Fatal(err)
	}
	reply, err := store.AddReply("echo-thread", ReplyRequest{UserID: "user-actor", Body: "Thanks @trail_friend and @TRAIL_FRIEND. Hi @outside_user and @echo_author."}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	mentions := store.replyMentions[reply.ID]
	if len(mentions) != 2 || !mentions["user-participant"] || !mentions["user-author"] || mentions["user-outsider"] {
		t.Fatalf("persisted mentions = %#v", mentions)
	}
	participantNotifications := store.Notifications("user-participant")
	if len(participantNotifications) != 1 || participantNotifications[0].Kind != NotificationMention {
		t.Fatalf("participant notifications = %#v", participantNotifications)
	}
	authorNotifications := store.Notifications("user-author")
	mentionCount := 0
	for _, notification := range authorNotifications {
		if notification.Kind == NotificationMention {
			mentionCount++
		}
	}
	if len(authorNotifications) != 2 || mentionCount != 0 {
		t.Fatalf("author should receive one ordinary Reply notification per Reply without a duplicate mention: %#v", authorNotifications)
	}
}

func TestReplyMentionEditsNotifyOnlyNewUnblockedParticipants(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := NewSocialStoreWithEchoAuthors(map[string]string{"echo-thread": "user-author"})
	store.RegisterParticipantHandle("user-participant", "trail_friend")
	if _, err := store.AddReply("echo-thread", ReplyRequest{UserID: "user-participant", Body: "Joining."}, now); err != nil {
		t.Fatal(err)
	}
	reply, err := store.AddReply("echo-thread", ReplyRequest{UserID: "user-actor", Body: "No mention yet."}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewMemorySocialRepository(store)
	if _, err = repository.UpdateReply(context.Background(), reply.ID, "user-actor", "Now @trail_friend is mentioned.", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateReply(context.Background(), reply.ID, "user-actor", "Again @trail_friend is mentioned.", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := store.Notifications("user-participant"); len(got) != 1 || got[0].Kind != NotificationMention {
		t.Fatalf("edited mention notifications = %#v", got)
	}
	notificationCount := len(store.notifications)
	if err = store.Block(VisibilityRequest{UserID: "user-participant", TargetID: "user-actor"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateReply(context.Background(), reply.ID, "user-actor", "Removed mention.", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.UpdateReply(context.Background(), reply.ID, "user-actor", "Blocked @trail_friend mention.", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if store.replyMentions[reply.ID]["user-participant"] || len(store.notifications) != notificationCount || len(store.Notifications("user-participant")) != 0 {
		t.Fatalf("blocked participant was mentioned or notified")
	}
}

func TestReactionsKeepOneTotalPerUser(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	putJSONAs(t, h, "/echoes/echo-active-1/reactions", `{"kind":"heart"}`, "user-a", http.StatusOK)
	response := putJSONAs(t, h, "/echoes/echo-active-1/reactions", `{"kind":"clap"}`, "user-a", http.StatusOK)
	var summary ReactionSummary
	decodeRecorderJSON(t, response, &summary)
	if summary.Totals[ReactionKindHeart] != 0 || summary.Totals[ReactionKindClap] != 1 {
		t.Fatalf("bad reaction summary: %+v", summary)
	}
}

func TestRepliesAreTextOnlyOneLevelAndOldestFirst(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	first := postJSONAs(t, h, "/echoes/echo-active-1/replies", `{"body":"first"}`, "user-a", http.StatusCreated)
	var reply Reply
	decodeRecorderJSON(t, first, &reply)
	postJSONAs(t, h, "/echoes/echo-active-1/replies", `{"body":"second","reply_to":"`+reply.ID+`"}`, "user-b", http.StatusCreated)
	postJSONAs(t, h, "/echoes/echo-active-1/replies", `{"body":"bad","reply_to":"missing"}`, "user-c", http.StatusBadRequest)

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/echoes/echo-active-1/replies", nil))
	var payload struct {
		Replies []Reply `json:"replies"`
	}
	decodeRecorderJSON(t, list, &payload)
	if len(payload.Replies) != 2 || payload.Replies[0].Body != "first" || payload.Replies[1].ReplyTo != reply.ID {
		t.Fatalf("bad replies: %+v", payload.Replies)
	}
}

func TestReplyEditsKeepRevisionHistoryAndDeleteContinuity(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	created := postJSONAs(t, h, "/echoes/echo-active-1/replies", `{"body":"first version"}`, "user-a", http.StatusCreated)
	var reply Reply
	decodeRecorderJSON(t, created, &reply)

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/replies/"+reply.ID, bytes.NewBufferString(`{"body":"second version"}`))
	patchRequest.Header.Set("X-EchoTrails-User-ID", "user-a")
	patchRequest.Header.Set("Idempotency-Key", "reply-edit-1")
	h.ServeHTTP(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("edit status = %d body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}

	historyRecorder := httptest.NewRecorder()
	historyRequest := httptest.NewRequest(http.MethodGet, "/replies/"+reply.ID+"/history", nil)
	historyRequest.Header.Set("X-EchoTrails-User-ID", "user-a")
	h.ServeHTTP(historyRecorder, historyRequest)
	var history struct {
		Revisions []ReplyRevision `json:"revisions"`
	}
	decodeRecorderJSON(t, historyRecorder, &history)
	if len(history.Revisions) != 2 || history.Revisions[0].Body != "first version" || history.Revisions[1].Body != "second version" {
		t.Fatalf("bad revision history: %+v", history.Revisions)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/replies/"+reply.ID, nil)
	deleteRequest.Header.Set("X-EchoTrails-User-ID", "user-a")
	deleteRequest.Header.Set("Idempotency-Key", "reply-delete-1")
	h.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleteRecorder.Code)
	}
	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/echoes/echo-active-1/replies", nil))
	if !strings.Contains(list.Body.String(), "Reply deleted") {
		t.Fatalf("deleted continuity placeholder missing: %s", list.Body.String())
	}
}

func TestStitchesCreateSourceLinkAndNotification(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	response := postJSONAs(t, h, "/echoes/echo-active-1/stitches", `{"title":"New angle","body":"Link this memory"}`, "user-a", http.StatusCreated)
	var stitch Stitch
	decodeRecorderJSON(t, response, &stitch)
	if stitch.SourceEchoID != "echo-active-1" || !strings.HasPrefix(stitch.EchoID, "echo-stitch-") {
		t.Fatalf("bad stitch: %+v", stitch)
	}
	notifications := httptest.NewRecorder()
	notificationRequest := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	notificationRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(notifications, notificationRequest)
	if !strings.Contains(notifications.Body.String(), NotificationStitch) {
		t.Fatalf("stitch notification missing: %s", notifications.Body.String())
	}
}

func TestReportsBlocksAndHidesGateVisibilityAndNotifications(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	reportResponse := postJSONAs(t, h, "/reports", `{"target_type":"echo","target_id":"echo-active-1","category":"privacy_location_safety","details":"Exact location shown"}`, "viewer", http.StatusCreated)
	var report Report
	decodeRecorderJSON(t, reportResponse, &report)
	if report.Category != "privacy_location_safety" || report.Details != "Exact location shown" {
		t.Fatalf("structured report lost category/details: %+v", report)
	}
	postJSONAs(t, h, "/hides", `{"target_id":"echo-active-1"}`, "viewer", http.StatusNoContent)
	putJSONAs(t, h, "/echoes/echo-active-1/reactions", `{"kind":"heart"}`, "viewer", http.StatusBadRequest)

	postJSONAs(t, h, "/blocks", `{"target_id":"blocked-user"}`, "user-seed-1", http.StatusNoContent)
	postJSONAs(t, h, "/echoes/echo-active-1/replies", `{"body":"hidden reply"}`, "blocked-user", http.StatusForbidden)
	notifications := httptest.NewRecorder()
	notificationRequest := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	notificationRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(notifications, notificationRequest)
	if strings.Contains(notifications.Body.String(), "blocked-user") {
		t.Fatalf("blocked actor leaked into notifications: %s", notifications.Body.String())
	}
}

func TestBlockResolvesPublicHandleWithoutExposingUserID(t *testing.T) {
	profiles := NewMemoryProfileStore()
	social := NewMemorySocialRepository(NewSocialStore())
	h := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Profiles: profiles, Social: social})
	if _, err := profiles.UpsertProfile(context.Background(), "blocked-user", "blocked@invalid.example", UpsertProfileRequest{Handle: "trail_neighbor", DisplayName: "Trail Neighbor"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	postJSONAs(t, h, "/blocks", `{"target_handle":"trail_neighbor"}`, "viewer", http.StatusNoContent)
	if err := social.Block(context.Background(), VisibilityRequest{UserID: "viewer", TargetID: "blocked-user"}); err != nil {
		t.Fatalf("resolved block was not idempotent: %v", err)
	}
}

func TestUserReportResolvesPublicHandleWithoutExposingUserID(t *testing.T) {
	profiles := NewMemoryProfileStore()
	social := NewMemorySocialRepository(NewSocialStore())
	h := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Profiles: profiles, Social: social})
	if _, err := profiles.UpsertProfile(context.Background(), "reported-user", "reported@invalid.example", UpsertProfileRequest{Handle: "trail_neighbor", DisplayName: "Trail Neighbor"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	response := postJSONAs(t, h, "/reports", `{"target_type":"user","target_handle":"trail_neighbor","category":"harassment","details":"Repeated harassment"}`, "viewer", http.StatusCreated)
	var report Report
	decodeRecorderJSON(t, response, &report)
	if report.TargetID != "reported-user" || report.TargetType != ReportTargetUser {
		t.Fatalf("resolved report = %+v", report)
	}
}

func TestSocialEchoValidationUsesInjectedStore(t *testing.T) {
	h := &Handler{store: NewStore(nil), media: NewMemoryMediaUploadStore(), social: NewMemorySocialRepository(NewSocialStoreWithEchoAuthors(map[string]string{}))}
	putJSONAs(t, h, "/echoes/echo-active-1/reactions", `{"kind":"heart"}`, "user-a", http.StatusNotFound)
}

func TestSocialRejectsGhostEchoAndMalformedRoutes(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	putJSONAs(t, h, "/echoes/ghost/reactions", `{"kind":"heart"}`, "user-a", http.StatusNotFound)

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/echoes/a/b/reactions", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("malformed route status = %d", recorder.Code)
	}
}

func postJSONAs(t *testing.T, handler http.Handler, path string, body string, userID string, want int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("X-EchoTrails-User-ID", userID)
	request.Header.Set("Idempotency-Key", "test-post-"+path)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("POST %s status = %d want %d body=%s", path, recorder.Code, want, recorder.Body.String())
	}
	return recorder
}

func putJSONAs(t *testing.T, handler http.Handler, path string, body string, userID string, want int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
	request.Header.Set("X-EchoTrails-User-ID", userID)
	request.Header.Set("Idempotency-Key", "test-put-"+path)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("PUT %s status = %d want %d body=%s", path, recorder.Code, want, recorder.Body.String())
	}
	return recorder
}

func decodeRecorderJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
