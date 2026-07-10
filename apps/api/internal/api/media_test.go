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

func TestMediaUploadLifecycleIsIdempotentAndHidesURLAfterComplete(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	body := []byte(`{"echo_id":"echo-active-1","media_type":"image","content_type":"image/jpeg","byte_size":1024}`)

	create := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/uploads", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "upload-key-1")
	req.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(create, req)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var first MediaUploadResponse
	if err := json.NewDecoder(create.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if first.UploadID == "" || first.UploadURL == "" || first.Method != "PUT" || !strings.HasPrefix(first.ObjectKey, "media/original/ast_") {
		t.Fatalf("bad upload response: %+v", first)
	}

	replay := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/media/uploads", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "upload-key-1")
	req.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(replay, req)
	var second MediaUploadResponse
	_ = json.NewDecoder(replay.Body).Decode(&second)
	if second.UploadID != first.UploadID {
		t.Fatalf("idempotency returned different upload: %s != %s", second.UploadID, first.UploadID)
	}

	complete := httptest.NewRecorder()
	completeRequest := httptest.NewRequest(http.MethodPost, "/media/uploads/"+first.UploadID+"/complete", nil)
	completeRequest.Header.Set("Idempotency-Key", "complete-1")
	completeRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(complete, completeRequest)
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status = %d body=%s", complete.Code, complete.Body.String())
	}
	var completed MediaUploadResponse
	_ = json.NewDecoder(complete.Body).Decode(&completed)
	if completed.Status != MediaUploadStatusUploaded || completed.ActivityStatus != MediaActivityStatusProcessing || completed.UploadURL != "" {
		t.Fatalf("complete did not hide URL: %+v", completed)
	}
	if completed.TranscriptStatus != MediaTranscriptStatusPending || completed.ModerationStatus != MediaModerationStatusPending || completed.ModerationRisk != MediaModerationRiskUnknown {
		t.Fatalf("complete did not expose safe activity status: %+v", completed)
	}
}

func TestMediaUploadRejectsBadContentTypeAndSize(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	for _, body := range []string{
		`{"echo_id":"echo-active-1","media_type":"image","content_type":"text/plain","byte_size":1024}`,
		`{"echo_id":"echo-active-1","media_type":"image","content_type":"image/png","byte_size":1024}`,
		`{"echo_id":"echo-active-1","media_type":"image","content_type":"image/png","byte_size":0}`,
	} {
		rec := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/media/uploads", strings.NewReader(body))
		request.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
		h.ServeHTTP(rec, request)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestMediaUploadStatusExposesActivityLifecycleWithoutPrivateURL(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	body := []byte(`{"echo_id":"echo-active-1","media_type":"audio","content_type":"audio/mp4","byte_size":2048}`)
	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/media/uploads", bytes.NewReader(body))
	createRequest.Header.Set("Idempotency-Key", "create-status-1")
	createRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(create, createRequest)
	var created MediaUploadResponse
	_ = json.NewDecoder(create.Body).Decode(&created)

	complete := httptest.NewRecorder()
	completeRequest := httptest.NewRequest(http.MethodPost, "/media/uploads/"+created.UploadID+"/complete", nil)
	completeRequest.Header.Set("Idempotency-Key", "complete-status-1")
	completeRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(complete, completeRequest)
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status = %d body=%s", complete.Code, complete.Body.String())
	}

	status := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodGet, "/media/uploads/"+created.UploadID, nil)
	statusRequest.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(status, statusRequest)
	var activity MediaUploadResponse
	_ = json.NewDecoder(status.Body).Decode(&activity)
	if activity.UploadURL != "" || activity.ObjectKey == "" || activity.ActivityStatus != MediaActivityStatusProcessing {
		t.Fatalf("bad activity status: %+v", activity)
	}
}

func TestExpiredMediaUploadCanRenewPrivateAuthorization(t *testing.T) {
	store := NewMemoryMediaUploadStore()
	created, err := store.CreateUpload(context.Background(), "owner", CreateMediaUploadRequest{EchoID: "echo", MediaType: "image", ContentType: "image/jpeg", ByteSize: 12}, "renew-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	record := store.uploads[created.UploadID]
	record.ExpiresAt = time.Now().Add(-time.Minute)
	store.uploads[created.UploadID] = record
	store.mu.Unlock()
	renewed, err := store.RenewUpload(context.Background(), "owner", created.UploadID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if renewed.UploadURL == "" || renewed.Status != MediaUploadStatusCreated || !renewed.ExpiresAt.After(time.Now()) {
		t.Fatalf("renewal did not return a fresh private target: %+v", renewed)
	}
}

func TestMediaUploadStatusNotFound(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/media/uploads/upl_missing", nil)
	request.Header.Set("X-EchoTrails-User-ID", "user-seed-1")
	h.ServeHTTP(rec, request)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMediaUploadIdempotencyReplayMarksExpired(t *testing.T) {
	store := NewMemoryMediaUploadStore()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now.Add(20 * time.Minute) }
	created, err := store.CreateUpload(context.Background(), "user-1", CreateMediaUploadRequest{EchoID: "echo-1", MediaType: "image", ContentType: "image/jpeg", ByteSize: 10}, "key-expire", now)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CreateUpload(context.Background(), "user-1", CreateMediaUploadRequest{EchoID: "echo-1", MediaType: "image", ContentType: "image/jpeg", ByteSize: 10}, "key-expire", now.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.UploadID != created.UploadID || replayed.Status != MediaUploadStatusExpired || replayed.ActivityStatus != MediaActivityStatusExpired || replayed.UploadURL != "" {
		t.Fatalf("bad expired replay: %+v", replayed)
	}
}
