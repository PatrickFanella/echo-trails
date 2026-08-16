package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type serviceAssetRequest struct {
	AssetID     string `json:"asset_id"`
	Bucket      string `json:"bucket"`
	ObjectKey   string `json:"object_key"`
	MediaType   string `json:"media_type"`
	ContentType string `json:"content_type"`
}

type moderationServiceResponse struct {
	RiskLevel  string          `json:"risk_level"`
	Status     string          `json:"status"`
	Categories map[string]bool `json:"categories"`
	Models     []string        `json:"models"`
}

type transcriptionServiceResponse struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Locale     string  `json:"locale"`
	Model      string  `json:"model"`
}

type textModerationServiceRequest struct {
	EchoID  string `json:"echo_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Caption string `json:"caption"`
}

type HTTPMediaServices struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPMediaServices(baseURL, token string, client *http.Client) (*HTTPMediaServices, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.TrimSpace(token) == "" {
		return nil, ErrProcessorMisconfigured
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &HTTPMediaServices{baseURL: baseURL, token: token, client: client}, nil
}

func (s *HTTPMediaServices) Scan(ctx context.Context, asset Asset) (ModerationReview, error) {
	var response moderationServiceResponse
	if err := s.post(ctx, "/v1/moderate", asset, &response); err != nil {
		return ModerationReview{}, err
	}
	if len(response.Models) == 0 {
		return ModerationReview{}, errors.New("moderation response did not identify models")
	}
	return ModerationReview{AssetID: asset.ID, Status: response.Status, RiskLevel: response.RiskLevel, Categories: response.Categories, ScannerName: strings.Join(response.Models, "+")}, nil
}

func (s *HTTPMediaServices) Transcribe(ctx context.Context, asset Asset) (Transcript, error) {
	var response transcriptionServiceResponse
	if err := s.post(ctx, "/v1/transcribe", asset, &response); err != nil {
		return Transcript{}, err
	}
	if !strings.Contains(strings.ToLower(response.Model), "whisper") {
		return Transcript{}, errors.New("transcription response is incomplete")
	}
	text := strings.TrimSpace(response.Text)
	if text == "" {
		// A successful no-speech result is still an accessibility result. Keeping
		// it successful prevents silent video and ambient-audio Echoes from
		// remaining private forever while giving clients meaningful copy.
		text = "No speech detected."
	}
	return Transcript{AssetID: asset.ID, Status: TranscriptStatusSucceeded, Locale: response.Locale, Text: text, Confidence: response.Confidence, ModelName: response.Model}, nil
}

func (s *HTTPMediaServices) ScanText(ctx context.Context, content EchoText) (EchoModerationReview, error) {
	body, err := json.Marshal(textModerationServiceRequest{EchoID: content.EchoID, Title: content.Title, Body: content.Body, Caption: content.Caption})
	if err != nil {
		return EchoModerationReview{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/moderate-text", bytes.NewReader(body))
	if err != nil {
		return EchoModerationReview{}, err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return EchoModerationReview{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return EchoModerationReview{}, fmt.Errorf("text moderation service returned %d", response.StatusCode)
	}
	var output moderationServiceResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&output); err != nil {
		return EchoModerationReview{}, err
	}
	if len(output.Models) == 0 {
		return EchoModerationReview{}, errors.New("text moderation response did not identify models")
	}
	return EchoModerationReview{EchoID: content.EchoID, Status: output.Status, RiskLevel: output.RiskLevel, Categories: output.Categories, ScannerName: strings.Join(output.Models, "+")}, nil
}

func (s *HTTPMediaServices) post(ctx context.Context, path string, asset Asset, output any) error {
	body, err := json.Marshal(serviceAssetRequest{AssetID: asset.ID, Bucket: asset.StorageBucket, ObjectKey: asset.ObjectKey, MediaType: asset.MediaType, ContentType: asset.ContentType})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("media service %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(message)))
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(output)
}
