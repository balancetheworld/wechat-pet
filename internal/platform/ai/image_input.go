package ai

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// 图片上限。文档 11.5：上限取供应商能力及服务端资源限制中较小者，在适配任务中固定。
// 当前为服务端资源限制的保守默认值；供应商能力上限（deepseek-flash 的真实图片像素/大小限制）
// 待真实组合能力验证后回填调整。
const (
	defaultImageMaxBytes      = 10 << 20    // 单张字节上限
	defaultImageMaxPixels     = 4096 * 4096 // 单张像素上限
	defaultImageMaxCount      = 4           // 单次消息张数上限
	defaultImageMaxTotalBytes = 20 << 20    // 单次消息图片累计大小
	defaultModelImageMaxSide  = 2048        // 受控版本边长上限
	defaultModelImagePixels   = 4096 * 4096 // 受控版本总像素上限
)

// ImageLimit 是图片校验与受控版本生成的资源上限。
type ImageLimit struct {
	MaxBytes       int   // 单张字节上限
	MaxPixels      int64 // 单张像素上限
	MaxCount       int   // 单次消息张数上限
	MaxTotalBytes  int64 // 单次消息图片累计大小上限
	MaxSide        int   // 受控版本边长上限
	MaxTotalPixels int64 // 受控版本总像素上限
}

// DefaultImageLimit 返回服务端资源限制的保守默认上限。
func DefaultImageLimit() ImageLimit {
	return ImageLimit{
		MaxBytes:       defaultImageMaxBytes,
		MaxPixels:      defaultImageMaxPixels,
		MaxCount:       defaultImageMaxCount,
		MaxTotalBytes:  defaultImageMaxTotalBytes,
		MaxSide:        defaultModelImageMaxSide,
		MaxTotalPixels: defaultModelImagePixels,
	}
}

var (
	ErrImageTooLarge        = errors.New("image exceeds byte limit")
	ErrImageTooManyPixels   = errors.New("image exceeds pixel limit")
	ErrImageTooMany         = errors.New("too many images")
	ErrImageTotalTooLarge   = errors.New("images exceed total byte limit")
	ErrImageUnsupportedType = errors.New("unsupported image type")
	ErrImageCorrupt         = errors.New("image decode failed")
)

// ImageInfo 是单张图片校验后的实际信息。文档 11.5：服务端检查实际文件类型，不凭文件名或扩展名判断。
type ImageInfo struct {
	Format string // jpeg / png / webp
	Width  int
	Height int
	Bytes  int
}

// ValidateImage 校验单张图片的实际文件类型、解码结果、字节与像素。
// 不校验扩展名，只看文件内容（magic bytes）与能否解码出有效尺寸。
func ValidateImage(data []byte, limit ImageLimit) (ImageInfo, error) {
	if len(data) > limit.MaxBytes {
		return ImageInfo{}, fmt.Errorf("%w: %d bytes", ErrImageTooLarge, len(data))
	}
	format := detectImageFormat(data)
	if format == "" {
		return ImageInfo{}, ErrImageUnsupportedType
	}
	config, err := decodeImageConfig(format, data)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("%w: %v", ErrImageCorrupt, err)
	}
	pixels := int64(config.Width) * int64(config.Height)
	if pixels > limit.MaxPixels {
		return ImageInfo{}, fmt.Errorf("%w: %dx%d", ErrImageTooManyPixels, config.Width, config.Height)
	}
	return ImageInfo{Format: format, Width: config.Width, Height: config.Height, Bytes: len(data)}, nil
}

// ValidateImages 校验一组图片的张数与累计大小。文档 11.5：张数与累计大小超限时阻止消息接收。
func ValidateImages(items [][]byte, limit ImageLimit) error {
	if len(items) > limit.MaxCount {
		return fmt.Errorf("%w: %d", ErrImageTooMany, len(items))
	}
	var total int64
	for _, item := range items {
		total += int64(len(item))
	}
	if total > limit.MaxTotalBytes {
		return fmt.Errorf("%w: %d bytes", ErrImageTotalTooLarge, total)
	}
	return nil
}

// MakeControlledVersion 为模型生成受控尺寸版本（缩放并重编码为 JPEG）。
// 文档 11.5：为模型生成受控尺寸版本，重编码同时去除无关定位等元数据；原资产与处理版本可追溯。
// 返回重编码后的 JPEG 字节。若图片已满足受控上限，仍重编码以去除元数据并统一格式。
func MakeControlledVersion(format string, data []byte, limit ImageLimit) ([]byte, error) {
	src, err := decodeImage(format, data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrImageCorrupt, err)
	}
	scaled := scaleDown(src, limit.MaxSide, limit.MaxTotalPixels)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode controlled version: %w", err)
	}
	return buf.Bytes(), nil
}

func detectImageFormat(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp"
	default:
		return ""
	}
}

func decodeImageConfig(format string, data []byte) (image.Config, error) {
	switch format {
	case "jpeg":
		return jpeg.DecodeConfig(bytes.NewReader(data))
	case "png":
		return png.DecodeConfig(bytes.NewReader(data))
	case "webp":
		return webp.DecodeConfig(bytes.NewReader(data))
	default:
		return image.Config{}, ErrImageUnsupportedType
	}
}

func decodeImage(format string, data []byte) (image.Image, error) {
	switch format {
	case "jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	case "png":
		return png.Decode(bytes.NewReader(data))
	case "webp":
		return webp.Decode(bytes.NewReader(data))
	default:
		return nil, ErrImageUnsupportedType
	}
}

// scaleDown 将图片缩放到受控上限内；已满足时原样返回。
// 注意：JPEG 的 EXIF Orientation 方向处理待真实素材验证后补充（见文档 11.5「处理方向」）。
func scaleDown(src image.Image, maxSide int, maxPixels int64) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}
	scale := 1.0
	if width > maxSide || height > maxSide {
		scale = math.Min(float64(maxSide)/float64(width), float64(maxSide)/float64(height))
	}
	if int64(width)*int64(height) > maxPixels {
		scale = math.Min(scale, math.Sqrt(float64(maxPixels)/float64(width*height)))
	}
	if scale >= 1.0 {
		return src
	}
	newWidth := max(1, int(math.Round(float64(width)*scale)))
	newHeight := max(1, int(math.Round(float64(height)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
