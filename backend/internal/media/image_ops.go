package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	xdraw "golang.org/x/image/draw"

	// 注册 JPEG/PNG/GIF 解码器
	_ "image/gif"
)

// TS 对照: media/image-ops.ts (474L)
// RUST_CANDIDATE: P2 — 图像编解码密集运算
//
// 拆分说明：
//   - EXIF 方向读取 → image_exif.go
//   - 方向规范化（旋转/翻转） → image_orient.go

// ---------- 类型 ----------

// ImageMetadata 图像元数据。
// TS 对照: image-ops.ts L8-11
type ImageMetadata struct {
	Width  int
	Height int
}

// ---------- 平台检测 ----------

// prefersSips 判断是否优先使用 macOS sips 而非 Go 内置处理。
// TS 对照: image-ops.ts L17-22
func prefersSips() bool {
	return runtime.GOOS == "darwin"
}

// ---------- 图像元数据 ----------

// GetImageMetadata 获取图像的宽高元数据。
// TS 对照: image-ops.ts L208-228
func GetImageMetadata(buffer []byte) (*ImageMetadata, error) {
	if prefersSips() {
		meta, err := sipsMetadataFromBuffer(buffer)
		if err == nil && meta != nil {
			return meta, nil
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buffer))
	if err != nil {
		return nil, fmt.Errorf("解码图像配置失败: %w", err)
	}
	return &ImageMetadata{Width: cfg.Width, Height: cfg.Height}, nil
}

// sipsMetadataFromBuffer 使用 macOS sips 获取图像元数据。
// TS 对照: image-ops.ts L136-163
func sipsMetadataFromBuffer(buffer []byte) (*ImageMetadata, error) {
	tmpFile, err := os.CreateTemp("", "sips-meta-*.jpg")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(buffer); err != nil {
		tmpFile.Close()
		return nil, err
	}
	tmpFile.Close()

	out, err := exec.Command("sips", "-g", "pixelWidth", "-g", "pixelHeight", tmpFile.Name()).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var width, height int
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pixelWidth:") {
			fmt.Sscanf(line, "pixelWidth: %d", &width)
		}
		if strings.HasPrefix(line, "pixelHeight:") {
			fmt.Sscanf(line, "pixelHeight: %d", &height)
		}
	}
	if width > 0 && height > 0 {
		return &ImageMetadata{Width: width, Height: height}, nil
	}
	return nil, fmt.Errorf("sips 未返回有效尺寸")
}

// ---------- 图像缩放 ----------

// ResizeToJpeg 缩放图像到 JPEG 格式。
// TS 对照: image-ops.ts L304-347
func ResizeToJpeg(buffer []byte, maxSide int, quality int) ([]byte, error) {
	if quality <= 0 {
		quality = 85
	}

	// macOS sips 路径
	if prefersSips() {
		result, err := sipsResizeToJpeg(buffer, maxSide, quality)
		if err == nil {
			return result, nil
		}
	}

	// Go 标准库路径
	img, _, err := image.Decode(bytes.NewReader(buffer))
	if err != nil {
		return nil, fmt.Errorf("解码图像失败: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxSide && h <= maxSide {
		// 不需要缩放，直接编码为 JPEG
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("JPEG 编码失败: %w", err)
		}
		return buf.Bytes(), nil
	}

	// 计算缩放比例
	ratio := float64(maxSide) / math.Max(float64(w), float64(h))
	newW := int(math.Round(float64(w) * ratio))
	newH := int(math.Round(float64(h) * ratio))

	resized := resizeImage(img, newW, newH)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("JPEG 编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// sipsResizeToJpeg 使用 macOS sips 缩放图像到 JPEG。
// TS 对照: image-ops.ts L165-193
func sipsResizeToJpeg(buffer []byte, maxSide, quality int) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "sips-resize-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input.jpg")
	outPath := filepath.Join(tmpDir, "output.jpg")
	if err := os.WriteFile(inPath, buffer, 0600); err != nil {
		return nil, err
	}

	args := []string{
		"-s", "format", "jpeg",
		"-s", "formatOptions", fmt.Sprintf("%d", quality),
		"--resampleHeightWidthMax", fmt.Sprintf("%d", maxSide),
		inPath,
		"--out", outPath,
	}
	if err := exec.Command("sips", args...).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(outPath)
}

// ---------- 格式转换 ----------

// ConvertHeicToJpeg 将 HEIC 图像转换为 JPEG。
// 按优先级尝试多种外部工具：
//  1. sips（macOS 内置，最高质量）
//  2. ImageMagick（magick / convert，跨平台，Windows 可通过 winget/choco 安装）
//  3. ffmpeg（跨平台，许多系统已预装）
//
// TS 对照: image-ops.ts L349-355
func ConvertHeicToJpeg(buffer []byte) ([]byte, error) {
	// 策略 1: macOS sips
	if prefersSips() {
		result, err := sipsConvertToJpeg(buffer)
		if err == nil {
			return result, nil
		}
		// sips 失败时继续尝试其他工具
	}

	// 策略 2: ImageMagick (magick 或 convert)
	if magickBin := findImageMagick(); magickBin != "" {
		result, err := magickConvertToJpeg(buffer, magickBin)
		if err == nil {
			return result, nil
		}
	}

	// 策略 3: ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		result, err := ffmpegConvertToJpeg(buffer)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("HEIC 转换不可用: 请安装 ImageMagick (magick) 或 ffmpeg。" +
		"Windows: winget install ImageMagick | Linux: apt install imagemagick")
}

// findImageMagick 查找 ImageMagick 可执行文件。
// ImageMagick 7 使用 "magick"，ImageMagick 6 使用 "convert"。
func findImageMagick() string {
	if _, err := exec.LookPath("magick"); err == nil {
		return "magick"
	}
	if _, err := exec.LookPath("convert"); err == nil {
		return "convert"
	}
	return ""
}

// magickConvertToJpeg 使用 ImageMagick 将 HEIC 转换为 JPEG。
func magickConvertToJpeg(buffer []byte, magickBin string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "magick-convert-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input.heic")
	outPath := filepath.Join(tmpDir, "output.jpg")
	if err := os.WriteFile(inPath, buffer, 0600); err != nil {
		return nil, err
	}

	var args []string
	if magickBin == "magick" {
		// ImageMagick 7: magick convert input output
		args = []string{"convert", inPath, "-quality", "90", outPath}
	} else {
		// ImageMagick 6: convert input output
		args = []string{inPath, "-quality", "90", outPath}
	}

	if err := exec.Command(magickBin, args...).Run(); err != nil {
		return nil, fmt.Errorf("ImageMagick HEIC 转换失败: %w", err)
	}
	return os.ReadFile(outPath)
}

// ffmpegConvertToJpeg 使用 ffmpeg 将 HEIC 转换为 JPEG。
func ffmpegConvertToJpeg(buffer []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-convert-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input.heic")
	outPath := filepath.Join(tmpDir, "output.jpg")
	if err := os.WriteFile(inPath, buffer, 0600); err != nil {
		return nil, err
	}

	args := []string{"-i", inPath, "-q:v", "2", "-y", outPath}
	if err := exec.Command("ffmpeg", args...).Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg HEIC 转换失败: %w", err)
	}
	return os.ReadFile(outPath)
}

// sipsConvertToJpeg 使用 macOS sips 转换为 JPEG。
// TS 对照: image-ops.ts L195-206
func sipsConvertToJpeg(buffer []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "sips-convert-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input")
	outPath := filepath.Join(tmpDir, "output.jpg")
	if err := os.WriteFile(inPath, buffer, 0600); err != nil {
		return nil, err
	}
	if err := exec.Command("sips", "-s", "format", "jpeg", inPath, "--out", outPath).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(outPath)
}

// ---------- PNG 操作 ----------

// HasAlphaChannel 检查图像是否有 alpha 通道（透明度）。
// TS 对照: image-ops.ts L357-372
func HasAlphaChannel(buffer []byte) bool {
	img, _, err := image.Decode(bytes.NewReader(buffer))
	if err != nil {
		return false
	}
	switch img.(type) {
	case *image.NRGBA, *image.NRGBA64, *image.RGBA, *image.RGBA64:
		return true
	default:
		return false
	}
}

// ResizeToPng 缩放图像到 PNG 格式，保留 alpha 通道。
// TS 对照: image-ops.ts L374-398
func ResizeToPng(buffer []byte, maxSide int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(buffer))
	if err != nil {
		return nil, fmt.Errorf("解码图像失败: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxSide && h <= maxSide {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("PNG 编码失败: %w", err)
		}
		return buf.Bytes(), nil
	}

	ratio := float64(maxSide) / math.Max(float64(w), float64(h))
	newW := int(math.Round(float64(w) * ratio))
	newH := int(math.Round(float64(h) * ratio))

	resized := resizeImage(img, newW, newH)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return nil, fmt.Errorf("PNG 编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// OptimizeToPngResult PNG 优化结果。
type OptimizeToPngResult struct {
	Buffer           []byte
	OptimizedSize    int
	ResizeSide       int
	CompressionLevel int
}

// OptimizeImageToPng 尝试多种分辨率使图像 PNG 大小不超过 maxBytes。
// TS 对照: image-ops.ts L400-457
func OptimizeImageToPng(buffer []byte, maxBytes int) (*OptimizeToPngResult, error) {
	// 尝试从大到小的分辨率
	sides := []int{2048, 1536, 1024, 768, 512, 384, 256}
	for _, side := range sides {
		result, err := ResizeToPng(buffer, side)
		if err != nil {
			continue
		}
		if len(result) <= maxBytes {
			return &OptimizeToPngResult{
				Buffer:        result,
				OptimizedSize: len(result),
				ResizeSide:    side,
			}, nil
		}
	}
	return nil, fmt.Errorf("无法将图像优化到 %d 字节以内", maxBytes)
}

// ---------- 内部辅助 ----------

// resizeImage 使用双三次插值（CatmullRom）缩放图像。
// 已从 BiLinear 升级为 CatmullRom（双三次），质量显著提升。
// RUST_CANDIDATE: P2 — Rust FFI 可选优化（SIMD 加速批量处理场景）
func resizeImage(src image.Image, newW, newH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}
