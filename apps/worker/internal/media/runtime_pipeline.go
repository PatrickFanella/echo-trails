package media

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const DerivativeTranscode = "transcode"

type ObjectReader interface {
	GetObject(context.Context, string, string, int64) ([]byte, error)
}

type ToolRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecToolRunner struct{}

func (ExecToolRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 500 {
			message = message[:500]
		}
		return output, fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, message)
	}
	return output, nil
}

type CommandPipeline struct {
	Objects        ObjectReader
	Runner         ToolRunner
	FFmpeg         string
	FFprobe        string
	ImageMagick    string
	MalwareScanner string
}

func NewCommandPipeline(objects ObjectReader, runner ToolRunner, ffmpeg, ffprobe, imageMagick, malwareScanner string) (*CommandPipeline, error) {
	if objects == nil || runner == nil || strings.TrimSpace(ffmpeg) == "" || strings.TrimSpace(ffprobe) == "" || strings.TrimSpace(imageMagick) == "" || strings.TrimSpace(malwareScanner) == "" {
		return nil, ErrProcessorMisconfigured
	}
	return &CommandPipeline{Objects: objects, Runner: runner, FFmpeg: ffmpeg, FFprobe: ffprobe, ImageMagick: imageMagick, MalwareScanner: malwareScanner}, nil
}

func (p *CommandPipeline) Process(ctx context.Context, asset Asset) (Result, error) {
	if _, err := BuildResult(asset); err != nil {
		return Result{}, err
	}
	maxBytes := asset.ByteSize
	if maxBytes <= 0 || maxBytes > 100*1024*1024 {
		maxBytes = 100 * 1024 * 1024
	}
	data, err := p.Objects.GetObject(ctx, asset.StorageBucket, asset.ObjectKey, maxBytes)
	if err != nil {
		return Result{}, err
	}
	if asset.ByteSize > 0 && int64(len(data)) != asset.ByteSize {
		return Result{}, errors.New("object size does not match upload record")
	}
	if !signatureMatches(asset.ContentType, data) {
		return Result{}, errors.New("object signature does not match declared content type")
	}

	directory, err := os.MkdirTemp("", "echotrails-media-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(directory)
	input := filepath.Join(directory, "input"+extensionFor(asset.ContentType))
	if err = os.WriteFile(input, data, 0600); err != nil {
		return Result{}, err
	}
	if _, err = p.Runner.Run(ctx, p.MalwareScanner, "--stream", "--no-summary", input); err != nil {
		return Result{}, fmt.Errorf("malware scan did not return clean: %w", err)
	}

	digest := sha256.Sum256(data)
	result := Result{
		AssetID: asset.ID, Status: AssetStatusPendingReview, MetadataStripped: true,
		SHA256: digest[:], VerifiedContentType: detectedContentType(asset.ContentType, data), MalwareScanStatus: "clean",
	}
	switch asset.MediaType {
	case "image":
		err = p.processImage(ctx, asset, directory, input, &result)
	case "audio":
		err = p.processAudio(ctx, asset, directory, input, &result)
	case "video":
		err = p.processVideo(ctx, asset, directory, input, &result)
	default:
		err = ErrUnsupportedMedia
	}
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func detectedContentType(declared string, data []byte) string {
	detected := http.DetectContentType(data)
	if strings.HasPrefix(detected, "image/jpeg") {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return strings.ToLower(strings.TrimSpace(declared))
	}
	return strings.Split(detected, ";")[0]
}

func (p *CommandPipeline) processImage(ctx context.Context, asset Asset, directory, input string, result *Result) error {
	preview := filepath.Join(directory, "preview.jpg")
	thumbnail := filepath.Join(directory, "thumbnail.jpg")
	if _, err := p.Runner.Run(ctx, p.ImageMagick, input, "-auto-orient", "-strip", "-resize", "1920x1920>", "-quality", "88", preview); err != nil {
		return err
	}
	if _, err := p.Runner.Run(ctx, p.ImageMagick, preview, "-strip", "-resize", "480x480>", "-quality", "82", thumbnail); err != nil {
		return err
	}
	probe, err := p.probe(ctx, preview)
	if err != nil {
		return err
	}
	result.Width, result.Height = probe.Width, probe.Height
	return appendFiles(result, asset, []fileDerivative{{DerivativePreview, "image/jpeg", preview}, {DerivativeThumbnail, "image/jpeg", thumbnail}})
}

func (p *CommandPipeline) processAudio(ctx context.Context, asset Asset, directory, input string, result *Result) error {
	transcode := filepath.Join(directory, "audio.m4a")
	pcm := filepath.Join(directory, "waveform.pcm")
	inputProbe, err := p.probe(ctx, input)
	if err != nil {
		return err
	}
	if inputProbe.DurationMS > 120000 {
		return errors.New("audio duration exceeds two minutes")
	}
	if _, err := p.Runner.Run(ctx, p.FFmpeg, "-v", "error", "-y", "-i", input, "-map_metadata", "-1", "-vn", "-c:a", "aac", "-b:a", "128k", transcode); err != nil {
		return err
	}
	probe, err := p.probe(ctx, transcode)
	if err != nil {
		return err
	}
	if probe.DurationMS > 120000 {
		return errors.New("audio duration exceeds two minutes")
	}
	result.DurationMS = probe.DurationMS
	if _, err = p.Runner.Run(ctx, p.FFmpeg, "-v", "error", "-y", "-i", transcode, "-ac", "1", "-ar", "8000", "-f", "s16le", pcm); err != nil {
		return err
	}
	waveform, err := waveformJSON(pcm)
	if err != nil {
		return err
	}
	result.Derivatives = append(result.Derivatives, Derivative{MediaAssetID: asset.ID, DerivativeType: DerivativeWaveform, ObjectKey: derivativeKey(asset.ID, DerivativeWaveform, "json"), ContentType: "application/json", MetadataSafe: true, Data: waveform})
	return appendFiles(result, asset, []fileDerivative{{DerivativeTranscode, "audio/mp4", transcode}})
}

func (p *CommandPipeline) processVideo(ctx context.Context, asset Asset, directory, input string, result *Result) error {
	transcode := filepath.Join(directory, "video.mp4")
	thumbnail := filepath.Join(directory, "thumbnail.jpg")
	pcm := filepath.Join(directory, "waveform.pcm")
	filter := "scale=1920:1080:force_original_aspect_ratio=decrease:force_divisible_by=2"
	inputProbe, err := p.probe(ctx, input)
	if err != nil {
		return err
	}
	if inputProbe.DurationMS > 60000 {
		return errors.New("video duration exceeds one minute")
	}
	arguments := []string{"-v", "error", "-y", "-i", input}
	if inputProbe.HasAudio {
		arguments = append(arguments, "-map", "0:v:0", "-map", "0:a:0")
	} else {
		// Silent picker videos still normalize to the release H.264/AAC contract.
		arguments = append(arguments, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000", "-map", "0:v:0", "-map", "1:a:0", "-shortest")
	}
	arguments = append(arguments, "-map_metadata", "-1", "-vf", filter, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "medium", "-movflags", "+faststart", "-c:a", "aac", "-b:a", "128k", transcode)
	if _, err := p.Runner.Run(ctx, p.FFmpeg, arguments...); err != nil {
		return err
	}
	probe, err := p.probe(ctx, transcode)
	if err != nil {
		return err
	}
	if probe.DurationMS > 60000 {
		return errors.New("video duration exceeds one minute")
	}
	result.DurationMS, result.Width, result.Height = probe.DurationMS, probe.Width, probe.Height
	if _, err = p.Runner.Run(ctx, p.FFmpeg, "-v", "error", "-y", "-ss", "1", "-i", transcode, "-frames:v", "1", "-map_metadata", "-1", thumbnail); err != nil {
		return err
	}
	if _, err = p.Runner.Run(ctx, p.FFmpeg, "-v", "error", "-y", "-i", transcode, "-ac", "1", "-ar", "8000", "-f", "s16le", pcm); err != nil {
		return err
	}
	waveform, err := waveformJSON(pcm)
	if err != nil {
		return err
	}
	result.Derivatives = append(result.Derivatives, Derivative{MediaAssetID: asset.ID, DerivativeType: DerivativeWaveform, ObjectKey: derivativeKey(asset.ID, DerivativeWaveform, "json"), ContentType: "application/json", MetadataSafe: true, Data: waveform})
	return appendFiles(result, asset, []fileDerivative{{DerivativePreview, "video/mp4", transcode}, {DerivativeThumbnail, "image/jpeg", thumbnail}})
}

type mediaProbe struct {
	DurationMS, Width, Height int
	HasAudio                  bool
}

func (p *CommandPipeline) probe(ctx context.Context, path string) (mediaProbe, error) {
	output, err := p.Runner.Run(ctx, p.FFprobe, "-v", "error", "-show_entries", "format=duration:stream=codec_type,width,height", "-of", "json", path)
	if err != nil {
		return mediaProbe{}, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err = json.Unmarshal(output, &payload); err != nil {
		return mediaProbe{}, err
	}
	duration, _ := strconv.ParseFloat(payload.Format.Duration, 64)
	probe := mediaProbe{DurationMS: int(math.Round(duration * 1000))}
	for _, stream := range payload.Streams {
		if stream.CodecType == "audio" {
			probe.HasAudio = true
		}
		if stream.Width > 0 && stream.Height > 0 {
			probe.Width, probe.Height = stream.Width, stream.Height
			break
		}
	}
	return probe, nil
}

type fileDerivative struct{ kind, contentType, path string }

func appendFiles(result *Result, asset Asset, files []fileDerivative) error {
	for _, file := range files {
		data, err := os.ReadFile(file.path)
		if err != nil {
			return err
		}
		ext := strings.TrimPrefix(filepath.Ext(file.path), ".")
		result.Derivatives = append(result.Derivatives, Derivative{MediaAssetID: asset.ID, DerivativeType: file.kind, ObjectKey: derivativeKey(asset.ID, file.kind, ext), ContentType: file.contentType, MetadataSafe: true, Data: data})
	}
	return nil
}

func derivativeKey(assetID, kind, extension string) string {
	return fmt.Sprintf("media/derivatives/%s/%s.%s", assetID, kind, extension)
}

func waveformJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 2 {
		return json.Marshal(map[string]any{"version": 1, "peaks": []float64{}})
	}
	samples, bins := len(data)/2, 100
	if samples < bins {
		bins = samples
	}
	peaks := make([]float64, bins)
	for i := 0; i < samples; i++ {
		value := math.Abs(float64(int16(binary.LittleEndian.Uint16(data[i*2:i*2+2])))) / 32768
		bin := i * bins / samples
		if value > peaks[bin] {
			peaks[bin] = math.Round(value*1000) / 1000
		}
	}
	return json.Marshal(map[string]any{"version": 1, "peaks": peaks})
}

func signatureMatches(contentType string, data []byte) bool {
	if len(data) < 12 {
		return false
	}
	switch contentType {
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case "image/png":
		return string(data[:8]) == "\x89PNG\r\n\x1a\n"
	case "image/heic", "audio/mp4", "video/mp4", "video/quicktime":
		return string(data[4:8]) == "ftyp"
	case "audio/mpeg":
		return (data[0] == 'I' && data[1] == 'D' && data[2] == '3') || (data[0] == 0xff && data[1]&0xe0 == 0xe0)
	case "audio/wav":
		return string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE"
	default:
		return strings.HasPrefix(http.DetectContentType(data[:min(len(data), 512)]), contentType)
	}
}

func extensionFor(contentType string) string {
	return map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/heic": ".heic", "audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/wav": ".wav", "video/mp4": ".mp4", "video/quicktime": ".mov"}[contentType]
}
