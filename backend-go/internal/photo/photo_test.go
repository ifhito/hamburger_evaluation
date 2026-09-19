package photo_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// encode returns an in-memory w x h image in the given stdlib format.
func encode(t *testing.T, format string, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "png":
		err = png.Encode(&buf, img)
	default:
		t.Fatalf("unknown format %q", format)
	}
	if err != nil {
		t.Fatalf("encoding %s: %v", format, err)
	}
	return buf.Bytes()
}

// pngHeader hand-crafts a PNG signature plus a valid IHDR chunk declaring
// w x h at the given bit depth (8 or 16), so DecodeConfig sees the
// dimensions and color model without the test allocating a real image of
// that size.
func pngHeader(t *testing.T, w, h uint32, bitDepth byte) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = bitDepth
	ihdr[9] = 2 // color type: truecolor
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(ihdr))); err != nil {
		t.Fatal(err)
	}
	buf.WriteString("IHDR")
	buf.Write(ihdr)
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	if err := binary.Write(&buf, binary.BigEndian, crc.Sum32()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeDims(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding processed output: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestProcess(t *testing.T) {
	webpFixture, err := os.ReadFile(filepath.Join("testdata", "tiny.webp"))
	if err != nil {
		t.Fatalf("reading webp fixture: %v", err)
	}
	tests := []struct {
		name            string
		input           []byte
		wantContentType string
		wantExt         string
		wantW, wantH    int
	}{
		{
			name:            "large jpeg is resized to the 1600px long edge",
			input:           encode(t, "jpeg", 2000, 1000),
			wantContentType: "image/jpeg",
			wantExt:         ".jpg",
			wantW:           1600,
			wantH:           800,
		},
		{
			name:            "small jpeg is not upscaled",
			input:           encode(t, "jpeg", 800, 400),
			wantContentType: "image/jpeg",
			wantExt:         ".jpg",
			wantW:           800,
			wantH:           400,
		},
		{
			name:            "large portrait png is resized and stays png",
			input:           encode(t, "png", 900, 1800),
			wantContentType: "image/png",
			wantExt:         ".png",
			wantW:           800,
			wantH:           1600,
		},
		{
			name:            "small png is not upscaled",
			input:           encode(t, "png", 100, 50),
			wantContentType: "image/png",
			wantExt:         ".png",
			wantW:           100,
			wantH:           50,
		},
		{
			name:            "webp is re-encoded as jpeg",
			input:           webpFixture,
			wantContentType: "image/jpeg",
			wantExt:         ".jpg",
			wantW:           8,
			wantH:           8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := photo.Process(bytes.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if got.ContentType != tt.wantContentType || got.Ext != tt.wantExt {
				t.Fatalf("Process = (%q, %q), want (%q, %q)", got.ContentType, got.Ext, tt.wantContentType, tt.wantExt)
			}
			if w, h := decodeDims(t, got.Data); w != tt.wantW || h != tt.wantH {
				t.Fatalf("output dimensions = %dx%d, want %dx%d", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestProcessRejections(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "pdf magic bytes", input: append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{'x'}, 64)...)},
		{name: "plain text", input: []byte("just some text, definitely not an image")},
		{name: "empty payload", input: nil},
		{name: "png magic bytes without image data", input: []byte("\x89PNG\r\n\x1a\ngarbage")},
		{name: "width beyond 10000", input: pngHeader(t, 10001, 1, 8)},
		{name: "height beyond 10000", input: pngHeader(t, 1, 10001, 8)},
		{name: "pixel count beyond 24M", input: pngHeader(t, 5000, 5000, 8)},
		// 4500x4500 is 20.25M pixels — under maxPixels — but at 16-bit
		// truecolor (8 bytes/px decoded) the estimate is ~162MB, over the
		// 128MiB decode memory limit.
		{name: "16-bit image under the pixel cap but over the decode memory limit", input: pngHeader(t, 4500, 4500, 16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := photo.Process(bytes.NewReader(tt.input))
			if !errors.Is(err, photo.ErrUnsupportedImage) {
				t.Fatalf("Process error = %v, want ErrUnsupportedImage", err)
			}
		})
	}
}
