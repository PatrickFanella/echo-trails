package api

import (
	"context"
	"errors"
	"strings"
)

type MemoryDiscoveryRepository struct {
	places PlaceStore
	echoes EchoStore
}

func NewMemoryDiscoveryRepository(places PlaceStore, echoes EchoStore) *MemoryDiscoveryRepository {
	return &MemoryDiscoveryRepository{places: places, echoes: echoes}
}

func (m *MemoryDiscoveryRepository) SearchDiscovery(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	places, err := m.places.SearchPlaces(ctx, request.Query, false, request.Latitude, request.Longitude)
	if err != nil {
		return SearchResponse{}, err
	}
	results := []EchoDiscoveryResult{}
	for _, place := range places {
		echoes, listErr := m.echoes.ListPublishedByPlace(ctx, place.ID, request.Limit)
		if listErr != nil {
			return SearchResponse{}, listErr
		}
		for _, record := range echoes {
			text := strings.ToLower(record.Title + " " + record.BodyText + " " + record.CaptionText)
			if strings.Contains(text, strings.ToLower(strings.TrimSpace(request.Query))) {
				results = append(results, EchoDiscoveryResult{Echo: toEchoResponse(record, request.ViewerID), Score: .5, Label: "Recent"})
			}
		}
	}
	return SearchResponse{Places: places, Echoes: results}, nil
}

func (m *MemoryDiscoveryRepository) NotableEchoes(ctx context.Context, placeID string, _, _ *float64, _ int, viewerID string, limit int) ([]EchoDiscoveryResult, error) {
	if placeID == "" {
		return []EchoDiscoveryResult{}, nil
	}
	echoes, err := m.echoes.ListPublishedByPlace(ctx, placeID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]EchoDiscoveryResult, 0, len(echoes))
	for _, record := range echoes {
		result = append(result, EchoDiscoveryResult{Echo: toEchoResponse(record, viewerID), Score: .5, Label: "Recent"})
	}
	return result, nil
}

var ErrDiscoveryInvalid = errors.New("invalid discovery request")

type EchoDiscoveryResult struct {
	Echo  EchoResponse `json:"echo"`
	Score float64      `json:"score"`
	Label string       `json:"label"`
}

type SearchRequest struct {
	Query        string
	Scope        string
	PlaceID      string
	Latitude     *float64
	Longitude    *float64
	RadiusMeters int
	ViewerID     string
	Cursor       string
	Limit        int
}

type SearchResponse struct {
	Places     []Place               `json:"places"`
	Echoes     []EchoDiscoveryResult `json:"echoes"`
	NextCursor string                `json:"next_cursor"`
}

type DiscoveryRepository interface {
	SearchDiscovery(context.Context, SearchRequest) (SearchResponse, error)
	NotableEchoes(context.Context, string, *float64, *float64, int, string, int) ([]EchoDiscoveryResult, error)
}
