package ai

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestDetectImageFormat(t *testing.T) {
	if detectImageFormat([]byte{0xFF, 0xD8, 0xFF, 0x00}) != "jpeg" {
		t.Fatal("jpeg magic not detected")
	}
	if detectImageFormat([]byte("\x89PNG\r\n\x1a\n")) != "png" {
		t.Fatal("png magic not detected")
	}
	riff := append([]byte("RIFF"), []byte{0, 0, 0, 0}...)
	riff = append(riff, []byte("WEBP")...)
	if detectImageFormat(riff) != "webp" {
		t.Fatal("webp magic not detected")
	}
	if detectImageFormat([]byte("hello")) != "" {
		t.Fatal("unknown should return empty")
	}
}

func makeSolidJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidateImage(t *testing.T) {
	limit := DefaultImageLimit()
	data := makeSolidJPEG(t, 10, 10)
	info, err := ValidateImage(data, limit)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "jpeg" || info.Width != 10 || info.Height != 10 {
		t.Fatalf("info = %+v", info)
	}
	small := limit
	small.MaxBytes = 10
	if _, err := ValidateImage(data, small); err == nil {
		t.Fatal("want error for oversized bytes")
	}
	corrupt := append([]byte{0xFF, 0xD8, 0xFF}, []byte("not a jpeg")...)
	if _, err := ValidateImage(corrupt, limit); err == nil {
		t.Fatal("want error for corrupt image")
	}
	tiny := limit
	tiny.MaxPixels = 10
	if _, err := ValidateImage(data, tiny); err == nil {
		t.Fatal("want error for oversized pixels")
	}
}

func TestValidateImagePNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	info, err := ValidateImage(buf.Bytes(), DefaultImageLimit())
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "png" || info.Width != 8 || info.Height != 8 {
		t.Fatalf("info = %+v", info)
	}
}

func TestValidateImages(t *testing.T) {
	limit := DefaultImageLimit()
	data := makeSolidJPEG(t, 5, 5)
	many := make([][]byte, limit.MaxCount+1)
	for i := range many {
		many[i] = data
	}
	if err := ValidateImages(many, limit); err == nil {
		t.Fatal("want error for too many images")
	}
	tiny := limit
	tiny.MaxTotalBytes = int64(len(data) - 1)
	if err := ValidateImages([][]byte{data, data}, tiny); err == nil {
		t.Fatal("want error for total bytes")
	}
}

func TestMakeControlledVersion(t *testing.T) {
	limit := DefaultImageLimit()
	data := makeSolidJPEG(t, 2100, 2100) // 超过 MaxSide 2048，触发缩放
	out, err := MakeControlledVersion("jpeg", data, limit)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width > limit.MaxSide || config.Height > limit.MaxSide {
		t.Fatalf("scaled size %dx%d exceeds maxSide %d", config.Width, config.Height, limit.MaxSide)
	}
	small := makeSolidJPEG(t, 100, 100)
	out2, err := MakeControlledVersion("jpeg", small, limit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(out2)); err != nil {
		t.Fatalf("small image should re-encode to JPEG: %v", err)
	}
}
