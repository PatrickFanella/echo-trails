package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	QuoteSnapshotWidth  = 1200
	QuoteSnapshotHeight = 630
)

type QuoteSnapshot struct {
	LinkID             string
	TargetEchoID       string
	OwnerUserID        string
	Body               string
	ReplyAuthorHandle  string
	SourceEchoTitle    string
	SourceAuthorHandle string
}

type RenderedQuoteSnapshot struct {
	QuoteSnapshot
	PNG    []byte
	Width  int
	Height int
}

type QuoteSnapshotCatalog interface {
	LoadQuoteSnapshot(context.Context, string) (QuoteSnapshot, error)
	SaveQuoteSnapshot(context.Context, RenderedQuoteSnapshot) error
	FailQuoteSnapshot(context.Context, string, error) error
}

type QuoteSnapshotProcessor struct{ Catalog QuoteSnapshotCatalog }

func (p QuoteSnapshotProcessor) Process(ctx context.Context, linkID string) (RenderedQuoteSnapshot, error) {
	if p.Catalog == nil {
		return RenderedQuoteSnapshot{}, ErrProcessorMisconfigured
	}
	snapshot, err := p.Catalog.LoadQuoteSnapshot(ctx, linkID)
	if err != nil {
		return RenderedQuoteSnapshot{}, err
	}
	pngBytes, err := RenderQuoteSnapshotPNG(snapshot)
	if err != nil {
		_ = p.Catalog.FailQuoteSnapshot(ctx, linkID, err)
		return RenderedQuoteSnapshot{}, err
	}
	rendered := RenderedQuoteSnapshot{QuoteSnapshot: snapshot, PNG: pngBytes, Width: QuoteSnapshotWidth, Height: QuoteSnapshotHeight}
	if err = p.Catalog.SaveQuoteSnapshot(ctx, rendered); err != nil {
		_ = p.Catalog.FailQuoteSnapshot(ctx, linkID, err)
		return RenderedQuoteSnapshot{}, err
	}
	return rendered, nil
}

func RenderQuoteSnapshotPNG(snapshot QuoteSnapshot) ([]byte, error) {
	body := strings.TrimSpace(snapshot.Body)
	if body == "" || len([]rune(body)) > 500 {
		return nil, errors.New("invalid quote snapshot body")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, QuoteSnapshotWidth, QuoteSnapshotHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 247, G: 244, B: 236, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(52, 52, 66, 578), &image.Uniform{C: color.RGBA{R: 38, G: 91, B: 82, A: 255}}, image.Point{}, draw.Src)

	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	bodyFace, err := opentype.NewFace(regular, &opentype.FaceOptions{Size: 34, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer bodyFace.Close()
	labelFace, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: 23, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer labelFace.Close()

	drawText(canvas, labelFace, color.RGBA{R: 38, G: 91, B: 82, A: 255}, 92, 105, "ECHOTRAILS · REPLY SNAPSHOT")
	y := 185
	for _, line := range wrapText(bodyFace, body, 980) {
		drawText(canvas, bodyFace, color.RGBA{R: 28, G: 35, B: 34, A: 255}, 92, y, line)
		y += 47
		if y > 465 {
			break
		}
	}
	footer := "@" + strings.TrimPrefix(strings.TrimSpace(snapshot.ReplyAuthorHandle), "@")
	if snapshot.SourceEchoTitle != "" {
		footer += "  ·  " + snapshot.SourceEchoTitle
	}
	drawText(canvas, labelFace, color.RGBA{R: 75, G: 82, B: 80, A: 255}, 92, 545, footer)

	var output bytes.Buffer
	if err = png.Encode(&output, canvas); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func drawText(target draw.Image, face font.Face, ink color.Color, x, y int, text string) {
	drawer := font.Drawer{Dst: target, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(x, y)}
	drawer.DrawString(text)
}

func wrapText(face font.Face, text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	lines := []string{}
	line := words[0]
	for _, word := range words[1:] {
		candidate := line + " " + word
		if font.MeasureString(face, candidate).Ceil() <= maxWidth {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = word
	}
	return append(lines, line)
}
