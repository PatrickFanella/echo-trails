package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type releaseFixtureReader struct{ data []byte }

func (r releaseFixtureReader) GetObject(context.Context, string, string, int64) ([]byte, error) {
	return append([]byte(nil), r.data...), nil
}

type releaseToolRunner struct{}

func (releaseToolRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "clean-fixture" {
		return nil, nil
	}
	return ExecToolRunner{}.Run(ctx, name, args...)
}

func TestReleaseProcessingLatency(t *testing.T) {
	if os.Getenv("ECHOTRAILS_PROCESSING_LATENCY_TEST") != "1" {
		t.Skip("set ECHOTRAILS_PROCESSING_LATENCY_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	directory := t.TempDir()
	photo := filepath.Join(directory, "photo.jpg")
	audio := filepath.Join(directory, "audio.m4a")
	video := filepath.Join(directory, "video.mp4")
	generateFixture(t, ctx, "ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=1920x1080", "-frames:v", "1", photo)
	generateFixture(t, ctx, "ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=119.5", "-c:a", "aac", "-b:a", "96k", audio)
	generateFixture(t, ctx, "ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=12:duration=59.5", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-t", "59.5", video)

	cases := []struct {
		name, path, mediaType, contentType string
		limit                              time.Duration
	}{
		{"photo", photo, "image", "image/jpeg", 60 * time.Second},
		{"two-minute-audio", audio, "audio", "audio/mp4", 3 * time.Minute},
		{"one-minute-video", video, "video", "video/mp4", 5 * time.Minute},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			pipeline, err := NewCommandPipeline(releaseFixtureReader{data}, releaseToolRunner{}, "ffmpeg", "ffprobe", "convert", "clean-fixture")
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			result, err := pipeline.Process(ctx, Asset{ID: "latency-" + test.name, StorageBucket: "private", ObjectKey: "fixture", MediaType: test.mediaType, ContentType: test.contentType, Status: AssetStatusUploaded, ByteSize: int64(len(data))})
			elapsed := time.Since(started)
			if err != nil {
				t.Fatal(err)
			}
			if elapsed >= test.limit {
				t.Fatalf("processing latency %s exceeds target %s", elapsed, test.limit)
			}
			if len(result.Derivatives) == 0 || !result.MetadataStripped || result.MalwareScanStatus != "clean" {
				t.Fatalf("incomplete processing result: %+v", result)
			}
			t.Logf("RELEASE_EVIDENCE processing kind=%s elapsed=%s target=%s derivatives=%d", test.name, elapsed, test.limit, len(result.Derivatives))
		})
	}
}

func generateFixture(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()
	command := exec.CommandContext(ctx, name, args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, output)
	}
}
