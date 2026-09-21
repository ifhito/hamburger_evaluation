package handler_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// jpegWithDeclaredSize は、ヘッダー(SOF)が指定の寸法を名乗る JPEG を返す。画素のデータは小さな画像の
// ままなので、デコードする前の寸法の確認(ヘッダーだけを見る検査)の試験に使う。
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

// fileWithBrand は、先頭に ftyp ボックス(HEIC や AVIF などの入れ物の形式が、最初に名乗る部分)を持つ
// バイト列を返す。主なブランド(4 文字)で、どの形式を名乗るかが決まる。
func fileWithBrand(brand string) []byte {
	head := []byte("\x00\x00\x00\x18ftyp" + brand + "\x00\x00\x00\x00mif1" + brand)
	return append(head, bytes.Repeat([]byte{0x5A, 0xC3}, 500)...)
}

func TestCreateReviewPhotoRejectionMessagesByCause(t *testing.T) {
	router, _, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(uid.N(1)))
	fields := map[string]string{
		"rating":    "4",
		"comment":   "Tasty",
		"shop_id":   fmt.Sprint(activeShopID),
		"burger_id": fmt.Sprint(cheeseBurgerID),
	}
	const (
		unsupported = `{"errors":["Photo must be a JPEG, PNG, or WebP image"]}`
		heif        = `{"errors":["Photo must be a JPEG, PNG, or WebP image (HEIC/HEIF is not supported)"]}`
		dimensions  = `{"errors":["Photo dimensions are too large (max 10000px per side and 24 megapixels)"]}`
	)

	tests := []struct {
		name  string
		photo []byte
		want  string
	}{
		{"iPhone の既定の形式(HEIC)は、対応しない理由が分かるメッセージになる", fileWithBrand("heic"), heif},
		{"HEIF(主なブランドが mif1)も、同じメッセージになる", fileWithBrand("mif1"), heif},
		{"5 MiB を超える HEIC も、ファイルの大きさではなく、対応しない形式のメッセージになる(残りを読まずに断るため)", append(fileWithBrand("heic"), make([]byte, 6_000_000)...), heif},
		{"AVIF は HEIC ではないので、ふつうの「対応しない形式」のメッセージになる", fileWithBrand("avif"), unsupported},
		{"写真でないファイルは、ふつうの「対応しない形式」のメッセージになる", []byte("just some text, definitely not an image"), unsupported},
		{"横が上限(1 万画素)を超えると名乗る JPEG は、寸法が大きすぎるというメッセージになる(形式の違いとは別)", jpegWithDeclaredSize(t, 12_000, 100), dimensions},
		{"画素数が上限(2,400 万)を超えると名乗る JPEG(1 辺は上限内)も、寸法のメッセージになる", jpegWithDeclaredSize(t, 6_000, 4_500), dimensions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := multipartBody(t, fields, tt.photo)
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
