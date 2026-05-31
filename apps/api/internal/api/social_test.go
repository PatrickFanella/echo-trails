package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReactionsKeepOneTotalPerUser(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	putJSON(t, h, "/echoes/echo-active-1/reactions", `{"user_id":"user-a","kind":"echo"}`, http.StatusOK)
	response := putJSON(t, h, "/echoes/echo-active-1/reactions", `{"user_id":"user-a","kind":"seen"}`, http.StatusOK)
	var summary ReactionSummary
	decodeRecorderJSON(t, response, &summary)
	if summary.Totals[ReactionKindEcho] != 0 || summary.Totals[ReactionKindSeen] != 1 {
		t.Fatalf("bad reaction summary: %+v", summary)
	}
}

func TestRepliesAreTextOnlyOneLevelAndOldestFirst(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	first := postJSON(t, h, "/echoes/echo-active-1/replies", `{"user_id":"user-a","body":"first"}`, http.StatusCreated)
	var reply Reply
	decodeRecorderJSON(t, first, &reply)
	postJSON(t, h, "/echoes/echo-active-1/replies", `{"user_id":"user-b","body":"second","reply_to":"`+reply.ID+`"}`, http.StatusCreated)
	postJSON(t, h, "/echoes/echo-active-1/replies", `{"user_id":"user-c","body":"bad","reply_to":"missing"}`, http.StatusBadRequest)

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

func TestStitchesCreateSourceLinkAndNotification(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	response := postJSON(t, h, "/echoes/echo-active-1/stitches", `{"user_id":"user-a","title":"New angle","body":"Link this memory"}`, http.StatusCreated)
	var stitch Stitch
	decodeRecorderJSON(t, response, &stitch)
	if stitch.SourceEchoID != "echo-active-1" || !strings.HasPrefix(stitch.EchoID, "echo-stitch-") {
		t.Fatalf("bad stitch: %+v", stitch)
	}
	notifications := httptest.NewRecorder()
	h.ServeHTTP(notifications, httptest.NewRequest(http.MethodGet, "/notifications?user_id=user-seed-1", nil))
	if !strings.Contains(notifications.Body.String(), NotificationStitch) {
		t.Fatalf("stitch notification missing: %s", notifications.Body.String())
	}
}

func TestReportsBlocksAndHidesGateVisibilityAndNotifications(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	postJSON(t, h, "/reports", `{"reporter_id":"viewer","target_type":"echo","target_id":"echo-active-1","reason":"unsafe"}`, http.StatusCreated)
	postJSON(t, h, "/hides", `{"user_id":"viewer","target_id":"echo-active-1"}`, http.StatusNoContent)
	putJSON(t, h, "/echoes/echo-active-1/reactions", `{"user_id":"viewer","kind":"echo"}`, http.StatusBadRequest)

	postJSON(t, h, "/blocks", `{"user_id":"user-seed-1","target_id":"blocked-user"}`, http.StatusNoContent)
	postJSON(t, h, "/echoes/echo-active-1/replies", `{"user_id":"blocked-user","body":"hidden reply"}`, http.StatusForbidden)
	notifications := httptest.NewRecorder()
	h.ServeHTTP(notifications, httptest.NewRequest(http.MethodGet, "/notifications?user_id=user-seed-1", nil))
	if strings.Contains(notifications.Body.String(), "blocked-user") {
		t.Fatalf("blocked actor leaked into notifications: %s", notifications.Body.String())
	}
}

func TestSocialEchoValidationUsesInjectedStore(t *testing.T) {
	h := &Handler{store: NewStore(nil), media: NewMemoryMediaUploadStore(), social: NewSocialStoreWithEchoAuthors(map[string]string{})}
	putJSON(t, h, "/echoes/echo-active-1/reactions", `{"user_id":"user-a","kind":"echo"}`, http.StatusNotFound)
}

func TestSocialRejectsGhostEchoAndMalformedRoutes(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	putJSON(t, h, "/echoes/ghost/reactions", `{"user_id":"user-a","kind":"echo"}`, http.StatusNotFound)

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/echoes/a/b/reactions", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("malformed route status = %d", recorder.Code)
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
	if recorder.Code != want {
		t.Fatalf("POST %s status = %d want %d body=%s", path, recorder.Code, want, recorder.Body.String())
	}
	return recorder
}

func putJSON(t *testing.T, handler http.Handler, path string, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body)))
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
