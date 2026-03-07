// gif_recorder.go — Phase 4.4: Multi-step browser operation GIF recording.
// Captures screenshots at intervals during browser automation and encodes
// them as an animated GIF for visual review and debugging.
package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"log/slog"
	"sync"
	"time"
)

// GIFRecorder captures browser screenshots and encodes them as animated GIF.
type GIFRecorder struct {
	mu        sync.Mutex
	frames    []image.Image
	delays    []int // centiseconds per frame
	logger    *slog.Logger
	maxWidth  int
	maxFrames int
	started   bool
}

// GIFRecorderConfig configures the GIF recorder.
type GIFRecorderConfig struct {
	MaxWidth  int // Max frame width (default: 800, to keep GIF size manageable)
	MaxFrames int // Max number of frames (default: 200, to prevent OOM)
}

// NewGIFRecorder creates a new GIF recorder.
func NewGIFRecorder(cfg GIFRecorderConfig, logger *slog.Logger) *GIFRecorder {
	if logger == nil {
		logger = slog.Default()
	}
	maxWidth := cfg.MaxWidth
	if maxWidth <= 0 {
		maxWidth = 800
	}
	maxFrames := cfg.MaxFrames
	if maxFrames <= 0 {
		maxFrames = 200
	}
	return &GIFRecorder{
		logger:    logger,
		maxWidth:  maxWidth,
		maxFrames: maxFrames,
	}
}

// AddFrame adds a screenshot frame (JPEG bytes) with the given delay.
func (r *GIFRecorder) AddFrame(jpegData []byte, delayCs int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.frames) >= r.maxFrames {
		r.logger.Debug("gif: frame limit reached, dropping frame", "maxFrames", r.maxFrames)
		return nil
	}

	if delayCs <= 0 {
		delayCs = 50 // default: 0.5s
	}

	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return fmt.Errorf("decode frame JPEG: %w", err)
	}

	// Downscale if needed.
	if img.Bounds().Dx() > r.maxWidth {
		img = downscaleImage(img, r.maxWidth)
	}

	r.frames = append(r.frames, img)
	r.delays = append(r.delays, delayCs)
	r.started = true
	return nil
}

// AddFrameFromBase64 adds a frame from base64-encoded JPEG data.
func (r *GIFRecorder) AddFrameFromBase64(b64Data string, delayCs int) error {
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return r.AddFrame(decoded, delayCs)
}

// FrameCount returns the number of captured frames.
func (r *GIFRecorder) FrameCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

// Encode encodes all captured frames into an animated GIF.
func (r *GIFRecorder) Encode() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.frames) == 0 {
		return nil, fmt.Errorf("no frames captured")
	}

	anim := &gif.GIF{}
	for i, img := range r.frames {
		// Quantize to 256-color palette.
		bounds := img.Bounds()
		palettedImg := image.NewPaletted(bounds, palette.Plan9)
		draw.FloydSteinberg.Draw(palettedImg, bounds, img, image.Point{})

		anim.Image = append(anim.Image, palettedImg)
		anim.Delay = append(anim.Delay, r.delays[i])
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, anim); err != nil {
		return nil, fmt.Errorf("encode GIF: %w", err)
	}

	r.logger.Info("GIF encoded", "frames", len(r.frames), "size", buf.Len())
	return buf.Bytes(), nil
}

// CaptureScreenshotFrame takes a screenshot via CDP and adds it as a frame.
func (r *GIFRecorder) CaptureScreenshotFrame(ctx context.Context, tools PlaywrightTools, target PWTargetOpts, delayCs int) error {
	data, err := tools.Screenshot(ctx, target)
	if err != nil {
		return fmt.Errorf("capture frame: %w", err)
	}

	// Screenshot returns base64-encoded JPEG.
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		// Not base64 — try raw bytes.
		return r.AddFrame(data, delayCs)
	}
	return r.AddFrame(decoded, delayCs)
}

// RecordAction captures before + after screenshots around an action.
func (r *GIFRecorder) RecordAction(ctx context.Context, tools PlaywrightTools, target PWTargetOpts, action func() error) error {
	// Before screenshot.
	if err := r.CaptureScreenshotFrame(ctx, tools, target, 50); err != nil {
		r.logger.Warn("gif: pre-action screenshot failed", "err", err)
	}

	// Execute the action.
	if err := action(); err != nil {
		return err
	}

	// Small delay to let the page settle after action.
	time.Sleep(300 * time.Millisecond)

	// After screenshot.
	if err := r.CaptureScreenshotFrame(ctx, tools, target, 100); err != nil {
		r.logger.Warn("gif: post-action screenshot failed", "err", err)
	}

	return nil
}

// downscaleImage downscales an image to the target width, preserving aspect ratio.
// Uses simple nearest-neighbor sampling for speed.
func downscaleImage(src image.Image, targetWidth int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= targetWidth {
		return src
	}

	ratio := float64(targetWidth) / float64(srcW)
	targetH := int(float64(srcH) * ratio)

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetH))
	for y := 0; y < targetH; y++ {
		srcY := int(float64(y) / ratio)
		for x := 0; x < targetWidth; x++ {
			srcX := int(float64(x) / ratio)
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}
	return dst
}

