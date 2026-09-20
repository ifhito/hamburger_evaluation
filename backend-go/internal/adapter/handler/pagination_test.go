package handler_test

import (
	"math"
	"net/http"
	"strings"
	"testing"
)

const (
	pageErrBody    = `{"errors":["Page must be an integer"]}`
	perPageErrBody = `{"errors":["Per page must be an integer"]}`
	bothErrBody    = `{"errors":["Page must be an integer","Per page must be an integer"]}`
)

// paginationEndpoint は、pagination の共有テーブルを流し込む一覧 endpoint である。
// setup は fake を配線した router と、repository の ListShops / ListReviews が
// 呼ばれた回数と最後の limit / offset を返す関数を用意する。どちらの endpoint も、
// 匿名の viewer に見える項目がちょうど 1 件になるよう seed される。
type paginationEndpoint struct {
	name  string
	path  string
	setup func(t *testing.T) (router http.Handler, listed func() (calls int, limit, offset int32))
}

var paginationEndpoints = []paginationEndpoint{
	{
		name: "GET /shops",
		path: "/shops",
		setup: func(t *testing.T) (http.Handler, func() (int, int32, int32)) {
			repo := seedShops(1)
			router, _, _, _ := newShopsRouter(t, repo)
			return router, func() (int, int32, int32) { return repo.listCalls, repo.lastLimit, repo.lastOffset }
		},
	},
	{
		name: "GET /reviews",
		path: "/reviews",
		setup: func(t *testing.T) (http.Handler, func() (int, int32, int32)) {
			repo := seedReviewWorld(1)
			router, aliceAuth, _, _ := newReviewsRouter(t, repo)
			seedFeed(t, router, aliceAuth)
			return router, func() (int, int32, int32) { return len(repo.listFilters), repo.lastLimit, repo.lastOffset }
		},
	},
}

// invalidIntegerValues は、整数として解釈できない query の値（URL エンコード済み）
// である。%20 は空白、%2B は + を表す（リテラルの + は復号されると空白になる）。
var invalidIntegerValues = []struct{ name, raw string }{
	{"英字", "abc"},
	{"小数", "1.5"},
	{"指数表記", "1e3"},
	{"16 進表記", "0x10"},
	{"先頭の空白", "%205"},
	{"末尾の空白", "5%20"},
	{"リテラルの + は空白に復号される", "+5"},
	{"空白だけ", "%20"},
	{"アンダースコア区切り", "1_0"},
	{"符号の重複", "--3"},
	{"符号 + だけ", "%2B"},
	{"符号 - だけ", "-"},
	{"符号の後に英字", "-abc"},
	{"非 ASCII（全）", "%E5%85%A8"},
	{"桁あふれする数字の後に英字", "99999999999999999999abc"},
	{"負の桁あふれする数字の後に英字", "-99999999999999999999x"},
	{"桁あふれする数字の後に小数部", "99999999999999999999.5"},
	{"桁あふれする数字の後に指数", "99999999999999999999e3"},
	{"桁あふれする数字の後に空白", "99999999999999999999%20"},
	{"末尾の改行", "5%0A"},
	{"全角の 5", "%EF%BC%95"},
}

// TestListPaginationInvalidInteger は GET /shops と GET /reviews が同じ入力表で、
// 整数でない page / per_page を 422 で明示的に失敗させ、repository を一度も
// 呼ばないことを扱う。
func TestListPaginationInvalidInteger(t *testing.T) {
	type testCase struct{ name, query, wantBody string }
	var tests []testCase
	for _, v := range invalidIntegerValues {
		tests = append(tests,
			testCase{"page が " + v.name, "?page=" + v.raw, pageErrBody},
			testCase{"per_page が " + v.name, "?per_page=" + v.raw, perPageErrBody},
		)
	}
	tests = append(tests,
		testCase{"page だけ不正なら page のエラーだけが返る", "?page=abc&per_page=5", pageErrBody},
		testCase{"per_page だけ不正なら per_page のエラーだけが返る", "?page=2&per_page=xyz", perPageErrBody},
		testCase{"両方不正なら両方が page、per_page の順で返る", "?page=abc&per_page=1.5", bothErrBody},
		testCase{"query の並びが逆でも page、per_page の順で返る", "?per_page=1.5&page=abc", bothErrBody},
		testCase{"範囲外の整数と不正な値の組では不正な方だけが返る", "?page=99999999999999999999&per_page=abc", perPageErrBody},
	)

	for _, ep := range paginationEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					router, listed := ep.setup(t)
					rec := do(router, http.MethodGet, ep.path+tt.query, "", "")
					if rec.Code != http.StatusUnprocessableEntity {
						t.Fatalf("GET %s%s status = %d, want 422 (body %s)", ep.path, tt.query, rec.Code, rec.Body)
					}
					if got := rec.Body.String(); got != tt.wantBody {
						t.Errorf("body = %s, want %s", got, tt.wantBody)
					}
					if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
						t.Errorf("Content-Type = %q, want application/json", got)
					}
					if calls, _, _ := listed(); calls != 0 {
						t.Errorf("repository was called %d times, want 0 for a 422", calls)
					}
				})
			}
		})
	}
}

// TestListPaginationDefaultsAndClamp は GET /shops と GET /reviews が同じ入力表で、
// 空・省略を既定値として、範囲外の整数（int を超える値を含む）を 422 にせず
// usecase の補正に任せ、repository に渡る limit / offset が期待どおりであることを
// 扱う。各 endpoint の seed は匿名の viewer に 1 件だけ見せるので、offset が 0 を
// 超えるなら本文は [] になり、0 なら 1 件が返る。
func TestListPaginationDefaultsAndClamp(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int32
		wantOffset int32
	}{
		{name: "パラメータの省略は既定値になる", query: "", wantLimit: 20, wantOffset: 0},
		{name: "空の page と per_page は既定値になる", query: "?page=&per_page=", wantLimit: 20, wantOffset: 0},
		{name: "空の page だけなら per_page は有効", query: "?page=&per_page=10", wantLimit: 10, wantOffset: 0},
		{name: "空の per_page だけなら page は有効", query: "?page=2&per_page=", wantLimit: 20, wantOffset: 20},
		{name: "通常の値はそのまま offset に反映される", query: "?page=3&per_page=7", wantLimit: 7, wantOffset: 14},
		{name: "page=0 は先頭ページに補正される", query: "?page=0", wantLimit: 20, wantOffset: 0},
		{name: "page=-3 は先頭ページに補正される", query: "?page=-3", wantLimit: 20, wantOffset: 0},
		{name: "per_page=0 は既定値に補正される", query: "?per_page=0", wantLimit: 20, wantOffset: 0},
		{name: "per_page=-3 は既定値に補正される", query: "?per_page=-3", wantLimit: 20, wantOffset: 0},
		{name: "per_page=1000 は上限 100 に補正される", query: "?per_page=1000", wantLimit: 100, wantOffset: 0},
		// %2B は + である。+5 / +10 は整数として解釈され、0 に丸められない。
		{name: "+5 と +10 は整数として解釈される", query: "?page=%2B5&per_page=%2B10", wantLimit: 10, wantOffset: 40},
		{name: "int を超える正の page は 422 にならず末尾側に補正される", query: "?page=99999999999999999999", wantLimit: 20, wantOffset: math.MaxInt32},
		{name: "int を超える負の page は先頭ページに補正される", query: "?page=-99999999999999999999", wantLimit: 20, wantOffset: 0},
		{name: "int を超える正の per_page は上限 100 に補正される", query: "?per_page=99999999999999999999", wantLimit: 100, wantOffset: 0},
		{name: "int を超える負の per_page は既定値に補正される", query: "?per_page=-99999999999999999999", wantLimit: 20, wantOffset: 0},
		{name: "int64 の最大値", query: "?page=9223372036854775807&per_page=9223372036854775807", wantLimit: 100, wantOffset: math.MaxInt32},
		{name: "int64 の最大値 + 1", query: "?page=9223372036854775808&per_page=9223372036854775808", wantLimit: 100, wantOffset: math.MaxInt32},
		{name: "int64 の最小値", query: "?page=-9223372036854775808&per_page=-9223372036854775808", wantLimit: 20, wantOffset: 0},
		{name: "int64 の最小値 - 1", query: "?page=-9223372036854775809&per_page=-9223372036854775809", wantLimit: 20, wantOffset: 0},
	}

	for _, ep := range paginationEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					router, listed := ep.setup(t)
					rec := do(router, http.MethodGet, ep.path+tt.query, "", "")
					if rec.Code != http.StatusOK {
						t.Fatalf("GET %s%s status = %d, want 200 (body %s)", ep.path, tt.query, rec.Code, rec.Body)
					}
					calls, limit, offset := listed()
					if calls != 1 {
						t.Fatalf("repository was called %d times, want 1", calls)
					}
					if limit != tt.wantLimit || offset != tt.wantOffset {
						t.Errorf("limit/offset = %d/%d, want %d/%d", limit, offset, tt.wantLimit, tt.wantOffset)
					}
					if empty := rec.Body.String() == `[]`; empty != (tt.wantOffset > 0) {
						t.Errorf("body = %s, want an empty page only when offset > 0", rec.Body)
					}
				})
			}
		})
	}
}

// TestListReviewsPaginationWithFilters は GET /reviews で page / per_page の
// 422 が他の filter と併用しても返ること、そして filter も page も不正な場合は
// 既存どおり filter の 422 が先に返ることを扱う。どちらも repository は
// 呼ばれない。
func TestListReviewsPaginationWithFilters(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantBody string
	}{
		{name: "他の filter と併用しても page の 422 が返る", query: "?page=abc&rating=4", wantBody: pageErrBody},
		{name: "有効な filter を全部付けても per_page の 422 が返る", query: "?rating=4&shop_id=1&user_id=1&keyword=x&per_page=1.5", wantBody: perPageErrBody},
		{name: "rating が不正なら page も不正でも rating の 422 が先に返る", query: "?page=abc&rating=abc", wantBody: `{"errors":["Rating must be an integer"]}`},
		{name: "shop_id が不正なら page も不正でも shop_id の 422 が先に返る", query: "?per_page=abc&shop_id=abc", wantBody: `{"errors":["Shop id must be an integer"]}`},
		{name: "user_id が不正なら page も per_page も不正でも user_id の 422 が先に返る", query: "?page=abc&per_page=abc&user_id=abc", wantBody: `{"errors":["User id must be an integer"]}`},
	}

	repo := seedReviewWorld(1)
	router, _, _, _ := newReviewsRouter(t, repo)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/reviews"+tt.query, "", "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("GET /reviews%s status = %d, want 422 (body %s)", tt.query, rec.Code, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
	if len(repo.listFilters) != 0 {
		t.Errorf("ListReviews was called %d times, want 0 for a 422", len(repo.listFilters))
	}
}
