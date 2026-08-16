package api

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
)

type PlaceStore interface {
	SearchPlaces(context.Context, string, bool, *float64, *float64) ([]Place, error)
	NearbyPlaces(context.Context, float64, float64, int, bool) ([]Place, error)
	GetPlace(context.Context, string) (Place, bool, error)
	AllPlaces(context.Context) ([]Place, error)
	CreatePlaceRequest(context.Context, string, PlaceRequest) (Place, error)
}

type PlacesStore struct {
	mu         sync.RWMutex
	places     map[string]Place
	placeAlias map[string]string
}

const (
	PlacePrincePondID       = "019f486a-0001-7000-8000-000000000001"
	PlaceMainStreetMetraID  = "019f486a-0002-7000-8000-000000000002"
	PlaceLibraryID          = "019f486a-0003-7000-8000-000000000003"
	PlaceTivoliTheatreID    = "019f486a-0004-7000-8000-000000000004"
	PlaceFishelParkID       = "019f486a-0005-7000-8000-000000000005"
	PlaceWashingtonParkID   = "019f486a-0006-7000-8000-000000000006"
	PlaceConstitutionParkID = "019f486a-0007-7000-8000-000000000007"
	PlaceMuseumID           = "019f486a-0008-7000-8000-000000000008"
	PlaceNorthHighSchoolID  = "019f486a-0009-7000-8000-000000000009"
)

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
	coordinate := func(lat, lon float64) (*float64, *float64) { return &lat, &lon }
	princeLat, princeLon := coordinate(41.7975912, -88.0153297)
	metraLat, metraLon := coordinate(41.7955440, -88.0086302)
	libraryLat, libraryLon := coordinate(41.7940943, -88.0114616)
	tivoliLat, tivoliLon := coordinate(41.7960844, -88.0088001)
	fishelLat, fishelLon := coordinate(41.7928667, -88.0116448)
	washingtonLat, washingtonLon := coordinate(41.7992530, -88.0057961)
	constitutionLat, constitutionLon := coordinate(41.7907635, -88.0089976)
	museumLat, museumLon := coordinate(41.7925048, -88.0051168)
	highSchoolLat, highSchoolLon := coordinate(41.8065417, -88.0122581)
	return []Place{
		{ID: PlacePrincePondID, Name: "Prince Pond", State: PlaceStateActive, Latitude: princeLat, Longitude: princeLon, RadiusMeters: 100, Category: "landmark", Aliases: []string{"Prince Pond Park"}, AliasIDs: []string{"alias-prince-pond"}, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/35362323"}}},
		{ID: PlaceMainStreetMetraID, Name: "Downers Grove Main Street Metra", State: PlaceStateActive, Latitude: metraLat, Longitude: metraLon, RadiusMeters: 100, Category: "transit_station", Aliases: []string{"Main Street Station", "Downers Grove Metra"}, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "node/25779937"}}},
		{ID: PlaceLibraryID, Name: "Downers Grove Public Library", State: PlaceStateActive, Latitude: libraryLat, Longitude: libraryLon, RadiusMeters: 75, Category: "library", Aliases: []string{"DGPL"}, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/200192088"}}},
		{ID: PlaceTivoliTheatreID, Name: "Tivoli Theatre", State: PlaceStateActive, Latitude: tivoliLat, Longitude: tivoliLon, RadiusMeters: 75, Category: "theatre", ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/200218663"}}},
		{ID: PlaceFishelParkID, Name: "Fishel Park", State: PlaceStateActive, Latitude: fishelLat, Longitude: fishelLon, RadiusMeters: 150, Category: "park", ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/200054182"}}},
		{ID: PlaceWashingtonParkID, Name: "Washington Park", State: PlaceStateActive, Latitude: washingtonLat, Longitude: washingtonLon, RadiusMeters: 200, Category: "park", ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/552662784"}}},
		{ID: PlaceConstitutionParkID, Name: "Constitution Park and Lincoln Center", State: PlaceStateActive, Latitude: constitutionLat, Longitude: constitutionLon, RadiusMeters: 200, Category: "park", Aliases: []string{"Constitution Park", "Lincoln Center"}, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/567113043"}}},
		{ID: PlaceMuseumID, Name: "Downers Grove Museum", State: PlaceStateActive, Latitude: museumLat, Longitude: museumLon, RadiusMeters: 100, Category: "museum", Aliases: []string{"Wandschneider Park"}, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/684600471"}}},
		{ID: PlaceNorthHighSchoolID, Name: "Downers Grove North High School", State: PlaceStateRestricted, Latitude: highSchoolLat, Longitude: highSchoolLon, RadiusMeters: 250, Category: "school", Sensitive: true, ProviderMappings: []ProviderMapping{{Provider: "openstreetmap", ExternalID: "way/548571172"}}},
		{ID: "place-pending-1", Name: "Future Plaza", State: PlaceStatePendingReview, RadiusMeters: 100},
		{ID: "place-rejected-1", Name: "Rejected Place", State: PlaceStateRejected, RejectionReason: "duplicate", ReplacementID: PlacePrincePondID},
		{ID: "place-merged-1", Name: "Prince Pond Playground", State: PlaceStateMerged, CanonicalID: PlacePrincePondID},
		{ID: "place-removed-1", Name: "Removed Diner", State: PlaceStateRemoved},
	}
}

func (s *PlacesStore) SearchPlaces(_ context.Context, query string, includeRestricted bool, _, _ *float64) ([]Place, error) {
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
	sortPlaces(out)
	return out, nil
}

func (s *PlacesStore) NearbyPlaces(_ context.Context, lat, lon float64, radiusMeters int, includeRestricted bool) ([]Place, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Place
	for _, p := range s.places {
		if p.Latitude == nil || p.Longitude == nil {
			continue
		}
		if p.State != PlaceStateActive && p.State != PlaceStateRestricted {
			continue
		}
		if p.State == PlaceStateRestricted && !includeRestricted {
			continue
		}
		if distanceMeters(lat, lon, *p.Latitude, *p.Longitude) <= float64(radiusMeters) {
			out = append(out, p)
		}
	}
	sortPlaces(out)
	return out, nil
}

func sortPlaces(places []Place) {
	sort.Slice(places, func(i, j int) bool { return places[i].ID < places[j].ID })
}

func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	rLat1 := lat1 * math.Pi / 180
	rLat2 := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusMeters * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
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

func (s *PlacesStore) GetPlace(_ context.Context, id string) (Place, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if targetID, ok := s.placeAlias[id]; ok {
		return Place{ID: id, State: PlaceStateMerged, CanonicalID: targetID}, true, nil
	}
	p, ok := s.places[id]
	return p, ok, nil
}

func (s *PlacesStore) AllPlaces(_ context.Context) ([]Place, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Place, 0, len(s.places))
	for _, place := range s.places {
		result = append(result, place)
	}
	return result, nil
}

func (s *PlacesStore) CreatePlaceRequest(_ context.Context, _ string, req PlaceRequest) (Place, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "request-" + slug(req.Name)
	p := Place{ID: id, Name: req.Name, State: PlaceStatePendingReview, Aliases: filterEmpty([]string{req.Alias}), ProviderMappings: filterMappings(req.Provider, req.ExternalID), Latitude: req.Latitude, Longitude: req.Longitude}
	s.places[id] = p
	return p, nil
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
