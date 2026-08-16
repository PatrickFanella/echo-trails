package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
)

func (h *Handler) handleDiscoverySearch(w http.ResponseWriter, r *http.Request) {
	latitude, longitude, err := optionalCoordinates(r)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	viewerID, _, _ := authenticatedUser(r)
	response, err := h.discovery.SearchDiscovery(r.Context(), SearchRequest{
		Query: r.URL.Query().Get("q"), Scope: r.URL.Query().Get("scope"), PlaceID: r.URL.Query().Get("place_id"),
		Latitude: latitude, Longitude: longitude, RadiusMeters: parseInt(r.URL.Query().Get("radius_meters"), 1000),
		ViewerID: viewerID, Cursor: r.URL.Query().Get("cursor"), Limit: parseInt(r.URL.Query().Get("limit"), 20),
	})
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleNotable(w http.ResponseWriter, r *http.Request) {
	latitude, longitude, err := optionalCoordinates(r)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	viewerID, _, _ := authenticatedUser(r)
	results, err := h.discovery.NotableEchoes(r.Context(), strings.TrimSpace(r.URL.Query().Get("place_id")), latitude, longitude,
		parseInt(r.URL.Query().Get("radius_meters"), 1000), viewerID, parseInt(r.URL.Query().Get("limit"), 20))
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"echoes": results})
}

func optionalCoordinates(r *http.Request) (*float64, *float64, error) {
	rawLatitude, rawLongitude := r.URL.Query().Get("lat"), r.URL.Query().Get("lon")
	if rawLatitude == "" && rawLongitude == "" {
		return nil, nil, nil
	}
	latitude, latitudeErr := strconv.ParseFloat(rawLatitude, 64)
	longitude, longitudeErr := strconv.ParseFloat(rawLongitude, 64)
	if latitudeErr != nil || longitudeErr != nil || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return nil, nil, ErrDiscoveryInvalid
	}
	return &latitude, &longitude, nil
}

func writeDiscoveryError(w http.ResponseWriter, err error) {
	if errors.Is(err, pagination.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "invalid pagination cursor")
		return
	}
	if errors.Is(err, ErrDiscoveryInvalid) {
		writeError(w, http.StatusBadRequest, "invalid_discovery_request", "invalid discovery request")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}
