package handler_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// heicFixture は photo パッケージの testdata の HEIC を読む。どちらも 640x480 の 4 色の絵で、
// portrait_irot.heic は「90 度回転して表示する」情報つき(iPhone の縦向きの写真と同じ作り)。
func heicFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "photo", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// withDeclaredSize は、HEIC の画像寸法の宣言(ispe ボックス)だけを書き換えた複製を返す。
func withDeclaredSize(t *testing.T, data []byte, width, height uint32) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	i := bytes.Index(out, []byte("ispe"))
	if i < 0 {
		t.Fatal("ispe box not found")
	}
	binary.BigEndian.PutUint32(out[i+8:], width)
	binary.BigEndian.PutUint32(out[i+12:], height)
	return out
}

func photoFields() map[string]string {
	return map[string]string{
		"rating":    "4",
		"comment":   "Tasty",
		"shop_id":   fmt.Sprint(activeShopID),
		"burger_id": fmt.Sprint(cheeseBurgerID),
	}
}

func TestCreateReviewWithHEICPhoto(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		width, height int
	}{
		{"向きの情報がない HEIC は、そのままの向きの JPEG で保存される", "landscape.heic", 640, 480},
		{"回転して表示する情報つきの HEIC(iPhone の縦向き)は、縦向きの JPEG で保存される", "portrait_irot.heic", 480, 640},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, photoDir, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(uid.N(1)))
			_, photoURL := createPhotoReview(t, router, aliceAuth, heicFixture(t, tt.fixture))

			if !bytes.HasSuffix([]byte(photoURL), []byte(".jpg")) {
				t.Errorf("photo_url = %q, want .jpg で終わる(HEIC は JPEG に変換して保存する)", photoURL)
			}
			stored, err := os.ReadFile(photoPath(t, photoDir, photoURL))
			if err != nil {
				t.Fatalf("保存されたファイルを読めない: %v", err)
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(stored))
			if err != nil {
				t.Fatalf("保存されたファイルが JPEG として読めない: %v", err)
			}
			if cfg.Width != tt.width || cfg.Height != tt.height {
				t.Errorf("保存された大きさ = %dx%d, want %dx%d", cfg.Width, cfg.Height, tt.width, tt.height)
			}
		})
	}
}

// jpegWithDeclaredSize は、ヘッダー(SOF)が指定の寸法を名乗る JPEG を返す。画素のデータは小さな画像の
// ままなので、ヘッダーだけを検査する処理(デコードの前の寸法の確認)の試験に使う。
func jpegWithDeclaredSize(t *testing.T, width, height uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	data := buf.Bytes()
	i := bytes.Index(data, []byte{0xFF, 0xC0}) // ベースラインの SOF0 マーカー
	if i < 0 {
		t.Fatal("SOF0 marker not found")
	}
	// マーカーの後ろに、長さ(2)、精度(1)、高さ(2)、幅(2)が並ぶ。
	binary.BigEndian.PutUint16(data[i+5:], height)
	binary.BigEndian.PutUint16(data[i+7:], width)
	return data
}

func TestCreateReviewPhotoRejectionsByCause(t *testing.T) {
	router, _, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(uid.N(1)))
	const (
		unsupported = `{"errors":["Photo must be a JPEG, PNG, WebP, or HEIC image"]}`
		dimensions  = `{"errors":["Photo dimensions are too large (max 10000px per side and 24 megapixels, 16 megapixels for HEIC)"]}`
		tooLarge    = `{"errors":["Photo is too large (max 5MB)"]}`
	)
	full := heicFixture(t, "landscape.heic")
	// 5 MiB を超える HEIC(先頭は本物のヘッダーで、後ろを 0 で埋めただけ)。
	oversize := append(append([]byte(nil), full...), make([]byte, 6_000_000)...)
	avif := append([]byte(nil), full...)
	copy(avif[8:12], "avif") // 主なブランドを AVIF に変える
	copy(avif[16:20], "avif")
	copy(avif[20:24], "avif")

	tests := []struct {
		name  string
		photo []byte
		want  string
	}{
		{"HEIC が 5 MiB を超えると、ファイルが大きすぎるというメッセージになる", oversize, tooLarge},
		{"画像の寸法を上限より大きく名乗る HEIC は、寸法が大きすぎるというメッセージになる", withDeclaredSize(t, full, 30_000, 30_000), dimensions},
		{"画素数が HEIC の上限を超える HEIC(1 辺は上限内)も、寸法のメッセージになる", withDeclaredSize(t, full, 5_000, 3_300), dimensions},
		{"寸法を上限より大きく名乗る JPEG も、寸法が大きすぎるというメッセージになる(形式の違いとは別のメッセージ)", jpegWithDeclaredSize(t, 12_000, 9_000), dimensions},
		{"AVIF は、対応しない形式というメッセージになる", avif, unsupported},
		{"途中で切れた HEIC は、対応しない形式(壊れている)というメッセージになる", full[:120], unsupported},
		{"HEIC のふりをした乱数は、対応しない形式というメッセージになる", append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), bytes.Repeat([]byte{0x5A, 0xC3}, 500)...), unsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := multipartBody(t, photoFields(), tt.photo)
			rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, aliceAuth)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
			}
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}
