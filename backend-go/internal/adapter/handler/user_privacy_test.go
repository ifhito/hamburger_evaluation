package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// auditForbiddenKeyParts は、他のエンドポイントの JSON に現れてはならないキー名の
// 部品（小文字）である。email と admin は「キー自体」が存在してはならず、
// 派生名（email_address、is_admin など）や password_digest も同じ扱いにする。
var auditForbiddenKeyParts = []string{"email", "admin", "password"}

// auditWalk は v を再帰的に走査し（map[string]any と []any を辿る）、次を検証する：
//   - auditForbiddenKeyParts を含むキーがどこにも存在しない
//   - "user" / "creator" キーの null でない値は {id, username} のキーだけを持つ
//
// 見つかったユーザー参照の username を返す（順序は不定なので呼び出し側でソートする）。
// 呼び出し側はこれを期待値と比較するので、走査が空振りして何も検証していない
// テストにはならない。
func auditWalk(t *testing.T, at string, v any) []string {
	t.Helper()
	var usernames []string
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			lower := strings.ToLower(key)
			for _, part := range auditForbiddenKeyParts {
				if strings.Contains(lower, part) {
					t.Errorf("%s: forbidden key %q is present", at, key)
				}
			}
			if (key == "user" || key == "creator") && child != nil {
				ref, ok := child.(map[string]any)
				if !ok {
					t.Errorf("%s.%s = %v (%T), want an object or null", at, key, child, child)
				} else {
					if got := userKeySet(ref); got != publicUserKeys {
						t.Errorf("%s.%s: keys = [%s], want [%s]", at, key, got, publicUserKeys)
					}
					if name, ok := ref["username"].(string); ok {
						usernames = append(usernames, name)
					}
				}
			}
			usernames = append(usernames, auditWalk(t, at+"."+key, child)...)
		}
	case []any:
		for i, child := range x {
			usernames = append(usernames, auditWalk(t, fmt.Sprintf("%s[%d]", at, i), child)...)
		}
	}
	return usernames
}

// TestOtherEndpointsDoNotLeakUserPrivateFields は、user 以外のエンドポイント
// （/reviews、/shops、/admin/shops）が、どの viewer に対しても email と admin を
// 返さないことを、本物の DB と router で固定する（AC8 の監査）。他人の email を
// 持つユーザーが作成者・投稿者として絡んだデータを用意し、レスポンスの JSON を再帰的に
// 走査して、email/admin 系のキーが存在しないこと、body に email 文字列（"@"）が
// 含まれないこと、ユーザー参照が {id, username} だけであることを確かめる。
// 期待する件数とユーザー参照の username も固定するので、空のレスポンスで通ることはない。
func TestOtherEndpointsDoNotLeakUserPrivateFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	const (
		xEmail    = "x-secret@example.com"
		yEmail    = "y-secret@example.com"
		zEmail    = "z-secret@example.com"
		rootEmail = "root-secret@example.com"
	)
	secrets := []string{xEmail, yEmail, zEmail, rootEmail}

	// X は admin の作成者・投稿者、Y は一般の投稿者（pending の shop の作成者）、
	// Z は何も投稿しない一般の viewer、root は moderation 用の admin である。
	xID, xAuth := signupUser(t, router, "xavier", xEmail, "Password123!")
	_, yAuth := signupUser(t, router, "yuki", yEmail, "Password123!")
	_, zAuth := signupUser(t, router, "zoe", zEmail, "Password123!")
	rootID, rootAuth := signupUser(t, router, "root", rootEmail, "Password123!")
	for _, id := range []string{xID, rootID} {
		if _, err := conn.Exec(ctx, `UPDATE users SET admin = true WHERE id = $1`, id); err != nil {
			t.Fatalf("promote user %s: %v", id, err)
		}
	}

	createShop := func(auth, name string) int64 {
		t.Helper()
		rec := do(router, http.MethodPost, "/shops", fmt.Sprintf(`{"shop":{"name":%q}}`, name), auth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create shop %s: status = %d (body %s)", name, rec.Code, rec.Body)
		}
		var resp struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode shop: %v", err)
		}
		return resp.ID
	}
	// X の shop は root が承認して active にする。Y の shop は pending のままにする。
	xShop := createShop(xAuth, "Xavier Active Burgers")
	if rec := do(router, http.MethodPost, fmt.Sprintf("/admin/shops/%d/approve", xShop), "", rootAuth); rec.Code != http.StatusOK {
		t.Fatalf("approve shop: status = %d (body %s)", rec.Code, rec.Body)
	}
	yShop := createShop(yAuth, "Yuki Pending Burgers")

	var burgerID int64
	if err := conn.QueryRow(ctx, `INSERT INTO burgers (name) VALUES ('Audit Burger') RETURNING id`).Scan(&burgerID); err != nil {
		t.Fatalf("insert burger: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, xShop, burgerID); err != nil {
		t.Fatalf("link burger: %v", err)
	}
	postReview := func(auth string, rating int) int64 {
		t.Helper()
		body := fmt.Sprintf(`{"review":{"rating":%d,"comment":"audit review","shop_id":%d,"burger_id":%d}}`, rating, xShop, burgerID)
		rec := do(router, http.MethodPost, "/reviews", body, auth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("post review: status = %d (body %s)", rec.Code, rec.Body)
		}
		var resp struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode review: %v", err)
		}
		return resp.ID
	}
	xReview := postReview(xAuth, 5)
	yReview := postReview(yAuth, 3)

	viewers := []struct {
		name string
		auth string
	}{
		{name: "匿名", auth: ""},
		{name: "投稿者 X (admin)", auth: xAuth},
		{name: "投稿者 Y", auth: yAuth},
		{name: "無関係の一般ユーザー Z", auth: zAuth},
		{name: "admin の root", auth: rootAuth},
	}

	type endpoint struct {
		path string
		// byViewer は viewer の名前をキーにした期待値で、キーがない viewer には
		// defaults を使う。
		defaults auditWant
		byViewer map[string]auditWant
	}
	endpoints := []endpoint{
		{
			path:     "/reviews",
			defaults: auditWant{status: http.StatusOK, items: 2, refs: []string{"xavier", "yuki"}},
		},
		{
			path:     fmt.Sprintf("/reviews/%d", xReview),
			defaults: auditWant{status: http.StatusOK, items: -1, refs: []string{"xavier"}},
		},
		{
			path:     fmt.Sprintf("/reviews/%d", yReview),
			defaults: auditWant{status: http.StatusOK, items: -1, refs: []string{"yuki"}},
		},
		{
			// 一覧の要素には user 参照がない（{id, name, status} だけ）。件数で空振りを防ぐ。
			path:     "/shops",
			defaults: auditWant{status: http.StatusOK, items: 1},
			byViewer: map[string]auditWant{
				"投稿者 X (admin)": {status: http.StatusOK, items: 2},
				"投稿者 Y":         {status: http.StatusOK, items: 2},
				"admin の root":  {status: http.StatusOK, items: 2},
			},
		},
		{
			// active な shop：creator と 2 件の review の author。
			path:     fmt.Sprintf("/shops/%d", xShop),
			defaults: auditWant{status: http.StatusOK, items: -1, refs: []string{"xavier", "xavier", "yuki"}},
		},
		{
			// pending な shop は creator と admin にだけ見える。見えない viewer は 404。
			path:     fmt.Sprintf("/shops/%d", yShop),
			defaults: auditWant{status: http.StatusNotFound, items: -1},
			byViewer: map[string]auditWant{
				"投稿者 X (admin)": {status: http.StatusOK, items: -1, refs: []string{"yuki"}},
				"投稿者 Y":         {status: http.StatusOK, items: -1, refs: []string{"yuki"}},
				"admin の root":  {status: http.StatusOK, items: -1, refs: []string{"yuki"}},
			},
		},
		{
			// admin の一覧は shop 2 件の creator（X と Y）を含む。admin でなければ 403/401。
			path:     "/admin/shops",
			defaults: auditWant{status: http.StatusForbidden, items: -1},
			byViewer: map[string]auditWant{
				"匿名":            {status: http.StatusUnauthorized, items: -1},
				"投稿者 X (admin)": {status: http.StatusOK, items: 2, refs: []string{"xavier", "yuki"}},
				"admin の root":  {status: http.StatusOK, items: 2, refs: []string{"xavier", "yuki"}},
			},
		},
	}

	for _, ep := range endpoints {
		for _, v := range viewers {
			want, ok := ep.byViewer[v.name]
			if !ok {
				want = ep.defaults
			}
			t.Run(fmt.Sprintf("GET %s を %s が取得しても email と admin は現れない", ep.path, v.name), func(t *testing.T) {
				rec := do(router, http.MethodGet, ep.path, "", v.auth)
				if rec.Code != want.status {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, want.status, rec.Body)
				}
				body := rec.Body.String()
				for _, secret := range secrets {
					if strings.Contains(body, secret) {
						t.Errorf("body contains the email %q: %s", secret, body)
					}
				}
				if strings.Contains(body, "@") {
					t.Errorf("body contains an email-like string: %s", body)
				}

				var decoded any
				if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
					t.Fatalf("body %q is not valid JSON: %v", body, err)
				}
				if list, isList := decoded.([]any); isList {
					if want.items >= 0 && len(list) != want.items {
						t.Errorf("items = %d, want %d (body %s)", len(list), want.items, body)
					}
				} else if want.items >= 0 {
					t.Errorf("body = %s, want a JSON array of %d items", body, want.items)
				}

				got := auditWalk(t, "$", decoded)
				sort.Strings(got)
				if strings.Join(got, ",") != strings.Join(want.refs, ",") {
					t.Errorf("user references = %v, want %v (body %s)", got, want.refs, body)
				}
			})
		}
	}
}

// auditWant は監査テストの 1 リクエストあたりの期待値である。
type auditWant struct {
	status int
	// items は、トップレベルが配列のときの要素数である。-1 は要素数を検証しない
	// （オブジェクトまたはエラー応答を想定）ことを表す。
	items int
	// refs は、走査で見つかるべきユーザー参照の username（ソート済み）である。
	refs []string
}
