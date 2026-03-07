package browser

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"log/slog"
	"testing"
)

func makeTestJPEG(w, h int, col color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, col)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}

func TestGIFRecorder_AddFrame(t *testing.T) {
	r := NewGIFRecorder(GIFRecorderConfig{MaxWidth: 400}, slog.Default())

	if r.FrameCount() != 0 {
		t.Fatalf("expected 0 frames, got %d", r.FrameCount())
	}

	frame := makeTestJPEG(100, 100, color.RGBA{255, 0, 0, 255})
	if err := r.AddFrame(frame, 50); err != nil {
		t.Fatalf("AddFrame: %v", err)
	}

	if r.FrameCount() != 1 {
		t.Fatalf("expected 1 frame, got %d", r.FrameCount())
	}
}

func TestGIFRecorder_Encode(t *testing.T) {
	r := NewGIFRecorder(GIFRecorderConfig{}, slog.Default())

	// Add 3 frames.
	colors := []color.Color{
		color.RGBA{255, 0, 0, 255},
		color.RGBA{0, 255, 0, 255},
		color.RGBA{0, 0, 255, 255},
	}
	for _, c := range colors {
		if err := r.AddFrame(makeTestJPEG(200, 150, c), 30); err != nil {
			t.Fatalf("AddFrame: %v", err)
		}
	}

	data, err := r.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty GIF data")
	}

	// Verify it's a valid GIF.
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode GIF: %v", err)
	}
	if len(g.Image) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(g.Image))
	}
}

func TestGIFRecorder_EncodeEmpty(t *testing.T) {
	r := NewGIFRecorder(GIFRecorderConfig{}, slog.Default())
	_, err := r.Encode()
	if err == nil {
		t.Fatal("expected error for empty recorder")
	}
}

func TestGIFRecorder_Downscale(t *testing.T) {
	r := NewGIFRecorder(GIFRecorderConfig{MaxWidth: 200}, slog.Default())

	// Create a 1000x500 frame — should be downscaled to 200x100.
	if err := r.AddFrame(makeTestJPEG(1000, 500, color.RGBA{0, 0, 0, 255}), 50); err != nil {
		t.Fatalf("AddFrame: %v", err)
	}

	data, err := r.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode GIF: %v", err)
	}

	bounds := g.Image[0].Bounds()
	if bounds.Dx() != 200 {
		t.Fatalf("expected width 200, got %d", bounds.Dx())
	}
	if bounds.Dy() != 100 {
		t.Fatalf("expected height 100, got %d", bounds.Dy())
	}
}

func TestDownscaleImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	dst := downscaleImage(src, 200)
	bounds := dst.Bounds()
	if bounds.Dx() != 200 {
		t.Fatalf("expected width 200, got %d", bounds.Dx())
	}
	if bounds.Dy() != 150 {
		t.Fatalf("expected height 150, got %d", bounds.Dy())
	}
}

func TestDownscaleImage_NoOp(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	dst := downscaleImage(src, 200)
	// Should return same image since it's smaller than target.
	if dst != src {
		t.Fatal("expected same image for no-op downscale")
	}
}

func TestGIFRecorder_MaxFrames(t *testing.T) {
	r := NewGIFRecorder(GIFRecorderConfig{MaxFrames: 3}, slog.Default())

	frame := makeTestJPEG(100, 100, color.RGBA{255, 0, 0, 255})
	for i := 0; i < 5; i++ {
		if err := r.AddFrame(frame, 50); err != nil {
			t.Fatalf("AddFrame %d: %v", i, err)
		}
	}

	// Should have capped at 3 frames.
	if r.FrameCount() != 3 {
		t.Fatalf("expected 3 frames (MaxFrames limit), got %d", r.FrameCount())
	}

	data, err := r.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode GIF: %v", err)
	}
	if len(g.Image) != 3 {
		t.Fatalf("expected 3 GIF frames, got %d", len(g.Image))
	}
}
