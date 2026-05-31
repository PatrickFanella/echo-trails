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
	h.ServeHTTP(replay, req)
	var second MediaUploadResponse
	_ = json.NewDecoder(replay.Body).Decode(&second)
	if second.UploadID != first.UploadID {
		t.Fatalf("idempotency returned different upload: %s != %s", second.UploadID, first.UploadID)
	}

	complete := httptest.NewRecorder()
	h.ServeHTTP(complete, httptest.NewRequest(http.MethodPost, "/media/uploads/"+first.UploadID+"/complete", nil))
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status = %d body=%s", complete.Code, complete.Body.String())
	}
	var completed MediaUploadResponse
	_ = json.NewDecoder(complete.Body).Decode(&completed)
	if completed.Status != MediaUploadStatusUploaded || completed.UploadURL != "" {
		t.Fatalf("complete did not hide URL: %+v", completed)
	}
}

func TestMediaUploadRejectsBadContentTypeAndSize(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	for _, body := range []string{
		`{"echo_id":"echo-active-1","media_type":"image","content_type":"text/plain","byte_size":1024}`,
		`{"echo_id":"echo-active-1","media_type":"image","content_type":"image/png","byte_size":0}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/media/uploads", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestMediaUploadStatusNotFound(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/uploads/upl_missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMediaUploadIdempotencyReplayMarksExpired(t *testing.T) {
	store := NewMemoryMediaUploadStore()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now.Add(20 * time.Minute) }
	created, err := store.CreateUpload(context.Background(), CreateMediaUploadRequest{EchoID: "echo-1", MediaType: "image", ContentType: "image/png", ByteSize: 10}, "key-expire", now)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CreateUpload(context.Background(), CreateMediaUploadRequest{EchoID: "echo-1", MediaType: "image", ContentType: "image/png", ByteSize: 10}, "key-expire", now.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.UploadID != created.UploadID || replayed.Status != MediaUploadStatusExpired || replayed.UploadURL != "" {
		t.Fatalf("bad expired replay: %+v", replayed)
	}
}
