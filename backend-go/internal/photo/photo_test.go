package photo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// encode は、指定された標準ライブラリのフォーマットの、メモリ上の w x h の
// 画像を返す。
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

// pngHeader は、PNG シグネチャと、指定された bit depth（8 または 16）で
// w x h を宣言する有効な IHDR チャンクを手作りする。これにより、テストが
// その大きさの実際の画像を確保しなくても、DecodeConfig が寸法と color model を
// 見られる。
func pngHeader(t *testing.T, w, h uint32, bitDepth byte) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = bitDepth
	ihdr[9] = 2 // color type：トゥルーカラー
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
			got, err := photo.Process(context.Background(), bytes.NewReader(tt.input))
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

// exifAPP1 は、与えられた TIFF ストリームを包む完全な JPEG APP1 セグメント
// （マーカー、長さ、"Exif\0\0" ヘッダ）を組み立てる。
func exifAPP1(tiff []byte) []byte {
	payload := append([]byte("Exif\x00\x00"), tiff...)
	length := len(payload) + 2
	seg := []byte{0xFF, 0xE1, byte(length >> 8), byte(length)}
	return append(seg, payload...)
}

// orientationTIFF は、IFD0 のエントリを 1 つだけ持つ little-endian の TIFF
// ストリームを組み立てる。エントリは、指定された値を持つ Orientation タグ
// （0x0112、SHORT、count 1）である。IFD は実際にはオフセット 8 にあり、
// 別の ifdOffset を渡すと、ヘッダがでたらめな場所を指すストリームができる。
func orientationTIFF(orientation uint16, ifdOffset uint32) []byte {
	tiff := []byte{'I', 'I', 42, 0} // little-endian のバイトオーダー + TIFF の magic
	tiff = binary.LittleEndian.AppendUint32(tiff, ifdOffset)
	tiff = append(tiff,
		1, 0, // IFD エントリが 1 つ
		0x12, 0x01, // タグ 0x0112 Orientation
		3, 0, // 型 SHORT
		1, 0, 0, 0, // count が 1
	)
	tiff = binary.LittleEndian.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0, 0)       // 値フィールドのパディング
	return append(tiff, 0, 0, 0, 0) // 次の IFD へのオフセット：なし
}

// spliceAfterSOI は、SOI マーカーの直後にセグメントを挿入する。実際の
// カメラは EXIF APP1 をそこに置く。
func spliceAfterSOI(t *testing.T, jpg, seg []byte) []byte {
	t.Helper()
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Fatal("not a JPEG: missing SOI")
	}
	out := make([]byte, 0, len(jpg)+len(seg))
	out = append(out, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

// quadrantJPEG は、4 つの象限がそれぞれ異なる単色になっている w x h の JPEG を
// エンコードする。左上が赤、右上が緑、左下が青、右下が黄である。大きな単色の
// ブロックにより、JPEG の劣化がサンプリングする中心に及ばないようにしている。
func quadrantJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch {
			case x < w/2 && y < h/2:
				c = color.NRGBA{R: 255, A: 255}
			case x >= w/2 && y < h/2:
				c = color.NRGBA{G: 255, A: 255}
			case x < w/2:
				c = color.NRGBA{B: 255, A: 255}
			default:
				c = color.NRGBA{R: 255, G: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding quadrant jpeg: %v", err)
	}
	return buf.Bytes()
}

// quadrantColors は data を decode し、各象限の中心をサンプリングして、
// 左上、右上、左下、右下の順に 8-bit の RGB として返す。
func quadrantColors(t *testing.T, data []byte) [4][3]int {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding processed output: %v", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	points := [4][2]int{
		{w / 4, h / 4},
		{3 * w / 4, h / 4},
		{w / 4, 3 * h / 4},
		{3 * w / 4, 3 * h / 4},
	}
	var out [4][3]int
	for i, p := range points {
		r, g, bl, _ := img.At(b.Min.X+p[0], b.Min.Y+p[1]).RGBA()
		out[i] = [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
	}
	return out
}

// colorClose は JPEG の劣化を許容する。各チャンネルはずれてもよいが、その
// ずれは象限の色の間にある 0 対 255 の差よりはるかに小さい。
func colorClose(got, want [3]int) bool {
	for i := range got {
		d := got[i] - want[i]
		if d < 0 {
			d = -d
		}
		if d > 64 {
			return false
		}
	}
	return true
}

func TestProcessExifOrientation(t *testing.T) {
	// 64x40 は 1600px の shrink のしきい値を下回るので、出力の寸法は
	// orientation の変換だけを反映する。
	const w, h = 64, 40
	red := [3]int{255, 0, 0}
	green := [3]int{0, 255, 0}
	blue := [3]int{0, 0, 255}
	yellow := [3]int{255, 255, 0}
	tests := []struct {
		name         string
		orientation  uint16
		wantW, wantH int
		// want は補正後の画像の象限の色（TL、TR、BL、BR）で、保存されている
		// ラスタ TL=red TR=green BL=blue BR=yellow から導出したものである。
		want [4][3]int
	}{
		{"orientation 1 passes through", 1, w, h, [4][3]int{red, green, blue, yellow}},
		{"orientation 2 flips horizontally", 2, w, h, [4][3]int{green, red, yellow, blue}},
		{"orientation 3 rotates 180", 3, w, h, [4][3]int{yellow, blue, green, red}},
		{"orientation 4 flips vertically", 4, w, h, [4][3]int{blue, yellow, red, green}},
		{"orientation 5 transposes", 5, h, w, [4][3]int{red, blue, green, yellow}},
		{"orientation 6 rotates 90 clockwise", 6, h, w, [4][3]int{blue, red, yellow, green}},
		{"orientation 7 transverses", 7, h, w, [4][3]int{yellow, green, blue, red}},
		{"orientation 8 rotates 90 counter-clockwise", 8, h, w, [4][3]int{green, yellow, red, blue}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := spliceAfterSOI(t, quadrantJPEG(t, w, h), exifAPP1(orientationTIFF(tt.orientation, 8)))
			got, err := photo.Process(context.Background(), bytes.NewReader(input))
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if got.ContentType != "image/jpeg" || got.Ext != ".jpg" {
				t.Fatalf("Process = (%q, %q), want (image/jpeg, .jpg)", got.ContentType, got.Ext)
			}
			if gw, gh := decodeDims(t, got.Data); gw != tt.wantW || gh != tt.wantH {
				t.Fatalf("output dimensions = %dx%d, want %dx%d", gw, gh, tt.wantW, tt.wantH)
			}
			colors := quadrantColors(t, got.Data)
			for i, name := range [4]string{"top-left", "top-right", "bottom-left", "bottom-right"} {
				if !colorClose(colors[i], tt.want[i]) {
					t.Errorf("%s quadrant = %v, want ~%v", name, colors[i], tt.want[i])
				}
			}
		})
	}
}

// TestProcessOrientationSegmentWalk は、Exif APP1 が SOI の直後の最初の要素
// ではない JPEG ストリームをカバーする。その前にある Exif 以外の APP1
// （例：XMP）や 0xFF のフィルバイトは走査を止めてはならず、したがって補正は
// 引き続き適用される。
func TestProcessOrientationSegmentWalk(t *testing.T) {
	const w, h = 64, 40
	exifSeg := exifAPP1(orientationTIFF(6, 8))
	xmpPayload := []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>")
	xmpLength := len(xmpPayload) + 2
	xmpSeg := append([]byte{0xFF, 0xE1, byte(xmpLength >> 8), byte(xmpLength)}, xmpPayload...)
	tests := []struct {
		name string
		pre  []byte
	}{
		{"XMP APP1 before the Exif APP1", xmpSeg},
		{"0xFF fill byte before the Exif APP1", []byte{0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := append(append([]byte{}, tt.pre...), exifSeg...)
			input := spliceAfterSOI(t, quadrantJPEG(t, w, h), seg)
			got, err := photo.Process(context.Background(), bytes.NewReader(input))
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if gw, gh := decodeDims(t, got.Data); gw != h || gh != w {
				t.Fatalf("output dimensions = %dx%d, want %dx%d (orientation 6 applied)", gw, gh, h, w)
			}
		})
	}
}

func TestProcessMalformedExifIsIgnored(t *testing.T) {
	const w, h = 64, 40
	red := [3]int{255, 0, 0}
	base := quadrantJPEG(t, w, h)
	tests := []struct {
		name string
		seg  []byte
	}{
		// セグメント長は JPEG としては有効だが、TIFF ストリームが 8 バイトの
		// ヘッダに満たないところで切れている。
		{"truncated APP1 payload", exifAPP1([]byte("II*\x00"))},
		{"IFD offset beyond the segment", exifAPP1(orientationTIFF(6, 0xFFFF))},
		{"orientation value 0", exifAPP1(orientationTIFF(0, 8))},
		{"orientation value 9", exifAPP1(orientationTIFF(9, 8))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := photo.Process(context.Background(), bytes.NewReader(spliceAfterSOI(t, base, tt.seg)))
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if gw, gh := decodeDims(t, got.Data); gw != w || gh != h {
				t.Fatalf("output dimensions = %dx%d, want %dx%d (no correction)", gw, gh, w, h)
			}
			if tl := quadrantColors(t, got.Data)[0]; !colorClose(tl, red) {
				t.Fatalf("top-left quadrant = %v, want ~%v (no correction)", tl, red)
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
		// 4500x4500 は 20.25M ピクセルで maxPixels 未満だが、16-bit の
		// truecolor（decode 後は 8 bytes/px）では推定値が約 162MB になり、
		// 128MiB の decode メモリ上限を超える。
		{name: "16-bit image under the pixel cap but over the decode memory limit", input: pngHeader(t, 4500, 4500, 16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := photo.Process(context.Background(), bytes.NewReader(tt.input))
			if !errors.Is(err, photo.ErrUnsupportedImage) {
				t.Fatalf("Process error = %v, want ErrUnsupportedImage", err)
			}
		})
	}
}
