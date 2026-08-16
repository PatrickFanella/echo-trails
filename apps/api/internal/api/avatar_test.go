package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAvatarLifecycleRequiresOwnedApprovedImage(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	profiles := NewMemoryProfileStore()
	if _, err := profiles.UpsertProfile(context.Background(), "user-owner", "owner@example.com", UpsertProfileRequest{Handle: "trail_owner", DisplayName: "Trail Owner"}, now); err != nil {
		t.Fatal(err)
	}
	media := NewMemoryMediaUploadStore()
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Profiles: profiles, Media: media, Clock: fixedClock{now: now}})

	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/me/avatar/uploads", strings.NewReader(`{"content_type":"image/jpeg","byte_size":2048}`))
	request.Header.Set("X-EchoTrails-User-ID", "user-owner")
	request.Header.Set("Idempotency-Key", "avatar-create-1")
	handler.ServeHTTP(create, request)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var upload MediaUploadResponse
	if err := json.NewDecoder(create.Body).Decode(&upload); err != nil {
		t.Fatal(err)
	}
	if upload.MediaAssetID == "" || upload.UploadURL == "" || !strings.Contains(upload.ObjectKey, "/original.") {
		t.Fatalf("unexpected avatar upload: %+v", upload)
	}

	selectAvatar := func(userID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/me/avatar", strings.NewReader(`{"media_asset_id":"`+upload.MediaAssetID+`"}`))
		request.Header.Set("X-EchoTrails-User-ID", userID)
		request.Header.Set("Idempotency-Key", "avatar-select-"+userID)
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	if pending := selectAvatar("user-owner"); pending.Code != http.StatusConflict {
		t.Fatalf("pending selection status=%d body=%s", pending.Code, pending.Body.String())
	}
	media.ApproveAvatarAsset(upload.MediaAssetID)
	if other := selectAvatar("user-other"); other.Code != http.StatusConflict {
		t.Fatalf("cross-owner selection status=%d body=%s", other.Code, other.Body.String())
	}
	selected := selectAvatar("user-owner")
	if selected.Code != http.StatusOK {
		t.Fatalf("approved selection status=%d body=%s", selected.Code, selected.Body.String())
	}
	var me MeProfile
	if err := json.NewDecoder(selected.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.AvatarURL != "/media/assets/"+upload.MediaAssetID {
		t.Fatalf("unexpected private avatar URL: %q", me.AvatarURL)
	}
	public := toPublicProfile(mustMemoryProfile(t, profiles, "user-owner"))
	if public.AvatarURL != me.AvatarURL {
		t.Fatalf("public avatar URL mismatch: %+v", public)
	}
	if strings.Contains(public.AvatarURL, upload.ObjectKey) || strings.Contains(public.AvatarURL, "original") {
		t.Fatalf("public profile leaked private object metadata: %+v", public)
	}
}

func TestAvatarUploadRejectsNonImageAndOversize(t *testing.T) {
	handler := NewHandler(NewStore(DefaultPlaces()))
	for _, body := range []string{
		`{"content_type":"video/mp4","byte_size":1024}`,
		`{"content_type":"image/png","byte_size":10485761}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/me/avatar/uploads", strings.NewReader(body))
		request.Header.Set("X-EchoTrails-User-ID", "user-owner")
		request.Header.Set("Idempotency-Key", "avatar-invalid")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

func mustMemoryProfile(t *testing.T, profiles *MemoryProfileStore, userID string) Profile {
	t.Helper()
	profile, found, err := profiles.GetProfileByUserID(context.Background(), userID)
	if err != nil || !found {
		t.Fatalf("profile lookup found=%v err=%v", found, err)
	}
	return profile
}
