// Package photo validates and normalizes uploaded review photos: it
// sniffs the real image format from magic bytes (the client-declared
// content type is ignored), guards against decompression bombs, downscales
// so the long edge fits maxEdge, and re-encodes. It depends on the
// standard library and golang.org/x/image only, so usecase may import it
// without violating the inward dependency rule.
package photo

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers webp with image.Decode (pure-Go decode-only)
)

// ErrUnsupportedImage marks an upload that is not a decodable
// jpeg/png/webp within the size limits; handlers map it to 422.
var ErrUnsupportedImage = errors.New("unsupported image")

const (
	// maxEdge is the longest output edge; larger images are downscaled,
	// smaller ones are never upscaled.
	maxEdge = 1600
	// maxDimension and maxPixels bound the size declared in the image
	// header, checked before the full decode (decompression-bomb guard).
	maxDimension = 10000
	maxPixels    = 40_000_000
	jpegQuality  = 85
)

// Processed is the normalized image ready for storage; the original
// upload bytes are discarded.
type Processed struct {
	// Data holds the re-encoded image.
	Data []byte
	// ContentType is "image/jpeg" (jpeg and webp sources) or "image/png".
	ContentType string
	// Ext is ".jpg" or ".png", for building the storage key.
	Ext string
}

// Process reads one uploaded image from r (streaming; it buffers
// internally only as much as decoding needs) and returns the normalized
// result. Payloads that are not jpeg/png/webp by magic bytes, fail to
// decode, or declare dimensions beyond the bomb guard yield a wrapped
// ErrUnsupportedImage.
func Process(r io.Reader) (Processed, error) {
	br := bufio.NewReader(r)
	head, err := br.Peek(512)
	if err != nil && len(head) == 0 {
		return Processed{}, fmt.Errorf("%w: empty or unreadable payload", ErrUnsupportedImage)
	}
	switch ct := http.DetectContentType(head); ct {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return Processed{}, fmt.Errorf("%w: detected %s", ErrUnsupportedImage, ct)
	}
	data, err := io.ReadAll(br)
	if err != nil {
		return Processed{}, fmt.Errorf("read image: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if cfg.Width > maxDimension || cfg.Height > maxDimension || cfg.Width*cfg.Height > maxPixels {
		return Processed{}, fmt.Errorf("%w: %dx%d exceeds the size limit", ErrUnsupportedImage, cfg.Width, cfg.Height)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Processed{}, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	img = shrink(img)
	var out bytes.Buffer
	if format == "png" {
		if err := png.Encode(&out, img); err != nil {
			return Processed{}, fmt.Errorf("encode png: %w", err)
		}
		return Processed{Data: out.Bytes(), ContentType: "image/png", Ext: ".png"}, nil
	}
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Processed{}, fmt.Errorf("encode jpeg: %w", err)
	}
	return Processed{Data: out.Bytes(), ContentType: "image/jpeg", Ext: ".jpg"}, nil
}

// shrink downscales img so its long edge is at most maxEdge, keeping the
// aspect ratio; images already within the bound pass through unchanged
// (no upscaling).
func shrink(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= maxEdge {
		return img
	}
	var dw, dh int
	if w >= h {
		dw, dh = maxEdge, max(1, h*maxEdge/w)
	} else {
		dw, dh = max(1, w*maxEdge/h), maxEdge
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
