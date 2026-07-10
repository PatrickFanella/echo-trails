package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscriptionTreatsNoSpeechAsSuccessfulAccessibilityResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/transcribe" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"","confidence":1,"locale":"und","model":"faster-whisper:large-v3-turbo@revision"}`))
	}))
	defer server.Close()
	services, err := NewHTTPMediaServices(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := services.Transcribe(context.Background(), Asset{ID: "asset", MediaType: "video", ContentType: "video/mp4", StorageBucket: "private", ObjectKey: "original.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Status != TranscriptStatusSucceeded || transcript.Text != "No speech detected." {
		t.Fatalf("unexpected transcript: %+v", transcript)
	}
}
