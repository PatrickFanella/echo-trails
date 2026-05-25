package main

import "time"

type EchoStatus string

const (
	EchoStatusDraft     EchoStatus = "draft"
	EchoStatusPublished EchoStatus = "published"
	maxTitleLen                    = 80
	maxBodyLen                     = 500
)

type Echo struct {
	ID        string
	AuthorID  string
	Title     string
	Body      string
	Status    EchoStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	Lat       float64
	Lon       float64
}

type createEchoRequest struct {
	Title string  `json:"title"`
	Body  string  `json:"body"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

type echoResponse struct {
	ID             string     `json:"id"`
	AuthorID       string     `json:"author_id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	Status         EchoStatus `json:"status"`
	PublicLocation location   `json:"public_location"`
	ExactLocation  *location  `json:"exact_location,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func publicLocation(lat, lon float64) location {
	return location{Lat: roundTo(lat, 2), Lon: roundTo(lon, 2)}
}

func roundTo(v float64, places int) float64 {
	factor := 1.0
	for i := 0; i < places; i++ {
		factor *= 10
	}
	if v >= 0 {
		return float64(int(v*factor+0.5)) / factor
	}
	return float64(int(v*factor-0.5)) / factor
}
