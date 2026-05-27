package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateLimits(t *testing.T) {
	h := NewHandler(NewStore())
	body := strings.Repeat("a", 81)
	rec := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": body, "body": "ok", "lat": 1, "lon": 2}, "u1", "k1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestIdempotentCreate(t *testing.T) {
	h := NewHandler(NewStore())
	first := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": "hi", "body": "body", "lat": 1.2345, "lon": 2.3456}, "u1", "k1")
	second := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": "hi", "body": "body", "lat": 1.2345, "lon": 2.3456}, "u1", "k1")
	conflict := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": "different", "body": "body", "lat": 1.2345, "lon": 2.3456}, "u1", "k1")
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("unexpected codes %d %d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("expected same response")
	}
	if conflict.Code != http.StatusConflict {
		t.Fatalf("got %d", conflict.Code)
	}
}

func TestPublicAndPrivateRead(t *testing.T) {
	h := NewHandler(NewStore())
	created := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": "hi", "body": "body", "lat": 10.1234, "lon": 20.5678}, "u1", "k1")
	var createdResp echoResponse
	mustDecode(t, created.Body.Bytes(), &createdResp)
	pub := doReq(t, h, http.MethodGet, "/echoes/"+createdResp.ID, nil, "", "")
	if pub.Code != http.StatusForbidden {
		t.Fatalf("draft should be private to author, got %d", pub.Code)
	}
	priv := doReq(t, h, http.MethodGet, "/echoes/"+createdResp.ID, nil, "u1", "")
	var privResp echoResponse
	mustDecode(t, priv.Body.Bytes(), &privResp)
	if privResp.ExactLocation == nil || privResp.ExactLocation.Lat != 10.1234 || privResp.ExactLocation.Lon != 20.5678 {
		t.Fatalf("missing exact location")
	}
	if privResp.PublicLocation.Lat == 10.1234 || privResp.PublicLocation.Lon == 20.5678 {
		t.Fatalf("public location leaked exact coords")
	}
}

func TestPublishAndPublicRead(t *testing.T) {
	h := NewHandler(NewStore())
	created := doReq(t, h, http.MethodPost, "/echoes", map[string]any{"title": "hi", "body": "body", "lat": 10.1234, "lon": 20.5678}, "u1", "k1")
	var createdResp echoResponse
	mustDecode(t, created.Body.Bytes(), &createdResp)
	pub := doReq(t, h, http.MethodPost, "/echoes/"+createdResp.ID+"/publish", nil, "u1", "")
	if pub.Code != http.StatusOK {
		t.Fatalf("got %d", pub.Code)
	}
	read := doReq(t, h, http.MethodGet, "/echoes/"+createdResp.ID, nil, "", "")
	if read.Code != http.StatusOK {
		t.Fatalf("got %d", read.Code)
	}
	var publicResp echoResponse
	mustDecode(t, read.Body.Bytes(), &publicResp)
	if publicResp.ExactLocation != nil {
		t.Fatalf("public read leaked exact location: %#v", publicResp.ExactLocation)
	}
}

func doReq(t *testing.T, h http.Handler, method, path string, payload map[string]any, userID, idem string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&body).Encode(payload)
	}
	req := httptest.NewRequest(method, path, &body)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mustDecode(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}
