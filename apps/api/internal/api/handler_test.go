package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchFiltersAndMatchesAliasesAndMappings(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	req := httptest.NewRequest(http.MethodGet, "/places/search?q=museum", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Places []Place `json:"places"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Places) != 1 || out.Places[0].ID != "place-active-1" {
		t.Fatalf("unexpected result: %+v", out.Places)
	}

	req = httptest.NewRequest(http.MethodGet, "/places/search?query=g-100", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Places) != 1 || out.Places[0].ID != "place-active-1" {
		t.Fatalf("alias mapping search failed: %+v", out.Places)
	}

	req = httptest.NewRequest(http.MethodGet, "/places/search?q=private", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Places) != 0 {
		t.Fatalf("restricted should be excluded")
	}

	req = httptest.NewRequest(http.MethodGet, "/places/search?q=private&include_restricted=true", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Places) != 1 || out.Places[0].ID != "place-restricted-1" {
		t.Fatalf("restricted include failed: %+v", out.Places)
	}
}

func TestDetailRedirectRejectAndActive(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/place-merged-1", nil))
	if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "/places/place-active-1" {
		t.Fatalf("merged redirect failed: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/alias-city-museum", nil))
	if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "/places/place-active-1" {
		t.Fatalf("alias redirect failed: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/place-rejected-1", nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("rejected status %d", rec.Code)
	}
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got["state"] != string(PlaceStateRejected) {
		t.Fatalf("rejected payload missing")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/place-active-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("active status %d", rec.Code)
	}
}

func TestCreateRequestPendingReview(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	body, _ := json.Marshal(PlaceRequest{Name: "New Place", Alias: "Fresh Spot", Provider: "google", ExternalID: "g-999"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/places/requests", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d", rec.Code)
	}
	var p Place
	_ = json.NewDecoder(rec.Body).Decode(&p)
	if p.State != PlaceStatePendingReview || p.Name != "New Place" || len(p.Aliases) != 1 || len(p.ProviderMappings) != 1 {
		t.Fatalf("bad request response: %+v", p)
	}
}

func TestNearbyReturnsActivePublicPlaces(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/discovery/nearby?lat=37.7749&lon=-122.4194&radius_meters=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Places []Place `json:"places"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Places) != 1 || out.Places[0].ID != "place-active-1" {
		t.Fatalf("unexpected nearby result: %+v", out.Places)
	}
}

func TestNearbyRejectsBadCoordinatesAndRadius(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	for _, path := range []string{
		"/discovery/nearby?lat=nope&lon=-122.4",
		"/discovery/nearby?lat=37.7&lon=-122.4&radius_meters=1000000",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status %d", path, rec.Code)
		}
	}
}

func TestPlaceEchoesReturnsPublishedEchoesOnlyForActivePlace(t *testing.T) {
	h := NewHandler(NewStore(DefaultPlaces()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/place-active-1/echoes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Echoes []EchoSummary `json:"echoes"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out.Echoes) != 1 || out.Echoes[0].Status != "published" {
		t.Fatalf("unexpected echoes: %+v", out.Echoes)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/places/place-restricted-1/echoes", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("restricted place echoes status %d", rec.Code)
	}
}
