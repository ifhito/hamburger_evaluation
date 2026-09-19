package photo

import (
	"bytes"
	"encoding/binary"
	"image"
)

// exifHeader prefixes the TIFF stream inside a JPEG APP1 segment.
var exifHeader = []byte("Exif\x00\x00")

const (
	orientationTag = 0x0112
	tiffTypeShort  = 3
	// maxSegmentScan caps how many JPEG segments jpegOrientation walks
	// before giving up; EXIF APP1 sits at the front of any real file, so a
	// sane bound also protects against pathological segment chains.
	maxSegmentScan = 32
)

// jpegOrientation extracts the EXIF Orientation value (1-8) from raw JPEG
// bytes. It is a deliberate best-effort parser: a photo with missing,
// truncated, or otherwise malformed EXIF must still be accepted, so ANY
// unexpected shape (no APP1, bad lengths, bogus offsets, out-of-range
// value) returns 1 — "no correction" — and never an error or a panic. The
// usual fail-loud rule does not apply here by design. All reads are
// bounded by explicit length checks against the input slice.
func jpegOrientation(data []byte) int {
	// SOI marker (0xFFD8) starts every JPEG.
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for seg := 0; seg < maxSegmentScan; seg++ {
		// 0xFF fill bytes may pad the stream before a marker; the marker
		// byte is the first non-0xFF after a run of 0xFF bytes.
		for i+1 < len(data) && data[i] == 0xFF && data[i+1] == 0xFF {
			i++
		}
		if i+4 > len(data) || data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		// Standalone markers (no length field): TEM, RSTn. Not expected
		// before SOS, but skipping them keeps the walk honest.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		// SOS: entropy-coded image data begins, no APP1 was found.
		if marker == 0xDA {
			return 1
		}
		length := int(data[i+2])<<8 | int(data[i+3])
		if length < 2 || i+2+length > len(data) {
			return 1
		}
		if marker == 0xE1 {
			// APP1 also carries non-Exif payloads (e.g. XMP); only stop
			// the walk for a segment that starts with the Exif header.
			payload := data[i+4 : i+2+length]
			if len(payload) >= len(exifHeader) && bytes.Equal(payload[:len(exifHeader)], exifHeader) {
				return exifOrientation(payload)
			}
		}
		i += 2 + length
	}
	return 1
}

// exifOrientation parses an APP1 payload ("Exif\0\0" + TIFF stream) and
// returns the IFD0 Orientation tag value, or 1 for anything malformed.
func exifOrientation(seg []byte) int {
	if len(seg) < len(exifHeader) || !bytes.Equal(seg[:len(exifHeader)], exifHeader) {
		return 1
	}
	tiff := seg[len(exifHeader):]
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		bo = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	off := int64(bo.Uint32(tiff[4:8]))
	if off < 8 || off+2 > int64(len(tiff)) {
		return 1
	}
	n := int(bo.Uint16(tiff[off : off+2]))
	entries := tiff[off+2:]
	if n*12 > len(entries) {
		return 1
	}
	for k := 0; k < n; k++ {
		e := entries[k*12 : k*12+12]
		if bo.Uint16(e[0:2]) != orientationTag {
			continue
		}
		if bo.Uint16(e[2:4]) != tiffTypeShort || bo.Uint32(e[4:8]) != 1 {
			return 1
		}
		// A SHORT with count 1 is stored inline in the first two value
		// bytes, in the stream's byte order.
		v := int(bo.Uint16(e[8:10]))
		if v < 1 || v > 8 {
			return 1
		}
		return v
	}
	return 1
}

// orient rewrites img so a viewer that ignores EXIF sees it upright, given
// the EXIF Orientation value o (1-8). Orientation 1 (and anything out of
// range) passes img through without allocating. The output is a plain
// NRGBA pixel copy; values 5-8 swap width and height.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var dst *image.NRGBA
	if o >= 5 {
		dst = image.NewNRGBA(image.Rect(0, 0, h, w))
	} else {
		dst = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // flip horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // flip vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 clockwise
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 90 counter-clockwise
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
