package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrAdminUnauthorized = errors.New("admin authorization required")
	ErrAdminInvalid      = errors.New("invalid admin request")
	ErrAdminNotFound     = errors.New("admin target not found")
)

type AdminSessionRecord struct {
	UserID    string
	Role      string
	ExpiresAt time.Time
}

type AdminQueueItem struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Status     string         `json:"status"`
	Reason     string         `json:"reason,omitempty"`
	TargetType string         `json:"target_type,omitempty"`
	TargetID   string         `json:"target_id,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type AdminQueuePage struct {
	Items      []AdminQueueItem `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type AdminMutationRequest struct {
	Action          string          `json:"action"`
	ReasonCode      string          `json:"reason_code"`
	Notes           string          `json:"notes,omitempty"`
	TargetType      string          `json:"target_type,omitempty"`
	TargetID        string          `json:"target_id,omitempty"`
	ReportID        string          `json:"report_id,omitempty"`
	MergeIntoID     string          `json:"merge_into_place_id,omitempty"`
	BoundaryGeoJSON json.RawMessage `json:"boundary_geojson,omitempty"`
	RadiusMeters    *float64        `json:"radius_meters,omitempty"`
	Confirmed       bool            `json:"confirmed"`
}

type AdminInviteRequest struct {
	Code       string     `json:"code"`
	MaxUses    int        `json:"max_uses"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	ReasonCode string     `json:"reason_code"`
	Confirmed  bool       `json:"confirmed"`
}

type AdminRepository interface {
	CreateAdminSession(context.Context, string, string, string, time.Time) (AdminSessionRecord, error)
	AuthenticateAdminSession(context.Context, string, string, time.Time, bool) (AdminSessionRecord, error)
	RevokeAdminSession(context.Context, string, time.Time) error
	ListAdminQueue(context.Context, string, string, int) (AdminQueuePage, error)
	ApplyAdminMutation(context.Context, AdminSessionRecord, string, AdminMutationRequest, string, time.Time) error
	CreateAdminInvite(context.Context, AdminSessionRecord, AdminInviteRequest, string, time.Time) error
	GetAdminMediaObject(context.Context, AdminSessionRecord, string) (string, error)
}
