package photo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// heicFixture は、testdata の HEIC を読み込む。どちらも 640x480 で、左上が赤・右上が緑・
// 左下が青・右下が黄の 4 色の絵である(向きが正しいかを、隅の色で確かめられる)。
//   - landscape.heic: 向きの情報がない、ふつうの横向きの写真
//   - portrait_irot.heic: 横向きの画素に「90 度回転して表示する」という情報(irot)が付いた、
//     iPhone の縦向きの写真と同じ作り。正しくデコードすると、480x640 の縦向きで、左上が青・右上が赤・
//     左下が黄・右下が緑になる
func heicFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// dominantColor は、画素を 4 色のどれに近いかで分類して名前で返す(JPEG の劣化に強くするため)。
func dominantColor(c color.Color) string {
	r, g, b, _ := c.RGBA()
	r, g, b = r>>8, g>>8, b>>8
	switch {
	case r > 170 && g < 110 && b < 110:
		return "red"
	case g > 110 && r < 110 && b < 110:
		return "green"
	case b > 170 && r < 110 && g < 110:
		return "blue"
	case r > 170 && g > 170 && b < 110:
		return "yellow"
	}
	return "other"
}

// cornerColors は、画像の四隅の少し内側の色を、左上・右上・左下・右下の順に返す。
func cornerColors(img image.Image) [4]string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	at := func(x, y int) string { return dominantColor(img.At(b.Min.X+x, b.Min.Y+y)) }
	return [4]string{at(w*3/8, h*3/8), at(w*5/8, h*3/8), at(w*3/8, h*5/8), at(w*5/8, h*5/8)}
}

// patchSize は、HEIC の中の画像寸法の宣言(ispe ボックス)を書き換えた複製を返す。実際の画素データは
// そのままなので、「ヘッダーだけ大きな寸法を名乗る細工したファイル」の再現に使う。
func patchSize(t *testing.T, data []byte, width, height uint32) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	i := bytes.Index(out, []byte("ispe"))
	if i < 0 {
		t.Fatal("ispe box not found in fixture")
	}
	// ispe の種類名の後ろに、版とフラグ(4 バイト)、横(4 バイト)、縦(4 バイト)が続く。
	binary.BigEndian.PutUint32(out[i+8:], width)
	binary.BigEndian.PutUint32(out[i+12:], height)
	return out
}

func process(t *testing.T, data []byte) (photo.Processed, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return photo.Process(ctx, bytes.NewReader(data))
}

func TestProcessHEIC(t *testing.T) {
	t.Run("向きの情報がない横向きの HEIC は、JPEG に変換され、絵の向きも保たれる", func(t *testing.T) {
		got, err := process(t, heicFixture(t, "landscape.heic"))
		if err != nil {
			t.Fatalf("Process returned error: %v", err)
		}
		if got.ContentType != "image/jpeg" || got.Ext != ".jpg" {
			t.Errorf("出力 = %s %s, want image/jpeg .jpg", got.ContentType, got.Ext)
		}
		img, err := jpeg.Decode(bytes.NewReader(got.Data))
		if err != nil {
			t.Fatalf("出力が JPEG として読めない: %v", err)
		}
		if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
			t.Errorf("大きさ = %dx%d, want 640x480", b.Dx(), b.Dy())
		}
		if want := [4]string{"red", "green", "blue", "yellow"}; cornerColors(img) != want {
			t.Errorf("四隅の色 = %v, want %v", cornerColors(img), want)
		}
	})

	t.Run("90 度回転して表示する情報つきの HEIC は、縦向きの正しい向きで保存され、二重に回転しない", func(t *testing.T) {
		got, err := process(t, heicFixture(t, "portrait_irot.heic"))
		if err != nil {
			t.Fatalf("Process returned error: %v", err)
		}
		img, err := jpeg.Decode(bytes.NewReader(got.Data))
		if err != nil {
			t.Fatalf("出力が JPEG として読めない: %v", err)
		}
		if b := img.Bounds(); b.Dx() != 480 || b.Dy() != 640 {
			t.Errorf("大きさ = %dx%d, want 480x640(縦向き)", b.Dx(), b.Dy())
		}
		if want := [4]string{"blue", "red", "yellow", "green"}; cornerColors(img) != want {
			t.Errorf("四隅の色 = %v, want %v(横向きの絵を、時計回りに 90 度回した向き)", cornerColors(img), want)
		}
	})

	t.Run("画像の寸法を上限より大きく名乗る HEIC は、デコードする前に断られ、大きなメモリを確保しない", func(t *testing.T) {
		tests := []struct {
			name          string
			width, height uint32
		}{
			{"1 辺が上限(1 万画素)を超える", 10_001, 100},
			{"画素数が HEIC の上限(1,600 万)を超える", 5_000, 3_300},
			{"とても大きい", 60_000, 60_000},
			{"32 ビットの範囲いっぱい", 0xFFFFFFFF, 0xFFFFFFFF},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data := patchSize(t, heicFixture(t, "landscape.heic"), tt.width, tt.height)
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				_, err := process(t, data)
				runtime.ReadMemStats(&after)
				if !errors.Is(err, photo.ErrDimensionsTooLarge) || !errors.Is(err, photo.ErrUnsupportedImage) {
					t.Fatalf("Process error = %v, want ErrDimensionsTooLarge(ErrUnsupportedImage でもある)", err)
				}
				// デコーダ(WASM。1 回のデコードで数百 MiB を使う)を呼んでいれば、この値では収まらない。
				if delta := after.TotalAlloc - before.TotalAlloc; delta > 8<<20 {
					t.Errorf("断るまでに %d バイトを確保した。デコーダを呼ばずに断るはず", delta)
				}
			})
		}
	})

	t.Run("上限ちょうどの寸法を名乗る HEIC は、寸法では断られない", func(t *testing.T) {
		// 宣言だけを書き換えたので、デコードは失敗しうる。ここで確かめるのは、寸法の理由では断られないこと。
		data := patchSize(t, heicFixture(t, "landscape.heic"), 4_000, 4_000)
		_, err := process(t, data)
		if errors.Is(err, photo.ErrDimensionsTooLarge) {
			t.Fatalf("1,600 万画素ちょうどが寸法で断られた: %v", err)
		}
	})

	t.Run("AVIF や、画像の連続(動画)の名乗りのファイルは、対応しない形式として断られる", func(t *testing.T) {
		tests := []struct {
			name    string
			replace map[string]string
		}{
			{"AVIF", map[string]string{"heic": "avif"}},
			{"AVIF の連続", map[string]string{"heic": "avis"}},
			{"画像の連続(msf1)だけ", map[string]string{"heic": "msf1", "mif1": "msf1", "miaf": "msf1"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data := heicFixture(t, "landscape.heic")
				// ftyp ボックス(先頭の 32 バイトほど)の中のブランドだけを書き換える。
				head := append([]byte(nil), data[:32]...)
				for from, to := range tt.replace {
					head = bytes.ReplaceAll(head, []byte(from), []byte(to))
				}
				data = append(head, data[32:]...)
				_, err := process(t, data)
				if !errors.Is(err, photo.ErrUnsupportedImage) || errors.Is(err, photo.ErrDimensionsTooLarge) {
					t.Fatalf("Process error = %v, want ErrUnsupportedImage(寸法の理由ではない)", err)
				}
			})
		}
	})

	t.Run("壊れた HEIC は、対応しない画像として断られ、プロセスは落ちない", func(t *testing.T) {
		full := heicFixture(t, "landscape.heic")
		tests := []struct {
			name string
			data []byte
		}{
			{"ftyp だけで終わる", full[:28]},
			{"途中で切れている(meta の途中)", full[:80]},
			{"画素のデータの手前で切れている", full[:len(full)-200]},
			{"ftyp のあとが 0 で埋まっている", append(append([]byte(nil), full[:28]...), make([]byte, 400)...)},
			{"ftyp を名乗るだけの乱数", append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), bytes.Repeat([]byte{0xA7, 0x3C, 0x91}, 300)...)},
			{"ボックスの大きさが不正(外側を超える)", func() []byte {
				d := append([]byte(nil), full...)
				binary.BigEndian.PutUint32(d[28:], 0x7FFFFFFF) // meta の大きさ
				return d
			}()},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := process(t, tt.data)
				if !errors.Is(err, photo.ErrUnsupportedImage) {
					t.Fatalf("Process error = %v, want ErrUnsupportedImage", err)
				}
			})
		}
	})

	t.Run("1 バイトずつ書き換えた HEIC を大量に与えても、パニックも停止もせず、成功か「対応しない画像」のどちらかになる", func(t *testing.T) {
		base := heicFixture(t, "portrait_irot.heic")
		// 場所をばらして 96 か所。各回は約 10 ミリ秒(WASM のデコード)で、-race でも数秒に収まる。
		for n := 0; n < 96; n++ {
			data := append([]byte(nil), base...)
			pos := (n*2654435761 + 17) % len(data) // 位置を疑似乱数でばらす(結果が毎回同じになる)
			data[pos] ^= byte(1 << (n % 8))
			got, err := process(t, data)
			switch {
			case err == nil:
				if _, derr := jpeg.Decode(bytes.NewReader(got.Data)); derr != nil {
					t.Fatalf("変異 %d(位置 %d): 成功したが出力が JPEG として読めない: %v", n, pos, derr)
				}
			case errors.Is(err, photo.ErrUnsupportedImage):
			default:
				t.Fatalf("変異 %d(位置 %d): 想定外のエラー: %v", n, pos, err)
			}
		}
	})

	t.Run("HEIC でない ftyp のないデータや、JPEG・PNG・WebP は、これまでどおり扱われる", func(t *testing.T) {
		if _, err := process(t, []byte("not an image at all, just text")); !errors.Is(err, photo.ErrUnsupportedImage) {
			t.Errorf("文字列: err = %v, want ErrUnsupportedImage", err)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 50, 40)), nil); err != nil {
			t.Fatal(err)
		}
		got, err := process(t, buf.Bytes())
		if err != nil || got.ContentType != "image/jpeg" {
			t.Errorf("JPEG: got=%v err=%v, want image/jpeg", got.ContentType, err)
		}
	})
}
