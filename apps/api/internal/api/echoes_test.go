package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type boundaryAwarePlaceStore struct {
	PlaceStore
	allowed           bool
	err               error
	calls             int
	constrainedPublic *Coordinate
}

func (s *boundaryAwarePlaceStore) ConstrainPublicLocation(_ context.Context, _ string, candidate Coordinate) (Coordinate, error) {
	if s.constrainedPublic != nil {
		return *s.constrainedPublic, s.err
	}
	return candidate, s.err
}

func (s *boundaryAwarePlaceStore) CreationLocationAllowed(_ context.Context, _ string, _, _ float64) (bool, error) {
	s.calls++
	return s.allowed, s.err
}

func TestCreationTokenAndEchoLifecycleArePlaceBoundAndPrivate(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	profiles := NewMemoryProfileStore()
	if _, err := profiles.UpsertProfile(context.Background(), "user-author", "author@example.com", UpsertProfileRequest{Handle: "place_author", DisplayName: "Place Author"}, now); err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{
		Profiles: profiles, Clock: fixedClock{now: now}, CreationSigningKey: []byte("test-creation-key"), PublicLocationKey: []byte("test-location-key"),
	})

	proofPayload := fmt.Sprintf(`{"place_id":%q,"latitude":41.7975912,"longitude":-88.0153297,"accuracy_meters":15,"captured_at":"2026-07-09T17:59:30Z"}`, PlacePrincePondID)
	proof := doJSON(t, handler, http.MethodPost, "/creation-tokens", proofPayload, authHeaders("user-author", "author@example.com"))
	if proof.Code != http.StatusCreated {
		t.Fatalf("proof status = %d body=%s", proof.Code, proof.Body.String())
	}
	var proofBody struct {
		CreationToken string `json:"creation_token"`
	}
	if err := json.NewDecoder(proof.Body).Decode(&proofBody); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"place_id":%q,"content_mode":"text","title":"A local memory","body_text":"People met here after the storm.","creation_token":%q}`, PlacePrincePondID, proofBody.CreationToken)
	created := doJSON(t, handler, http.MethodPost, "/echoes", body, authHeaders("user-author", "author@example.com"), map[string]string{"Idempotency-Key": "echo-create-1"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "41.7975912") || strings.Contains(created.Body.String(), "-88.0153297") || strings.Contains(created.Body.String(), "exact_location") {
		t.Fatalf("creation response leaked exact location: %s", created.Body.String())
	}
	var echo EchoResponse
	if err := json.NewDecoder(created.Body).Decode(&echo); err != nil {
		t.Fatal(err)
	}
	if echo.AuthorHandle != "place_author" || (echo.PublicLocation.Lat == 41.7975912 && echo.PublicLocation.Lon == -88.0153297) {
		t.Fatalf("private/public location contract failed: %+v", echo)
	}

	privateDraft := doJSON(t, handler, http.MethodGet, "/echoes/"+echo.ID, ``, authHeaders("user-other", "other@example.com"))
	if privateDraft.Code != http.StatusForbidden {
		t.Fatalf("draft visibility status = %d", privateDraft.Code)
	}
	submitted := doJSON(t, handler, http.MethodPost, "/echoes/"+echo.ID+"/submit", ``, authHeaders("user-author", "author@example.com"))
	if submitted.Code != http.StatusOK {
		t.Fatalf("submit status = %d body=%s", submitted.Code, submitted.Body.String())
	}
	public := doJSON(t, handler, http.MethodGet, "/echoes/"+echo.ID, ``)
	if public.Code != http.StatusOK {
		t.Fatalf("public status = %d body=%s", public.Code, public.Body.String())
	}
	var publicEcho EchoResponse
	if err := json.NewDecoder(public.Body).Decode(&publicEcho); err != nil {
		t.Fatal(err)
	}
	if publicEcho.Status != EchoStatusPublished {
		t.Fatalf("public response leaked private state: %+v", publicEcho)
	}

	replay := doJSON(t, handler, http.MethodPost, "/echoes", body, authHeaders("user-author", "author@example.com"), map[string]string{"Idempotency-Key": "echo-create-1"})
	var replayEcho EchoResponse
	if err := json.NewDecoder(replay.Body).Decode(&replayEcho); err != nil {
		t.Fatal(err)
	}
	if replay.Code != http.StatusCreated || replayEcho.ID != echo.ID || replayEcho.Status != EchoStatusDraft {
		t.Fatalf("idempotent replay mismatch: %d %+v", replay.Code, replayEcho)
	}
	reuse := doJSON(t, handler, http.MethodPost, "/echoes", body, authHeaders("user-author", "author@example.com"), map[string]string{"Idempotency-Key": "echo-create-2"})
	if reuse.Code != http.StatusForbidden {
		t.Fatalf("creation token reuse status = %d", reuse.Code)
	}
}

func TestCreationTokenRejectsRestrictedPlace(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Clock: fixedClock{now: now}})
	payload := fmt.Sprintf(`{"place_id":%q,"latitude":41.8065417,"longitude":-88.0122581,"accuracy_meters":15,"captured_at":"2026-07-09T17:59:30Z"}`, PlaceNorthHighSchoolID)
	response := doJSON(t, handler, http.MethodPost, "/creation-tokens", payload, authHeaders("user-author", "author@example.com"))
	if response.Code != http.StatusForbidden {
		t.Fatalf("restricted proof status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestCreationTokenUsesAuthoritativeBoundaryInsteadOfPointRadius(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	base := NewStore(DefaultPlaces())
	store := &boundaryAwarePlaceStore{PlaceStore: base, allowed: true}
	handler := NewHandlerWithConfig(store, HandlerConfig{Clock: fixedClock{now: now}})

	// This coordinate is far outside Prince Pond's point radius. A production
	// repository can still accept it when an imported polygon plus its 25 m
	// access buffer contains the proof.
	payload := fmt.Sprintf(`{"place_id":%q,"latitude":41.8005,"longitude":-88.0153297,"accuracy_meters":15,"captured_at":"2026-07-09T17:59:30Z"}`, PlacePrincePondID)
	response := doJSON(t, handler, http.MethodPost, "/creation-tokens", payload, authHeaders("user-author", "author@example.com"))
	if response.Code != http.StatusCreated {
		t.Fatalf("boundary-authorized proof status = %d body=%s", response.Code, response.Body.String())
	}
	if store.calls != 1 {
		t.Fatalf("boundary checks = %d, want 1", store.calls)
	}
}

func TestCreationTokenRejectsWhenAuthoritativeBoundaryRejectsPoint(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	store := &boundaryAwarePlaceStore{PlaceStore: NewStore(DefaultPlaces()), allowed: false}
	handler := NewHandlerWithConfig(store, HandlerConfig{Clock: fixedClock{now: now}})

	payload := fmt.Sprintf(`{"place_id":%q,"latitude":41.7975912,"longitude":-88.0153297,"accuracy_meters":15,"captured_at":"2026-07-09T17:59:30Z"}`, PlacePrincePondID)
	response := doJSON(t, handler, http.MethodPost, "/creation-tokens", payload, authHeaders("user-author", "author@example.com"))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "outside_place_boundary") {
		t.Fatalf("boundary-rejected proof status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestPublicJitterIsConstrainedToImportedBoundary(t *testing.T) {
	want := Coordinate{Lat: 41.7977, Lon: -88.0151}
	store := &boundaryAwarePlaceStore{PlaceStore: NewStore(DefaultPlaces()), allowed: true, constrainedPublic: &want}
	handler := NewHandlerWithConfig(store, HandlerConfig{PublicLocationKey: []byte("test-location-key")}).(*Handler)
	place, found, err := store.GetPlace(context.Background(), PlacePrincePondID)
	if err != nil || !found {
		t.Fatalf("load place: found=%v err=%v", found, err)
	}
	got, err := handler.publicLocationForPlace(context.Background(), place, "stable-echo-seed")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("public coordinate = %+v, want boundary-constrained %+v", got, want)
	}
}

func TestAuthorCanPatchAndDeleteDraftEcho(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	store := NewMemoryEchoStore([]EchoRecord{{
		ID: "echo-draft-1", AuthorUserID: "user-author", AuthorHandle: "place_author", PlaceID: PlacePrincePondID,
		ContentMode: EchoContentText, Title: "Original", BodyText: "Original body", Status: EchoStatusDraft,
		PublicLocation: Coordinate{Lat: 41.7975, Lon: -88.0153}, CreatedAt: now, UpdatedAt: now,
	}})
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Echoes: store, Clock: fixedClock{now: now.Add(time.Minute)}})

	patched := doJSON(t, handler, http.MethodPatch, "/echoes/echo-draft-1", `{"title":"Updated title","body_text":"Updated body"}`,
		authHeaders("user-author", "author@example.com"), map[string]string{"Idempotency-Key": "echo-patch-1"})
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), "Updated title") {
		t.Fatalf("patch status = %d body=%s", patched.Code, patched.Body.String())
	}

	other := doJSON(t, handler, http.MethodPatch, "/echoes/echo-draft-1", `{"title":"Not allowed"}`,
		authHeaders("user-other", "other@example.com"), map[string]string{"Idempotency-Key": "echo-patch-other"})
	if other.Code != http.StatusForbidden {
		t.Fatalf("other author patch status = %d body=%s", other.Code, other.Body.String())
	}

	deleted := doJSON(t, handler, http.MethodDelete, "/echoes/echo-draft-1", ``,
		authHeaders("user-author", "author@example.com"), map[string]string{"Idempotency-Key": "echo-delete-1"})
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", deleted.Code, deleted.Body.String())
	}
	missing := doJSON(t, handler, http.MethodGet, "/echoes/echo-draft-1", ``, authHeaders("user-author", "author@example.com"))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted echo remains visible: %d %s", missing.Code, missing.Body.String())
	}
}

func TestPublishedEchoCannotBePatchedWithoutRemoderation(t *testing.T) {
	handler := NewHandler(NewStore(DefaultPlaces()))
	response := doJSON(t, handler, http.MethodPatch, "/echoes/echo-active-1", `{"title":"Unsafe bypass"}`,
		authHeaders("user-seed-1", "seed@example.com"), map[string]string{"Idempotency-Key": "published-patch-1"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("published patch status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestFullStitchUsesOwnPlaceProofAndContentLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	profiles := NewMemoryProfileStore()
	if _, err := profiles.UpsertProfile(context.Background(), "user-stitch", "stitch@example.com", UpsertProfileRequest{Handle: "stitch_author", DisplayName: "Stitch Author"}, now); err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{
		Profiles: profiles, Clock: fixedClock{now: now}, CreationSigningKey: []byte("test-creation-key"), PublicLocationKey: []byte("test-location-key"),
	})
	proofPayload := fmt.Sprintf(`{"place_id":%q,"latitude":41.7940943,"longitude":-88.0114616,"accuracy_meters":12,"captured_at":"2026-07-09T17:59:30Z"}`, PlaceLibraryID)
	proof := doJSON(t, handler, http.MethodPost, "/creation-tokens", proofPayload, authHeaders("user-stitch", "stitch@example.com"))
	if proof.Code != http.StatusCreated {
		t.Fatalf("proof status = %d body=%s", proof.Code, proof.Body.String())
	}
	var proofBody struct {
		CreationToken string `json:"creation_token"`
	}
	if err := json.NewDecoder(proof.Body).Decode(&proofBody); err != nil {
		t.Fatal(err)
	}
	requestBody := fmt.Sprintf(`{"place_id":%q,"content_mode":"text","title":"Another angle","body":"This story continued at the library.","creation_token":%q}`, PlaceLibraryID, proofBody.CreationToken)
	created := doJSON(t, handler, http.MethodPost, "/echoes/echo-active-1/stitches", requestBody,
		authHeaders("user-stitch", "stitch@example.com"), map[string]string{"Idempotency-Key": "stitch-create-1"})
	if created.Code != http.StatusCreated {
		t.Fatalf("stitch status = %d body=%s", created.Code, created.Body.String())
	}
	var stitch Stitch
	if err := json.NewDecoder(created.Body).Decode(&stitch); err != nil {
		t.Fatal(err)
	}
	if stitch.SourceEchoID != "echo-active-1" || stitch.Status != EchoStatusDraft || stitch.EchoID == "" {
		t.Fatalf("bad full Stitch: %+v", stitch)
	}
}

func TestSocialMutationIgnoresForgedUserIdentity(t *testing.T) {
	handler := NewHandler(NewStore(DefaultPlaces()))
	response := doJSON(t, handler, http.MethodPut, "/echoes/echo-active-1/reactions", `{"user_id":"forged-user","kind":"heart"}`, authHeaders("real-user", "real@example.com"))
	if response.Code != http.StatusOK {
		t.Fatalf("reaction status = %d body=%s", response.Code, response.Body.String())
	}
	notifications := doJSON(t, handler, http.MethodGet, "/notifications", ``, authHeaders("user-seed-1", "seed@example.com"))
	if notifications.Code != http.StatusOK || !containsJSONValue(notifications.Body.Bytes(), "real-user") || containsJSONValue(notifications.Body.Bytes(), "forged-user") {
		t.Fatalf("notification identity mismatch: %s", notifications.Body.String())
	}
}

func containsJSONValue(data []byte, value string) bool {
	var payload any
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	return valueInJSON(payload, value)
}

func valueInJSON(value any, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case []any:
		for _, item := range typed {
			if valueInJSON(item, target) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if valueInJSON(item, target) {
				return true
			}
		}
	}
	return false
}
