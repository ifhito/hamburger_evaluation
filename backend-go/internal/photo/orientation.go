package photo

import (
	"bytes"
	"encoding/binary"
	"image"
)

// exifHeader は JPEG の APP1 セグメント内で TIFF ストリームの先頭に付く。
var exifHeader = []byte("Exif\x00\x00")

const (
	orientationTag = 0x0112
	tiffTypeShort  = 3
	// maxSegmentScan は、jpegOrientation が諦めるまでにたどる JPEG
	// セグメントの数の上限である。EXIF APP1 は実在するファイルでは先頭付近に
	// あるので、妥当な上限を置けば、病的なセグメント連鎖に対する保護にもなる。
	maxSegmentScan = 32
)

// jpegOrientation は、生の JPEG バイト列から EXIF Orientation の値（1-8）を
// 取り出す。これは意図的な best-effort のパーサである。EXIF が欠けている、
// 切り詰められている、あるいはその他の形で壊れている写真でも受け入れられ
// なければならないので、いかなる想定外の形（APP1 がない、長さが不正、
// オフセットがでたらめ、値が範囲外）でも 1（「補正なし」）を返し、error を
// 返すことも panic することも決してない。通常の fail-loud のルールは、設計上、
// ここには適用されない。すべての読み取りは、入力スライスに対する明示的な
// 長さチェックによって範囲が制限されている。
func jpegOrientation(data []byte) int {
	// SOI マーカー（0xFFD8）はすべての JPEG の先頭にある。
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for seg := 0; seg < maxSegmentScan; seg++ {
		// マーカーの前に 0xFF のフィルバイトが詰められていることがある。
		// マーカーバイトは、0xFF が連続した後の最初の 0xFF 以外のバイト
		// である。
		for i+1 < len(data) && data[i] == 0xFF && data[i+1] == 0xFF {
			i++
		}
		if i+4 > len(data) || data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		// 単独のマーカー（長さフィールドなし）：TEM、RSTn。SOS より前には
		// 現れないはずだが、読み飛ばしておくと走査が正しく保たれる。
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		// SOS：エントロピー符号化された画像データが始まり、APP1 は
		// 見つからなかった。
		if marker == 0xDA {
			return 1
		}
		length := int(data[i+2])<<8 | int(data[i+3])
		if length < 2 || i+2+length > len(data) {
			return 1
		}
		if marker == 0xE1 {
			// APP1 は Exif 以外のペイロード（例：XMP）も運ぶ。Exif ヘッダで
			// 始まるセグメントに対してのみ走査を止める。
			payload := data[i+4 : i+2+length]
			if len(payload) >= len(exifHeader) && bytes.Equal(payload[:len(exifHeader)], exifHeader) {
				return exifOrientation(payload)
			}
		}
		i += 2 + length
	}
	return 1
}

// exifOrientation は、APP1 のペイロード（"Exif\0\0" + TIFF ストリーム）を
// パースし、IFD0 の Orientation タグの値を返す。壊れているものについては 1 を
// 返す。
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
		// count が 1 の SHORT は、値フィールドの先頭 2 バイトに、ストリームの
		// バイトオーダーでインラインに格納される。
		v := int(bo.Uint16(e[8:10]))
		if v < 1 || v > 8 {
			return 1
		}
		return v
	}
	return 1
}

// orient は、EXIF Orientation の値 o（1-8）を受けて、EXIF を無視する
// ビューアでも正立して見えるように img を書き直す。Orientation が 1（および
// 範囲外のもの）の場合は、アロケーションせずに img をそのまま通す。出力は
// 単純な NRGBA のピクセルコピーであり、値が 5-8 のときは幅と高さが入れ替わる。
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
			case 2: // 水平方向に反転
				dx, dy = w-1-x, y
			case 3: // 180 度回転
				dx, dy = w-1-x, h-1-y
			case 4: // 垂直方向に反転
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // 時計回りに 90 度回転
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // 反時計回りに 90 度回転
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
