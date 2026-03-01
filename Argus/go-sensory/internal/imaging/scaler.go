// Package imaging provides image scaling and encoding utilities.
// Implements the dual-track output pattern: low-res for VLM (save tokens),
// high-res for human display.
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"strconv"

	"Argus-compound/go-sensory/internal/capture"

	xdraw "golang.org/x/image/draw"
)

// DefaultVLMMaxDim is the default maximum dimension (long edge) for VLM images.
// 768 balances token savings (~50% vs 1024) with acceptable detail for VLM tasks.
// Override via VLM_IMAGE_MAX_DIM env var if needed.
const DefaultVLMMaxDim = 768

// DefaultVLMQuality is the default JPEG quality for VLM images.
const DefaultVLMQuality = 50

// DefaultDisplayQuality is the default JPEG quality for display images.
const DefaultDisplayQuality = 80

// Scaler provides dual-track image encoding:
//   - ForVLM: downscaled + lower quality (saves tokens/bandwidth)
//   - ForDisplay: original resolution + higher quality (human viewing)
type Scaler struct {
	VLMMaxDim      int // Long-edge max for VLM images (default 1024)
	VLMQuality     int // JPEG quality for VLM (default 50)
	DisplayQuality int // JPEG quality for display (default 80)
}

// NewScaler creates a Scaler with defaults, overridable via env vars:
//   - VLM_IMAGE_MAX_DIM (default 1024)
//   - VLM_IMAGE_QUALITY (default 50)
//   - DISPLAY_IMAGE_QUALITY (default 80)
func NewScaler() *Scaler {
	s := &Scaler{
		VLMMaxDim:      DefaultVLMMaxDim,
		VLMQuality:     DefaultVLMQuality,
		DisplayQuality: DefaultDisplayQuality,
	}

	if v := os.Getenv("VLM_IMAGE_MAX_DIM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.VLMMaxDim = n
		}
	}
	if v := os.Getenv("VLM_IMAGE_QUALITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			s.VLMQuality = n
		}
	}
	if v := os.Getenv("DISPLAY_IMAGE_QUALITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			s.DisplayQuality = n
		}
	}

	log.Printf("[ImageScaler] VLM: maxDim=%d quality=%d | Display: quality=%d",
		s.VLMMaxDim, s.VLMQuality, s.DisplayQuality)
	return s
}

// ForVLM encodes a frame for VLM inference: downscaled to fit VLMMaxDim
// on the long edge, using CatmullRom (bicubic) interpolation.
//
// Token savings example (Gemini 1.5 Pro, 768px tile):
//
//	3456×2234 → 1024×662: ~3870 → ~516 tokens (87% reduction)
func (s *Scaler) ForVLM(frame *capture.Frame) ([]byte, error) {
	srcImg := frameToNRGBA(frame)
	scaled := s.downscale(srcImg, s.VLMMaxDim)

	origW, origH := srcImg.Bounds().Dx(), srcImg.Bounds().Dy()
	newW, newH := scaled.Bounds().Dx(), scaled.Bounds().Dy()

	data, err := encodeJPEG(scaled, s.VLMQuality)
	if err != nil {
		return nil, err
	}

	if origW != newW || origH != newH {
		log.Printf("[ImageScaler] VLM: %dx%d → %dx%d, %s → %s (%.0f%% reduction)",
			origW, origH, newW, newH,
			humanSize(origW*origH*4), humanSize(len(data)),
			(1-float64(len(data))/float64(origW*origH*4))*100)
	}

	return data, nil
}

// ForDisplay encodes a frame for human viewing: original resolution,
// higher JPEG quality. No downscaling applied.
func (s *Scaler) ForDisplay(frame *capture.Frame) ([]byte, error) {
	srcImg := frameToNRGBA(frame)
	return encodeJPEG(srcImg, s.DisplayQuality)
}

// ScaleJPEG downscales already-encoded JPEG bytes for VLM use.
// Useful for SoM-annotated images that are already JPEG.
func (s *Scaler) ScaleJPEG(jpegData []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG for scaling: %w", err)
	}

	scaled := s.downscale(img, s.VLMMaxDim)
	return encodeJPEG(scaled, s.VLMQuality)
}

// downscale fits the image within maxDim on the long edge.
// Uses CatmullRom (bicubic) for high-quality downsampling.
// Returns the original image if already within bounds.
func (s *Scaler) downscale(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Already within bounds
	if w <= maxDim && h <= maxDim {
		return src
	}

	// Compute target size preserving aspect ratio
	var newW, newH int
	if w >= h {
		newW = maxDim
		newH = h * maxDim / w
	} else {
		newH = maxDim
		newW = w * maxDim / h
	}

	dst := image.NewNRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

// frameToNRGBA converts a BGRA capture.Frame to an NRGBA image.
func frameToNRGBA(frame *capture.Frame) *image.NRGBA {
	w, h := frame.Width, frame.Height
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	srcStride := frame.Stride
	if srcStride == 0 {
		srcStride = w * 4
	}

	for y := 0; y < h; y++ {
		srcRow := frame.Pixels[y*srcStride : y*srcStride+w*4]
		dstRow := img.Pix[y*img.Stride : y*img.Stride+w*4]
		copy(dstRow, srcRow)
		for i := 0; i < len(dstRow); i += 4 {
			dstRow[i+0], dstRow[i+2] = dstRow[i+2], dstRow[i+0] // BGRA→RGBA
		}
	}
	return img
}

// encodeJPEG encodes an image to JPEG bytes with pre-sized buffer.
func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	bounds := img.Bounds()
	var buf bytes.Buffer
	buf.Grow(bounds.Dx() * bounds.Dy() / 10)
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("JPEG encode: %w", err)
	}
	return buf.Bytes(), nil
}

// humanSize formats byte count as human-readable string.
func humanSize(b int) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
