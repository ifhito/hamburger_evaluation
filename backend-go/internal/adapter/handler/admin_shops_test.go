package handler_test

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// shopStoreFake（shops_test.go で宣言）の moderation 用メソッド群。この fake は
// タイムスタンプを持たないので、id desc が ShopQuery.ListShopsForModeration の
// created_at desc、id desc という順序の代わりを務める。

func (f *shopStoreFake) CreateShop(_ context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	// id は UUID なので、テストで使う uid.N(n) の n の最大値 + 1 を新しい id にする。
	next := 1
	for _, d := range f.shops {
		if n, err := strconv.Atoi(d.ID[24:]); err == nil {
			next = max(next, n+1)
		}
	}
	shop.ID = uid.N(next)
	f.shops = append(f.shops, domain.ShopDetail{Shop: shop})
	return shop, nil
}

func (f *shopStoreFake) ListShopsForModeration(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.ShopDetail, 0, len(f.shops))
	for _, d := range f.shops {
		if status != nil && d.Status != *status {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (f *shopStoreFake) UpdateShopName(_ context.Context, id string, name string) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	for i, d := range f.shops {
		if d.ID == id {
			f.shops[i].Shop.Name = name
			return f.shops[i].Shop, nil
		}
	}
	return domain.Shop{}, domain.ErrShopNotFound
}

func (f *shopStoreFake) UpdateShopStatus(_ context.Context, id string, status domain.ShopStatus, note *string) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	for i, d := range f.shops {
		if d.ID == id {
			f.shops[i].Shop.Status = status
			f.shops[i].Shop.ModerationNote = note
			return f.shops[i].Shop, nil
		}
	}
	return domain.Shop{}, domain.ErrShopNotFound
}

// TestCreateShop は POST /shops を扱う：RequireAuth がこれをゲートし、空または
// 欠落した name は Rails parity の 422 であり、成功時は viewer を creator
// として持つ admin shop の形で 201 を返す。
func TestCreateShop(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		auth       bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "未認証は 401 を返す",
			body:       `{"shop":{"name":"New Shack"}}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Unauthorized"}`,
		},
		{
			name:       "空白のみの name は 422 を返す",
			body:       `{"shop":{"name":"   "}}`,
			auth:       true,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "shop ラッパーがなくても同じ 422 を返す",
			body:       `{}`,
			auth:       true,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "不正な JSON は 400 を返す",
			body:       `{"shop":`,
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "有効な name なら creator 付きの pending な shop を 201 で返す",
			body:       `{"shop":{"name":"New Shack"}}`,
			auth:       true,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":"` + uid.N(4) + `","name":"New Shack","status":"pending","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
			authHeader := ""
			if tt.auth {
				authHeader = aliceAuth
			}
			rec := do(router, http.MethodPost, "/shops", tt.body, authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}

	t.Run("作成した pending な shop は creator の一覧には出るが匿名の一覧には出ない", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		if rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"New Shack"}}`, aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d (body %s)", rec.Code, rec.Body)
		}
		anon := do(router, http.MethodGet, "/shops?keyword=New+Shack", "", "").Body.String()
		if anon != `[]` {
			t.Errorf("anonymous list = %s, want []", anon)
		}
		own := do(router, http.MethodGet, "/shops?keyword=New+Shack", "", aliceAuth).Body.String()
		if want := `[{"id":"` + uid.N(4) + `","name":"New Shack","status":"pending"}]`; own != want {
			t.Errorf("creator list = %s, want %s", own, want)
		}
	})
}

// TestAdminShopsForbidden は、usecase 側の認可の HTTP へのマッピングを
// 固定する。すべての admin エンドポイントは、認証済みの非 admin に対して 403 を
// 返し（shop の存在を探る手段はない）、トークンがなければ 401 を返す。
func TestAdminShopsForbidden(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
		body   string // PUT には整形式の body が必要。不正な JSON は慣例として 400 になる
	}{
		{method: http.MethodGet, path: "/admin/shops"},
		{method: http.MethodPut, path: "/admin/shops/" + uid.N(1), body: `{"shop":{"name":"x"}}`},
		{method: http.MethodPost, path: "/admin/shops/" + uid.N(1) + "/approve"},
		{method: http.MethodPost, path: "/admin/shops/" + uid.N(1) + "/reject"},
	}
	router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
	for _, ep := range endpoints {
		// テスト名は、id の値が変わっても一定になるよう、id を 1 に置き換えた表示にする。
		shown := ep.method + " " + strings.ReplaceAll(ep.path, uid.N(1), "1")
		t.Run(shown+" は非 admin だと 403 になる", func(t *testing.T) {
			rec := do(router, ep.method, ep.path, ep.body, aliceAuth)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusForbidden, rec.Body)
			}
			if got := rec.Body.String(); got != `{"error":"Forbidden"}` {
				t.Errorf("body = %q, want the Forbidden JSON", got)
			}
		})
		t.Run(shown+" は未認証だと 401 になる", func(t *testing.T) {
			rec := do(router, ep.method, ep.path, "", "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnauthorized, rec.Body)
			}
		})
	}

	t.Run("未知の shop id でも非 admin は 403 になる", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(999)+"/approve", "", aliceAuth)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d (existence must not leak)", rec.Code, http.StatusForbidden)
		}
	})
}

// TestAdminListShops は GET /admin/shops を扱う：すべての shop を admin shop の
// 形で新しい順に返すこと、pending/active/rejected の filter、そして未知の
// filter の値が空配列に縮退すること。
func TestAdminListShops(t *testing.T) {
	repo := seedShops(uid.N(1))
	repo.shops[2].Shop.ModerationNote = shopPtr("needs fixes")
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name     string
		query    string
		wantBody string
	}{
		{
			name:  "すべての shop を新しい順に返す",
			query: "",
			wantBody: `[{"id":"` + uid.N(3) + `","name":"Rejected Grill","status":"rejected","moderation_note":"needs fixes","creator":null,"can_approve":true,"can_reject":false},` +
				`{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":true},` +
				`{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","moderation_note":null,"creator":null,"can_approve":false,"can_reject":true}]`,
		},
		{
			name:     "status=pending で絞り込む",
			query:    "?status=pending",
			wantBody: `[{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":true}]`,
		},
		{
			name:     "未知の status は空配列になる",
			query:    "?status=bogus",
			wantBody: `[]`,
		},
		{
			name:  "status が空ならすべての shop を返す",
			query: "?status=",
			wantBody: `[{"id":"` + uid.N(3) + `","name":"Rejected Grill","status":"rejected","moderation_note":"needs fixes","creator":null,"can_approve":true,"can_reject":false},` +
				`{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":true},` +
				`{"id":"` + uid.N(1) + `","name":"Active Diner","status":"active","moderation_note":null,"creator":null,"can_approve":false,"can_reject":true}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/admin/shops"+tt.query, "", adminAuth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestAdminUpdateShop は PUT /admin/shops/{id} を扱う：admin shop の形での
// rename、空の name に対する Rails parity の 422、そして未知の id と非数値の id
// に対する一様な 404。
func TestAdminUpdateShop(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "rename は status を変えずに 200 を返す",
			path:       "/admin/shops/" + uid.N(2),
			body:       `{"shop":{"name":"Renamed Shack"}}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"` + uid.N(2) + `","name":"Renamed Shack","status":"pending","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":true}`,
		},
		{
			name:       "空の name は 422 を返す",
			path:       "/admin/shops/" + uid.N(2),
			body:       `{"shop":{"name":""}}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "未知の id は 404 を返す",
			path:       "/admin/shops/" + uid.N(999),
			body:       `{"shop":{"name":"Renamed"}}`,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"Shop not found"}`,
		},
		{
			name:       "非数値の id は同じ 404 を返す",
			path:       "/admin/shops/abc",
			body:       `{"shop":{"name":"Renamed"}}`,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"Shop not found"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _, adminAuth, _ := newShopsRouter(t, seedShops(uid.N(1)))
			rec := do(router, http.MethodPut, tt.path, tt.body, adminAuth)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestAdminApproveShop は POST /admin/shops/{id}/approve を扱う：status が
// active で note がクリアされた 200（rejected からの再承認でも同様）、
// どのような request body も無視されること、未知の id には 404、そして
// 承認された shop が匿名でも見えるようになること。
func TestAdminApproveShop(t *testing.T) {
	repo := seedShops(uid.N(1))
	repo.shops[2].Shop.ModerationNote = shopPtr("needs fixes")
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	t.Run("rejected の shop を approve して note を消す", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(3)+"/approve", "", adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + uid.N(3) + `","name":"Rejected Grill","status":"active","moderation_note":null,"creator":null,"can_approve":false,"can_reject":true}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("approve された shop は匿名の一覧に出る", func(t *testing.T) {
		got := do(router, http.MethodGet, "/shops?keyword=Rejected+Grill", "", "").Body.String()
		if want := `[{"id":"` + uid.N(3) + `","name":"Rejected Grill","status":"active"}]`; got != want {
			t.Errorf("anonymous list = %s, want %s", got, want)
		}
	})

	t.Run("リクエスト body は無視される", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(2)+"/approve", `not even json`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
	})

	t.Run("UUID の正規形でない id で承認しようとすると、存在しないショップと同じ 404 になる", func(t *testing.T) {
		for _, id := range []string{"1", "abc", upperUUID} {
			rec := do(router, http.MethodPost, "/admin/shops/"+id+"/approve", "", adminAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Shop not found"}` {
				t.Errorf("id %q: status/body = %d %s, want the uniform shop 404", id, rec.Code, rec.Body)
			}
		}
	})

	t.Run("未知の id は 404 を返す", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(999)+"/approve", "", adminAuth)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body)
		}
		if got := rec.Body.String(); got != `{"error":"Shop not found"}` {
			t.Errorf("body = %q, want the uniform shop 404", got)
		}
	})
}

// TestAdminRejectShop は POST /admin/shops/{id}/reject を扱う：トップレベルの
// 任意の moderation_note がそのまま返されること、body が完全に空なら note が
// null で reject されること、そして reject された shop が匿名の一覧から
// 消えること。
func TestAdminRejectShop(t *testing.T) {
	router, _, adminAuth, _ := newShopsRouter(t, seedShops(uid.N(1)))

	t.Run("note 付きで reject する", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(1)+"/reject", `{"moderation_note":"spam"}`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + uid.N(1) + `","name":"Active Diner","status":"rejected","moderation_note":"spam","creator":null,"can_approve":true,"can_reject":false}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("reject された shop は匿名の一覧から消える", func(t *testing.T) {
		got := do(router, http.MethodGet, "/shops?keyword=Active+Diner", "", "").Body.String()
		if got != `[]` {
			t.Errorf("anonymous list = %s, want []", got)
		}
	})

	t.Run("空の body は note を null にして reject する", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(2)+"/reject", "", adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + uid.N(2) + `","name":"Alice Pending","status":"rejected","moderation_note":null,"creator":{"id":"` + uid.N(1) + `","username":"alice"},"can_approve":true,"can_reject":false}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("不正な JSON の body は 400 を返す", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(2)+"/reject", `{"moderation_note":`, adminAuth)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body)
		}
	})

	t.Run("未知の id は 404 を返す", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(999)+"/reject", "", adminAuth)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body)
		}
	})
}
