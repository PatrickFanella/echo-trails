package media

import (
	"bytes"
	"image/png"
	"testing"
)

func TestRenderQuoteSnapshotPNGIsCanonicalSize(t *testing.T) {
	data, err := RenderQuoteSnapshotPNG(QuoteSnapshot{Body: "The pond froze so smoothly that the whole neighborhood came out to skate.", ReplyAuthorHandle: "storywalker", SourceEchoTitle: "Winter at Prince Pond"})
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != QuoteSnapshotWidth || image.Bounds().Dy() != QuoteSnapshotHeight {
		t.Fatalf("snapshot size = %dx%d", image.Bounds().Dx(), image.Bounds().Dy())
	}
}
