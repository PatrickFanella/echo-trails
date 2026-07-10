package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type fixtureObjectReader struct{ data []byte }

func (r fixtureObjectReader) GetObject(context.Context, string, string, int64) ([]byte, error) {
	return append([]byte(nil), r.data...), nil
}

type fixtureToolRunner struct{ calls [][]string }

func (r *fixtureToolRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if name == "probe" {
		return []byte(`{"format":{"duration":"0"},"streams":[{"width":640,"height":480}]}`), nil
	}
	if name == "image" {
		output := args[len(args)-1]
		return nil, os.WriteFile(output, []byte{0xff, 0xd8, 0xff, 0xdb, 0xff, 0xd9}, 0600)
	}
	return nil, nil
}

type silentVideoToolRunner struct{ calls [][]string }

func (r *silentVideoToolRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if name == "probe" {
		path := args[len(args)-1]
		if strings.HasSuffix(path, "input.mp4") {
			return []byte(`{"format":{"duration":"1.0"},"streams":[{"codec_type":"video","width":640,"height":360}]}`), nil
		}
		return []byte(`{"format":{"duration":"1.0"},"streams":[{"codec_type":"video","width":640,"height":360},{"codec_type":"audio"}]}`), nil
	}
	if name == "ffmpeg" {
		output := args[len(args)-1]
		data := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
		if strings.HasSuffix(output, ".jpg") {
			data = []byte{0xff, 0xd8, 0xff, 0xdb, 0xff, 0xd9}
		} else if strings.HasSuffix(output, ".pcm") {
			data = []byte{0, 0, 1, 0}
		}
		return nil, os.WriteFile(output, data, 0600)
	}
	return nil, nil
}

func TestCommandPipelineVerifiesAndStripsImageMetadata(t *testing.T) {
	runner := &fixtureToolRunner{}
	pipeline, err := NewCommandPipeline(fixtureObjectReader{data: []byte{0xff, 0xd8, 0xff, 0xe1, 0x45, 0x78, 0x69, 0x66, 0, 0, 1, 2}}, runner, "ffmpeg", "probe", "image", "malware")
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Process(context.Background(), Asset{ID: "asset", StorageBucket: "private", ObjectKey: "original", MediaType: "image", ContentType: "image/jpeg", Status: AssetStatusUploaded, ByteSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SHA256) != 32 || len(result.Derivatives) != 2 || result.Width != 640 || result.Height != 480 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, derivative := range result.Derivatives {
		if len(derivative.Data) == 0 || !derivative.MetadataSafe {
			t.Fatalf("unsafe derivative: %+v", derivative)
		}
	}
	joined := ""
	for _, call := range runner.calls {
		joined += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(joined, "malware --stream --no-summary") || strings.Count(joined, "-strip") < 2 {
		t.Fatalf("required safety commands missing:\n%s", joined)
	}
}

func TestCommandPipelineRejectsSpoofedSignatureBeforeTools(t *testing.T) {
	runner := &fixtureToolRunner{}
	pipeline, _ := NewCommandPipeline(fixtureObjectReader{data: []byte("not a jpeg file")}, runner, "ffmpeg", "probe", "image", "malware")
	_, err := pipeline.Process(context.Background(), Asset{ID: "asset", StorageBucket: "private", ObjectKey: "original", MediaType: "image", ContentType: "image/jpeg", Status: AssetStatusUploaded, ByteSize: 15})
	if err == nil || len(runner.calls) != 0 {
		t.Fatalf("spoofed object was processed: err=%v calls=%v", err, runner.calls)
	}
}

func TestCommandPipelineAddsAACTrackToSilentVideo(t *testing.T) {
	original := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	runner := &silentVideoToolRunner{}
	pipeline, err := NewCommandPipeline(fixtureObjectReader{data: original}, runner, "ffmpeg", "probe", "image", "malware")
	if err != nil {
		t.Fatal(err)
	}
	result, err := pipeline.Process(context.Background(), Asset{ID: "silent-video", StorageBucket: "private", ObjectKey: "original", MediaType: "video", ContentType: "video/mp4", Status: AssetStatusUploaded, ByteSize: int64(len(original))})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range runner.calls {
		joined += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(joined, "anullsrc=channel_layout=stereo:sample_rate=48000") || !strings.Contains(joined, "-map 1:a:0 -shortest") {
		t.Fatalf("silent AAC normalization missing:\n%s", joined)
	}
	if result.DurationMS != 1000 || len(result.Derivatives) != 3 {
		t.Fatalf("unexpected silent video result: %+v", result)
	}
}

func TestHTTPMediaServicesRequiresModelEvidenceAndSupportsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing service auth")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/v1/moderate-text" {
			var request map[string]string
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request["echo_id"] != "echo-1" {
				t.Errorf("bad text request: %v", request)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "risk_level": "low", "categories": map[string]bool{}, "models": []string{"ShieldGemma"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "risk_level": "low", "categories": map[string]bool{}})
	}))
	defer server.Close()
	services, err := NewHTTPMediaServices(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services.Scan(context.Background(), Asset{ID: "asset"}); err == nil {
		t.Fatal("model-free moderation response was accepted")
	}
	review, err := services.ScanText(context.Background(), EchoText{EchoID: "echo-1", Title: "A story"})
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != ModerationStatusApproved || review.ScannerName != "ShieldGemma" {
		t.Fatalf("bad review: %+v", review)
	}
}
