package api

import "time"

type PlaceState string

const (
	PlaceStateActive        PlaceState = "active"
	PlaceStatePendingReview PlaceState = "pending_review"
	PlaceStateMerged        PlaceState = "merged"
	PlaceStateRestricted    PlaceState = "restricted"
	PlaceStateRemoved       PlaceState = "removed"
	PlaceStateRejected      PlaceState = "rejected"
)

type ProviderMapping struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type Place struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	State            PlaceState        `json:"state"`
	Latitude         *float64          `json:"latitude,omitempty"`
	Longitude        *float64          `json:"longitude,omitempty"`
	Aliases          []string          `json:"aliases,omitempty"`
	AliasIDs         []string          `json:"alias_ids,omitempty"`
	ProviderMappings []ProviderMapping `json:"provider_mappings,omitempty"`
	CanonicalID      string            `json:"canonical_id,omitempty"`
	ReplacementID    string            `json:"replacement_id,omitempty"`
	RejectionReason  string            `json:"rejection_reason,omitempty"`
}

type PlaceRequest struct {
	Name       string   `json:"name"`
	Alias      string   `json:"alias,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	ExternalID string   `json:"external_id,omitempty"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
}

type EchoSummary struct {
	ID          string    `json:"id"`
	PlaceID     string    `json:"place_id"`
	AuthorID    string    `json:"author_id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Status      string    `json:"status"`
	PublishedAt time.Time `json:"published_at"`
}
