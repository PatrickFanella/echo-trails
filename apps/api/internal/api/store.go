package api

import (
	"strings"
	"sync"
)

type PlacesStore struct {
	mu         sync.RWMutex
	places     map[string]Place
	placeAlias map[string]string
}

func NewStore(initial []Place) *PlacesStore {
	s := &PlacesStore{places: map[string]Place{}, placeAlias: map[string]string{}}
	for _, p := range initial {
		s.places[p.ID] = p
		for _, aliasID := range p.AliasIDs {
			s.placeAlias[aliasID] = p.ID
		}
	}
	return s
}

func DefaultPlaces() []Place {
	lat1, lon1 := 37.7749, -122.4194
	lat2, lon2 := 37.7793, -122.4192
	lat3, lon3 := 37.7810, -122.4170
	return []Place{
		{ID: "place-active-1", Name: "City Museum", State: PlaceStateActive, Latitude: &lat1, Longitude: &lon1, Aliases: []string{"Museum of the City"}, AliasIDs: []string{"alias-city-museum"}, ProviderMappings: []ProviderMapping{{Provider: "google", ExternalID: "g-100"}}},
		{ID: "place-restricted-1", Name: "Private Club", State: PlaceStateRestricted, Latitude: &lat2, Longitude: &lon2, Aliases: []string{"Members Club"}, ProviderMappings: []ProviderMapping{{Provider: "apple", ExternalID: "a-200"}}},
		{ID: "place-pending-1", Name: "Future Plaza", State: PlaceStatePendingReview, Latitude: &lat3, Longitude: &lon3},
		{ID: "place-rejected-1", Name: "Rejected Cafe", State: PlaceStateRejected, RejectionReason: "duplicate", ReplacementID: "place-active-1"},
		{ID: "place-merged-1", Name: "Old Museum", State: PlaceStateMerged, CanonicalID: "place-active-1"},
		{ID: "place-removed-1", Name: "Removed Diner", State: PlaceStateRemoved},
	}
}

func (s *PlacesStore) Search(query string, includeRestricted bool) []Place {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Place
	for _, p := range s.places {
		if p.State != PlaceStateActive && p.State != PlaceStateRestricted {
			continue
		}
		if p.State == PlaceStateRestricted && !includeRestricted {
			continue
		}
		if q == "" || matchesPlace(p, q) {
			out = append(out, p)
		}
	}
	return out
}

func matchesPlace(p Place, q string) bool {
	if strings.Contains(strings.ToLower(p.Name), q) {
		return true
	}
	for _, a := range p.Aliases {
		if strings.Contains(strings.ToLower(a), q) {
			return true
		}
	}
	for _, m := range p.ProviderMappings {
		if strings.Contains(strings.ToLower(m.Provider), q) || strings.Contains(strings.ToLower(m.ExternalID), q) {
			return true
		}
	}
	return false
}

func (s *PlacesStore) Get(id string) (Place, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if targetID, ok := s.placeAlias[id]; ok {
		return Place{ID: id, State: PlaceStateMerged, CanonicalID: targetID}, true
	}
	p, ok := s.places[id]
	return p, ok
}

func (s *PlacesStore) CreateRequest(req PlaceRequest) Place {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "request-" + slug(req.Name)
	p := Place{ID: id, Name: req.Name, State: PlaceStatePendingReview, Aliases: filterEmpty([]string{req.Alias}), ProviderMappings: filterMappings(req.Provider, req.ExternalID), Latitude: req.Latitude, Longitude: req.Longitude}
	s.places[id] = p
	return p
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return strings.Trim(s, "-")
}

func filterEmpty(in []string) []string {
	var out []string
	for _, v := range in {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
func filterMappings(provider, external string) []ProviderMapping {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(external) == "" {
		return nil
	}
	return []ProviderMapping{{Provider: provider, ExternalID: external}}
}
