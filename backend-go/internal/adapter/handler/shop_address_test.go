package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// shopAddressBody は、ショップの応答のうち住所の 3 項目である。
type shopAddressBody struct {
	PrefectureCode *int   `json:"prefecture_code"`
	City           string `json:"city"`
	StreetAddress  string `json:"street_address"`
}

// testAddress は、テストの住所を domain.NewAddress で作る(不正ならテストを止める)。
func testAddress(t *testing.T, prefectureCode *int, city, streetAddress string) domain.Address {
	t.Helper()
	address, err := domain.NewAddress(prefectureCode, city, streetAddress)
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}
	return address
}

func decodeAddress(t *testing.T, body []byte) shopAddressBody {
	t.Helper()
	var got shopAddressBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, body)
	}
	return got
}

// TestCreateShopAddress は、POST /shops の住所(都道府県のコード・市区町村・番地以降)を扱う。
func TestCreateShopAddress(t *testing.T) {
	t.Run("住所つきで申請すると、201 の応答とショップの詳細に同じ住所が入る", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		rec := do(router, http.MethodPost, "/shops",
			`{"shop":{"name":"Shibuya Burger","prefecture_code":13,"city":"渋谷区","street_address":"神南1-2-3"}}`, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		want := shopAddressBody{PrefectureCode: shopPtr(13), City: "渋谷区", StreetAddress: "神南1-2-3"}
		if got := decodeAddress(t, rec.Body.Bytes()); *got.PrefectureCode != 13 || got.City != want.City || got.StreetAddress != want.StreetAddress {
			t.Errorf("作成の応答の住所 = %+v, want %+v", got, want)
		}
		detail := do(router, http.MethodGet, "/shops/"+uid.N(4), "", aliceAuth)
		if detail.Code != http.StatusOK {
			t.Fatalf("detail status = %d (body %s)", detail.Code, detail.Body)
		}
		if got := decodeAddress(t, detail.Body.Bytes()); got.PrefectureCode == nil || *got.PrefectureCode != 13 || got.City != want.City || got.StreetAddress != want.StreetAddress {
			t.Errorf("詳細の住所 = %+v, want %+v", got, want)
		}
	})

	t.Run("住所なしで申請すると、都道府県は null・市区町村と番地以降は空文字になる", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"No Address"}}`, aliceAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"prefecture_code":null,"city":"","street_address":""`) {
			t.Errorf("body = %s, want 未設定の住所", rec.Body)
		}
	})

	t.Run("null と空文字の住所は、未設定として受け付ける", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"Nulls","prefecture_code":null,"city":null,"street_address":""}}`, aliceAuth)
		if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"prefecture_code":null,"city":"","street_address":""`) {
			t.Errorf("status/body = %d %s, want 201 と未設定の住所", rec.Code, rec.Body)
		}
	})

	for _, code := range []string{`0`, `48`, `"abc"`, `1.5`, `"13"`, `true`} {
		t.Run("都道府県のコードが "+code+" なら 422 で、ショップは作られない", func(t *testing.T) {
			repo := seedShops(uid.N(1))
			router, aliceAuth, _, _ := newShopsRouter(t, repo)
			rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"Bad Prefecture","prefecture_code":`+code+`}}`, aliceAuth)
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != `{"errors":["Prefecture is invalid"]}` {
				t.Errorf("status/body = %d %s, want 422 Prefecture is invalid", rec.Code, rec.Body)
			}
			if len(repo.shops) != 3 {
				t.Errorf("shops = %d 件, want 作られていない(3 件のまま)", len(repo.shops))
			}
		})
	}

	t.Run("市区町村と番地以降が両方とも上限を超えると、両方の違反を 422 で列挙する", func(t *testing.T) {
		repo := seedShops(uid.N(1))
		router, aliceAuth, _, _ := newShopsRouter(t, repo)
		body := `{"shop":{"name":"Long","city":"` + strings.Repeat("区", domain.MaxCityChars+1) +
			`","street_address":"` + strings.Repeat("丁", domain.MaxStreetAddressChars+1) + `"}}`
		rec := do(router, http.MethodPost, "/shops", body, aliceAuth)
		want := `{"errors":["City is too long (maximum is 100 characters)","Street address is too long (maximum is 200 characters)"]}`
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
			t.Errorf("status/body = %d %s, want 422 %s", rec.Code, rec.Body, want)
		}
		if len(repo.shops) != 3 {
			t.Errorf("shops = %d 件, want 作られていない(3 件のまま)", len(repo.shops))
		}
	})

	t.Run("日本語を求めると、住所の違反も日本語で返す", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		req := httptest.NewRequest(http.MethodPost, "/shops", strings.NewReader(`{"shop":{"name":"Bad","prefecture_code":99}}`))
		req.Header.Set("Authorization", aliceAuth)
		req.Header.Set("Accept-Language", "ja")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "都道府県の指定が正しくありません") {
			t.Errorf("status/body = %d %s, want 日本語の 422", rec.Code, rec.Body)
		}
	})

	t.Run("未ログインの申請は、住所があっても 401 になる", func(t *testing.T) {
		router, _, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"Anon","prefecture_code":13}}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

// TestAdminUpdateShopAddress は、PUT /admin/shops/{id} の住所の部分更新を扱う: 送った項目だけが変わり、
// null を送ると消える。
func TestAdminUpdateShopAddress(t *testing.T) {
	seeded := func() *shopStoreFake {
		repo := seedShops(uid.N(1))
		repo.shops[1].Shop.Address = testAddress(t, shopPtr(13), "渋谷区", "神南1-2-3")
		return repo
	}

	t.Run("市区町村だけを送ると、市区町村だけが変わる", func(t *testing.T) {
		repo := seeded()
		router, _, adminAuth, _ := newShopsRouter(t, repo)
		rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"Alice Pending","city":"新宿区"}}`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		got := decodeAddress(t, rec.Body.Bytes())
		if got.PrefectureCode == nil || *got.PrefectureCode != 13 || got.City != "新宿区" || got.StreetAddress != "神南1-2-3" {
			t.Errorf("住所 = %+v, want 13・新宿区・神南1-2-3", got)
		}
		if saved := repo.shops[1].Shop.Address; saved != testAddress(t, shopPtr(13), "新宿区", "神南1-2-3") {
			t.Errorf("保存された住所 = %+v", saved)
		}
	})

	t.Run("都道府県に null を送ると、都道府県だけが消える", func(t *testing.T) {
		router, _, adminAuth, _ := newShopsRouter(t, seeded())
		rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"Alice Pending","prefecture_code":null}}`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if got := decodeAddress(t, rec.Body.Bytes()); got.PrefectureCode != nil || got.City != "渋谷区" || got.StreetAddress != "神南1-2-3" {
			t.Errorf("住所 = %+v, want null・渋谷区・神南1-2-3", got)
		}
	})

	t.Run("番地以降に空文字を送ると消える", func(t *testing.T) {
		router, _, adminAuth, _ := newShopsRouter(t, seeded())
		rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"Alice Pending","street_address":""}}`, adminAuth)
		if got := decodeAddress(t, rec.Body.Bytes()); rec.Code != http.StatusOK || got.StreetAddress != "" || got.City != "渋谷区" {
			t.Errorf("status/住所 = %d %+v, want 200・番地以降だけが空", rec.Code, got)
		}
	})

	t.Run("範囲外の都道府県は 422 で、何も変わらない", func(t *testing.T) {
		repo := seeded()
		router, _, adminAuth, _ := newShopsRouter(t, repo)
		rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"Renamed","prefecture_code":48,"city":"新宿区"}}`, adminAuth)
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != `{"errors":["Prefecture is invalid"]}` {
			t.Errorf("status/body = %d %s, want 422 Prefecture is invalid", rec.Code, rec.Body)
		}
		if saved := repo.shops[1].Shop; saved.Name != "Alice Pending" || saved.Address.City() != "渋谷区" {
			t.Errorf("保存された shop = %+v, want 変わらない", saved)
		}
	})

	t.Run("管理者でなければ、住所の更新も 403 になる", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seeded())
		rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"Alice Pending","city":"新宿区"}}`, aliceAuth)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})
}

// TestListShopsPrefectureFilter は、GET /shops の prefecture_code の絞り込みの入口を扱う。
func TestListShopsPrefectureFilter(t *testing.T) {
	repo := seedShops(uid.N(1))
	repo.shops = append(repo.shops,
		domain.ShopDetail{Shop: domain.Shop{ID: uid.N(4), Name: "Tokyo Burger", Status: domain.ShopStatusActive, Address: testAddress(t, shopPtr(13), "渋谷区", "")}},
		domain.ShopDetail{Shop: domain.Shop{ID: uid.N(5), Name: "Osaka Burger", Status: domain.ShopStatusActive, Address: testAddress(t, shopPtr(27), "", "")}},
	)
	router, _, _, _ := newShopsRouter(t, repo)

	t.Run("prefecture_code=13 は東京都のショップだけを返し、住所を含む", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops?prefecture_code=13", "", "")
		want := `[{"id":"` + uid.N(4) + `","name":"Tokyo Burger","status":"active","map_url":null,"closed_at":null,"prefecture_code":13,"city":"渋谷区","street_address":"","photo_url":null,"average_rating":null,"review_count":0}]`
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("status/body = %d %s, want 200 %s", rec.Code, rec.Body, want)
		}
	})

	t.Run("空の prefecture_code は絞り込まない", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/shops?prefecture_code=", "", "")
		var items []json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &items); rec.Code != http.StatusOK || err != nil || len(items) != 3 {
			t.Errorf("status/body = %d %s, want 200 で active な 3 件", rec.Code, rec.Body)
		}
	})

	for _, raw := range []string{"abc", "0", "48", "1.5"} {
		t.Run("prefecture_code="+raw+" は 422 になる", func(t *testing.T) {
			rec := do(router, http.MethodGet, "/shops?prefecture_code="+raw, "", "")
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != `{"errors":["Prefecture is invalid"]}` {
				t.Errorf("status/body = %d %s, want 422 Prefecture is invalid", rec.Code, rec.Body)
			}
		})
	}
}

// TestMCPListShopsPrefecture は、MCP の list_shops の都道府県の絞り込みが、REST と同じ検証・同じ文言で、
// 住所つきのショップを返すことを確かめる。
func TestMCPListShopsPrefecture(t *testing.T) {
	k := newMCPKit(t)
	k.shops.shops[0].Shop.Address = testAddress(t, shopPtr(13), "渋谷区", "神南1-2-3")
	k.shops.shops = append(k.shops.shops, domain.ShopDetail{Shop: domain.Shop{ID: uid.N(50), Name: "Osaka Burger", Status: domain.ShopStatusActive,
		Address: testAddress(t, shopPtr(27), "", "")}})
	session := k.connect(t, k.token(k.alice, readScope))

	t.Run("prefecture_code 13 は東京都のショップだけを、住所つきで返す", func(t *testing.T) {
		text, isErr := call(t, session, "list_shops", map[string]any{"prefecture_code": 13})
		if isErr {
			t.Fatalf("list_shops failed: %s", text)
		}
		var got struct {
			Items []struct {
				Name string `json:"name"`
				shopAddressBody
			} `json:"items"`
		}
		mustJSON(t, text, &got)
		if len(got.Items) != 1 || got.Items[0].Name != "Active Diner" {
			t.Fatalf("items = %s, want 東京都の Active Diner だけ", text)
		}
		if a := got.Items[0].shopAddressBody; a.PrefectureCode == nil || *a.PrefectureCode != 13 || a.City != "渋谷区" || a.StreetAddress != "神南1-2-3" {
			t.Errorf("住所 = %+v, want 13・渋谷区・神南1-2-3", a)
		}
	})

	t.Run("範囲外の prefecture_code は REST と同じ文言のエラーになる", func(t *testing.T) {
		text, isErr := call(t, session, "list_shops", map[string]any{"prefecture_code": 48})
		if !isErr || text != "Prefecture is invalid" {
			t.Errorf("list_shops = %q (isError=%v), want Prefecture is invalid", text, isErr)
		}
	})
}

// TestMCPSubmitShopAddress は、MCP の submit_shop が、住所の 3 項目を受け取って保存し、範囲外の都道府県は
// REST と同じ文言で拒否して、ショップを作らないことを確かめる。
func TestMCPSubmitShopAddress(t *testing.T) {
	k := newMCPKit(t)
	session := k.connect(t, k.token(k.bob, readScope, writeScope))

	t.Run("都道府県・市区町村・番地以降を渡すと、応答に同じ住所が入る", func(t *testing.T) {
		text, isErr := call(t, session, "submit_shop", map[string]any{"name": "住所つきの店", "prefecture_code": 13, "city": "渋谷区", "street_address": "神南1-2-3"})
		if isErr {
			t.Fatalf("submit_shop failed: %s", text)
		}
		var got shopAddressBody
		mustJSON(t, text, &got)
		if got.PrefectureCode == nil || *got.PrefectureCode != 13 || got.City != "渋谷区" || got.StreetAddress != "神南1-2-3" {
			t.Errorf("住所 = %+v, want 13・渋谷区・神南1-2-3", got)
		}
	})

	t.Run("範囲外の都道府県は REST と同じ文言のエラーになり、ショップは作られない", func(t *testing.T) {
		before := len(k.shops.shops)
		text, isErr := call(t, session, "submit_shop", map[string]any{"name": "都道府県が不正な店", "prefecture_code": 48})
		if !isErr || text != "Prefecture is invalid" {
			t.Errorf("submit_shop = %q (isError=%v), want Prefecture is invalid", text, isErr)
		}
		if after := len(k.shops.shops); after != before {
			t.Errorf("ショップの件数 = %d, want %d のまま", after, before)
		}
	})
}
