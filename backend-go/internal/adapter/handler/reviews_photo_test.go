package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jpegBytes は、末尾を 0 で size バイトまで埋めた、デコード可能な JPEG を返す
// （Go の decoder は EOI marker で止まるので、この padding はアップロードを
// 膨らませるだけである）。padding によって、数 MB 級の fixture の構築が安価で
// 決定的になる。
func jpegBytes(t *testing.T, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 80, 60)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	if buf.Len() > size {
		t.Fatalf("base jpeg is %d bytes, larger than the requested %d", buf.Len(), size)
	}
	buf.Write(make([]byte, size-buf.Len()))
	return buf.Bytes()
}

// pngBytes は、小さなデコード可能な PNG を返す。
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 80))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// multipartBody は、フラットなテキストの field と任意の photo part から
// multipart/form-data の body を構築する（各 photo part は image/jpeg の
// "photo.jpg" として宣言される。server は magic bytes を信頼しなければならず、
// この宣言を信頼してはならない）。
func multipartBody(t *testing.T, fields map[string]string, photos ...[]byte) (body *bytes.Buffer, contentType string) {
	t.Helper()
	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	for _, photo := range photos {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="photo"; filename="photo.jpg"`)
		header.Set("Content-Type", "image/jpeg")
		part, err := w.CreatePart(header)
		if err != nil {
			t.Fatalf("create photo part: %v", err)
		}
		if _, err := part.Write(photo); err != nil {
			t.Fatalf("write photo part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, w.FormDataContentType()
}

// doMultipart は multipart の request を router に対して in-process で 1 件
// 実行する。
func doMultipart(router http.Handler, method, path string, body *bytes.Buffer, contentType, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", contentType)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodePhotoURL は review の response body から id と photo_url を取り出す。
func decodePhotoURL(t *testing.T, body []byte) (id int64, photoURL *string) {
	t.Helper()
	var resp struct {
		ID       int64   `json:"id"`
		PhotoURL *string `json:"photo_url"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", body, err)
	}
	return resp.ID, resp.PhotoURL
}

// photoPath は photo_url を、disk root 配下のそのファイルへ対応づける。
func photoPath(t *testing.T, photoDir, photoURL string) string {
	t.Helper()
	key, ok := strings.CutPrefix(photoURL, "/photos/")
	if !ok {
		t.Fatalf("photo_url %q does not start with /photos/", photoURL)
	}
	return filepath.Join(photoDir, filepath.FromSlash(key))
}

// createPhotoReview は、与えられた photo を付けた multipart の review を
// 投稿し、その id と photo_url を返す。
func createPhotoReview(t *testing.T, router http.Handler, auth string, photo []byte) (int64, string) {
	t.Helper()
	fields := map[string]string{
		"rating":    "4",
		"comment":   "Tasty",
		"shop_id":   fmt.Sprint(activeShopID),
		"burger_id": fmt.Sprint(cheeseBurgerID),
	}
	body, contentType := multipartBody(t, fields, photo)
	rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, auth)
	if rec.Code != http.StatusCreated {
		t.Fatalf("multipart create: status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
	}
	id, photoURL := decodePhotoURL(t, rec.Body.Bytes())
	if photoURL == nil {
		t.Fatalf("multipart create: photo_url = null, want non-null (body %s)", rec.Body)
	}
	return id, *photoURL
}

// TestCreateReviewWithPhoto は S10 AC1 を扱う：約 2MB の JPEG を付けた
// multipart の create は、null でない photo_url を伴う 201 を返し、正規化された
// ファイルが disk 上に置かれ、detail エンドポイントが同じ photo_url を
// そのまま返し、photo 自体が GET /photos/ の配下で配信される。
func TestCreateReviewWithPhoto(t *testing.T) {
	router, photoDir, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
	id, photoURL := createPhotoReview(t, router, aliceAuth, jpegBytes(t, 2_000_000))

	if !strings.HasPrefix(photoURL, "/photos/reviews/") || !strings.HasSuffix(photoURL, ".jpg") {
		t.Errorf("photo_url = %q, want /photos/reviews/<random>.jpg", photoURL)
	}
	if _, err := os.Stat(photoPath(t, photoDir, photoURL)); err != nil {
		t.Errorf("stored photo file: %v", err)
	}

	detail := do(router, http.MethodGet, fmt.Sprintf("/reviews/%d", id), "", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d (body %s)", detail.Code, detail.Body)
	}
	if _, got := decodePhotoURL(t, detail.Body.Bytes()); got == nil || *got != photoURL {
		t.Errorf("detail photo_url = %v, want %q", got, photoURL)
	}

	served := do(router, http.MethodGet, photoURL, "", "")
	if served.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d (body %s)", photoURL, served.Code, http.StatusOK, served.Body)
	}
	if served.Body.Len() == 0 {
		t.Error("served photo is empty")
	}
}

// TestCreateReviewPhotoRejections は S10 AC3 を扱う：5MiB を超える画像は 422
// "too large" であり、JPEG のファイル名を持ち image/jpeg の content type が
// 宣言された PDF は 422 "unsupported" である。判定するのは magic bytes であり、
// 宣言では決してない。photo part の重複は不正な body（400）である。
func TestCreateReviewPhotoRejections(t *testing.T) {
	router, _, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
	fields := map[string]string{
		"rating":    "4",
		"comment":   "Tasty",
		"shop_id":   fmt.Sprint(activeShopID),
		"burger_id": fmt.Sprint(cheeseBurgerID),
	}

	t.Run("6MB photo returns 422 too large", func(t *testing.T) {
		body, contentType := multipartBody(t, fields, jpegBytes(t, 6_000_000))
		rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Photo is too large (max 5MB)"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("PDF bytes disguised as JPEG return 422 unsupported", func(t *testing.T) {
		pdf := append([]byte("%PDF-1.4\n"), make([]byte, 2048)...)
		body, contentType := multipartBody(t, fields, pdf)
		rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Photo must be a JPEG, PNG, or WebP image"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("duplicate photo part returns 400", func(t *testing.T) {
		small := jpegBytes(t, 4096)
		body, contentType := multipartBody(t, fields, small, small)
		rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, aliceAuth)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body)
		}
	})

	t.Run("truncated body without the closing boundary returns 400", func(t *testing.T) {
		// field の part は完全だが、最後の boundary の末尾 "--\r\n" が切り
		// 落とされている：HTTP としては完結しているが multipart としては途中で
		// 切れている。Go 1.22 の NextPart はこれを WRAP された io.EOF として
		// 報告する。これを正常な終端として扱うと、photo が黙って捨てられたまま
		// 201 を返してしまう。
		full, contentType := multipartBody(t, fields, jpegBytes(t, 4096))
		raw := full.Bytes()
		truncated := bytes.NewBuffer(raw[:len(raw)-len("--\r\n")])
		rec := doMultipart(router, http.MethodPost, "/reviews", truncated, contentType, aliceAuth)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body)
		}
	})

	t.Run("multipart rating 0 gets the JSON validation message", func(t *testing.T) {
		body, contentType := multipartBody(t, map[string]string{
			"rating":    "not-a-number",
			"comment":   "ok",
			"shop_id":   fmt.Sprint(activeShopID),
			"burger_id": fmt.Sprint(cheeseBurgerID),
		})
		rec := doMultipart(router, http.MethodPost, "/reviews", body, contentType, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Rating must be in 1..5"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})
}

// TestDeleteReviewWithPhoto は S10 AC4 を扱う：photo 付きの review を削除すると
// 204 を返し、disk 上のファイルを best-effort で削除する。
func TestDeleteReviewWithPhoto(t *testing.T) {
	router, photoDir, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
	id, photoURL := createPhotoReview(t, router, aliceAuth, jpegBytes(t, 50_000))
	path := photoPath(t, photoDir, photoURL)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("photo file before delete: %v", err)
	}

	rec := do(router, http.MethodDelete, fmt.Sprintf("/reviews/%d", id), "", aliceAuth)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("photo file after delete: err = %v, want not-exist", err)
	}
}

// TestUpdateReviewPhoto は S10 AC5 と、通常の update では保持するというルールを
// 扱う：新しい photo を付けた multipart の update は photo_url と disk 上の
// ファイルを入れ替え、一方 photo を含まない JSON の update は保存済みの
// photo に手を触れない。
func TestUpdateReviewPhoto(t *testing.T) {
	t.Run("AC5 new photo replaces url and file", func(t *testing.T) {
		router, photoDir, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
		id, oldURL := createPhotoReview(t, router, aliceAuth, jpegBytes(t, 50_000))
		oldPath := photoPath(t, photoDir, oldURL)

		body, contentType := multipartBody(t, map[string]string{"rating": "5", "comment": "Better"}, pngBytes(t))
		rec := doMultipart(router, http.MethodPut, fmt.Sprintf("/reviews/%d", id), body, contentType, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		_, newURL := decodePhotoURL(t, rec.Body.Bytes())
		if newURL == nil {
			t.Fatalf("updated photo_url = null, want non-null (body %s)", rec.Body)
		}
		if *newURL == oldURL {
			t.Errorf("updated photo_url = %q, want a new key", *newURL)
		}
		if !strings.HasSuffix(*newURL, ".png") {
			t.Errorf("updated photo_url = %q, want a .png key for a PNG upload", *newURL)
		}
		if _, err := os.Stat(photoPath(t, photoDir, *newURL)); err != nil {
			t.Errorf("new photo file: %v", err)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old photo file: err = %v, want not-exist", err)
		}
	})

	t.Run("photo-less JSON update keeps the photo", func(t *testing.T) {
		router, photoDir, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
		id, photoURL := createPhotoReview(t, router, aliceAuth, jpegBytes(t, 50_000))

		rec := do(router, http.MethodPut, fmt.Sprintf("/reviews/%d", id),
			`{"review":{"rating":5,"comment":"Still tasty"}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		if _, got := decodePhotoURL(t, rec.Body.Bytes()); got == nil || *got != photoURL {
			t.Errorf("photo_url after plain update = %v, want %q", got, photoURL)
		}
		detail := do(router, http.MethodGet, fmt.Sprintf("/reviews/%d", id), "", "")
		if _, got := decodePhotoURL(t, detail.Body.Bytes()); got == nil || *got != photoURL {
			t.Errorf("detail photo_url after plain update = %v, want %q", got, photoURL)
		}
		if _, err := os.Stat(photoPath(t, photoDir, photoURL)); err != nil {
			t.Errorf("photo file after plain update: %v", err)
		}
	})
}

// TestPhotoTraversal は、/photos の file server が root の外へ出られないことを
// 検証する：エンコードされた ".." のパスは 400/404 を返し、photo dir の外の
// ファイルは決して返さない。
func TestPhotoTraversal(t *testing.T) {
	router, photoDir, _, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
	secret := filepath.Join(filepath.Dir(photoDir), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("write secret fixture: %v", err)
	}

	rec := do(router, http.MethodGet, "/photos/%2e%2e/secret.txt", "", "")
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 400 or 404 (body %s)", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "top secret") {
		t.Error("traversal request leaked the file outside the photo root")
	}

	if rec := do(router, http.MethodGet, "/photos/nope.jpg", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing photo status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestPhotoDirectoryRequests は handler.PhotoFileServer の listing 禁止ルール
// （cmd/api が配線するのと同じラッパー）を固定する。directory への request、
// すなわちマウントの root と、保存済みファイルを含む既存の reviews/
// サブディレクトリは、404 を返し、保存済みの key を決して列挙しない。
func TestPhotoDirectoryRequests(t *testing.T) {
	router, _, aliceAuth, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(1))
	// 保存済みの photo によって、reviews/ サブディレクトリがファイル付きで
	// 存在することが保証される。
	_, photoURL := createPhotoReview(t, router, aliceAuth, jpegBytes(t, 50_000))
	storedName := strings.TrimPrefix(photoURL, "/photos/reviews/")

	for _, path := range []string{"/photos/", "/photos/reviews/"} {
		rec := do(router, http.MethodGet, path, "", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d (body %s)", path, rec.Code, http.StatusNotFound, rec.Body)
		}
		if strings.Contains(rec.Body.String(), storedName) {
			t.Errorf("GET %s leaked the stored key %q in a listing", path, storedName)
		}
	}
}

// TestReviewBodyLimit は S10 の body の上限を固定する：review の書き込み
// エンドポイントは 1 MiB から 6 MiB の body を受け付け（401 は、request が認証
// なしで上限を通過したことを示す）、6 MiB を超える body は 413 で拒否し、
// それ以外のすべての route はグローバルな 1 MiB の上限を保つ。
func TestReviewBodyLimit(t *testing.T) {
	router, _, _, _ := newReviewsRouter(t, seedReviewWorld(1))
	twoMiB := strings.Repeat("a", 2<<20)
	sevenMiB := strings.Repeat("a", 7<<20)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{name: "POST /reviews 2MiB passes the cap", method: http.MethodPost, path: "/reviews", body: twoMiB, wantCode: http.StatusUnauthorized},
		{name: "PUT /reviews/1 2MiB passes the cap", method: http.MethodPut, path: "/reviews/1", body: twoMiB, wantCode: http.StatusUnauthorized},
		{name: "POST /reviews 7MiB is 413", method: http.MethodPost, path: "/reviews", body: sevenMiB, wantCode: http.StatusRequestEntityTooLarge},
		{name: "PUT /reviews/1 7MiB is 413", method: http.MethodPut, path: "/reviews/1", body: sevenMiB, wantCode: http.StatusRequestEntityTooLarge},
		{name: "POST /shops keeps the 1MiB cap", method: http.MethodPost, path: "/shops", body: twoMiB, wantCode: http.StatusRequestEntityTooLarge},
		{name: "POST /signup keeps the 1MiB cap", method: http.MethodPost, path: "/signup", body: twoMiB, wantCode: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, tt.method, tt.path, tt.body, "")
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.wantCode, rec.Body)
			}
		})
	}
}
