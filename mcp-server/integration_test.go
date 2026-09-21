//go:build integration

// 実 API と実 DB に対する結合テスト。scripts/integration.sh が、隔離した Docker の環境
// （docker-compose.integration.yml）を立てて走らせる。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// seedPassword は cmd/seed が作る開発用ユーザー全員に共通のパスワードである（秘密ではない）。
const seedPassword = "Password123!"

func integrationAPIURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("HAMBURGER_IT_API_URL")
	if url == "" {
		t.Fatal("HAMBURGER_IT_API_URL が未設定（scripts/integration.sh から実行する）")
	}
	return url
}

func integrationConfig(t *testing.T, token string, write bool) config {
	return config{apiURL: integrationAPIURL(t), token: token, allowWrite: write, timeout: 10 * time.Second, maxResponseBytes: defaultMaxResponseBytes}
}

// direct は MCP を通さずに API を直接呼ぶ。期待するメッセージを API 自身から得るために使う。
func direct(t *testing.T, method, path, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, integrationAPIURL(t)+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("API に接続できない: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

type loggedIn struct {
	token string
	id    string
}

func login(t *testing.T, email string) loggedIn {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": seedPassword})
	status, raw := direct(t, http.MethodPost, "/login", "", string(body))
	var parsed struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if status != http.StatusOK || json.Unmarshal(raw, &parsed) != nil || parsed.Token == "" {
		t.Fatalf("login(%s): status=%d body=%s", email, status, raw)
	}
	return loggedIn{token: parsed.Token, id: parsed.ID}
}

func decodeTool[T any](t *testing.T, out toolOutput) T {
	t.Helper()
	if out.isError {
		t.Fatalf("tool がエラーを返した: %s", out.text())
	}
	var v T
	if err := json.Unmarshal([]byte(out.text()), &v); err != nil {
		t.Fatalf("結果が JSON でない: %v\n%s", err, out.text())
	}
	return v
}

type shopSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type shopList struct {
	HasMore bool          `json:"has_more"`
	Items   []shopSummary `json:"items"`
}

type reviewView struct {
	ID      string `json:"id"`
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
	CanEdit bool   `json:"can_edit"`
	User    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Burger *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"burger"`
}

type reviewList struct {
	HasMore bool         `json:"has_more"`
	Items   []reviewView `json:"items"`
}

func findShop(t *testing.T, session *mcp.ClientSession, name string) shopSummary {
	t.Helper()
	list := decodeTool[shopList](t, call(t, session, "list_shops", map[string]any{"keyword": name}))
	for _, shop := range list.Items {
		if shop.Name == name {
			return shop
		}
	}
	t.Fatalf("seed の shop %q が見つからない: %+v", name, list.Items)
	return shopSummary{}
}

func TestIntegrationReadTools(t *testing.T) {
	alice := login(t, "alice@example.com")
	anonymous := connect(t, integrationConfig(t, "", false))

	t.Run("ショップ一覧は keyword で絞れ、ページ送りの has_more を返す", func(t *testing.T) {
		all := decodeTool[shopList](t, call(t, anonymous, "list_shops", nil))
		if len(all.Items) < 3 {
			t.Fatalf("seed の 3 件以上があるはず: %+v", all.Items)
		}
		hit := findShop(t, anonymous, "Shake Shack 渋谷")
		if hit.Status != "active" {
			t.Fatalf("status: %q", hit.Status)
		}
		paged := decodeTool[shopList](t, call(t, anonymous, "list_shops", map[string]any{"per_page": 1}))
		if len(paged.Items) != 1 || !paged.HasMore {
			t.Fatalf("per_page=1 なら 1 件で、続きがあるはず: %+v", paged)
		}
		second := decodeTool[shopList](t, call(t, anonymous, "list_shops", map[string]any{"per_page": 1, "page": 2}))
		if len(second.Items) != 1 || second.Items[0].ID == paged.Items[0].ID {
			t.Fatalf("2 ページ目は別のショップのはず: %+v", second)
		}
	})

	t.Run("ショップ詳細にレビューと can_review が含まれる", func(t *testing.T) {
		shop := findShop(t, anonymous, "Shake Shack 渋谷")
		detail := decodeTool[struct {
			Name      string       `json:"name"`
			Reviews   []reviewView `json:"reviews"`
			CanReview *bool        `json:"can_review"`
		}](t, call(t, anonymous, "get_shop", map[string]any{"id": shop.ID}))
		if detail.Name != shop.Name || len(detail.Reviews) == 0 || detail.CanReview == nil {
			t.Fatalf("%+v", detail)
		}
	})

	t.Run("レビュー一覧は shop_id・user_id・rating・keyword で絞れる", func(t *testing.T) {
		shop := findShop(t, anonymous, "Shake Shack 渋谷")
		byShop := decodeTool[reviewList](t, call(t, anonymous, "list_reviews", map[string]any{"shop_id": shop.ID}))
		if len(byShop.Items) == 0 {
			t.Fatal("shop_id で絞ったレビューがない")
		}
		byUser := decodeTool[reviewList](t, call(t, anonymous, "list_reviews", map[string]any{"user_id": alice.id}))
		if len(byUser.Items) == 0 {
			t.Fatal("user_id で絞ったレビューがない")
		}
		for _, review := range byUser.Items {
			if review.User.ID != alice.id {
				t.Fatalf("user_id で絞ったのに別のユーザーのレビュー: %+v", review)
			}
		}
		byRating := decodeTool[reviewList](t, call(t, anonymous, "list_reviews", map[string]any{"rating": 5}))
		if len(byRating.Items) == 0 {
			t.Fatal("rating=5 のレビューがない")
		}
		for _, review := range byRating.Items {
			if review.Rating != 5 {
				t.Fatalf("rating=5 で絞ったのに: %+v", review)
			}
		}
		byKeyword := decodeTool[reviewList](t, call(t, anonymous, "list_reviews", map[string]any{"keyword": "肉汁"}))
		if len(byKeyword.Items) == 0 {
			t.Fatal("keyword で絞ったレビューがない")
		}
		for _, review := range byKeyword.Items {
			if !strings.Contains(review.Comment, "肉汁") {
				t.Fatalf("keyword で絞ったのに: %+v", review)
			}
		}
	})

	t.Run("レビュー詳細は本人かどうかを can_edit で返す", func(t *testing.T) {
		mine := decodeTool[reviewList](t, call(t, anonymous, "list_reviews", map[string]any{"user_id": alice.id})).Items[0]
		anonymousView := decodeTool[reviewView](t, call(t, anonymous, "get_review", map[string]any{"id": mine.ID}))
		if anonymousView.ID != mine.ID || anonymousView.CanEdit {
			t.Fatalf("トークンなしでは can_edit は false: %+v", anonymousView)
		}
		asAlice := connect(t, integrationConfig(t, alice.token, false))
		if own := decodeTool[reviewView](t, call(t, asAlice, "get_review", map[string]any{"id": mine.ID})); !own.CanEdit {
			t.Fatalf("本人のトークンなら can_edit は true: %+v", own)
		}
	})

	t.Run("ユーザーのプロフィール。email は本人のトークンのときだけ API が返す", func(t *testing.T) {
		other := decodeTool[map[string]any](t, call(t, anonymous, "get_user", map[string]any{"id": alice.id}))
		if other["username"] != "alice" {
			t.Fatalf("%+v", other)
		}
		if _, leaked := other["email"]; leaked {
			t.Fatalf("トークンなしで email が返ってきた: %+v", other)
		}
		self := decodeTool[map[string]any](t, call(t, connect(t, integrationConfig(t, alice.token, false)), "get_user", map[string]any{"id": alice.id}))
		if self["email"] != "alice@example.com" {
			t.Fatalf("本人には email が返るはず: %+v", self)
		}
	})

	t.Run("API の規則を返す", func(t *testing.T) {
		meta := decodeTool[struct {
			Rating struct{ Min, Max int } `json:"rating"`
		}](t, call(t, anonymous, "get_meta", nil))
		if meta.Rating.Min != 1 || meta.Rating.Max != 5 {
			t.Fatalf("%+v", meta)
		}
	})
}

func TestIntegrationWriteToolsAreOffByDefault(t *testing.T) {
	session := connect(t, integrationConfig(t, login(t, "alice@example.com").token, false))
	for _, name := range writeToolNames {
		if contains(toolNames(t, session), name) {
			t.Errorf("%s は既定では公開されないはず", name)
		}
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_review", Arguments: map[string]any{}})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("書き込みが無効なら create_review は呼べないはず: %+v", res)
	}
}

func TestIntegrationReviewLifecycle(t *testing.T) {
	alice, bob := login(t, "alice@example.com"), login(t, "bob@example.com")
	asAlice := connect(t, integrationConfig(t, alice.token, true))
	asBob := connect(t, integrationConfig(t, bob.token, true))
	shop := findShop(t, asAlice, "Shake Shack 渋谷")

	created := decodeTool[reviewView](t, call(t, asAlice, "create_review", map[string]any{
		"shop_id": shop.ID, "burger_name": "結合テスト用バーガー", "rating": 4, "comment": "結合テストの投稿",
	}))
	if created.ID == "" || created.User.ID != alice.id || !created.CanEdit || created.Burger == nil || created.Burger.Name != "結合テスト用バーガー" {
		t.Fatalf("%+v", created)
	}

	if got := decodeTool[reviewView](t, call(t, asAlice, "get_review", map[string]any{"id": created.ID})); got.Comment != "結合テストの投稿" {
		t.Fatalf("%+v", got)
	}

	updated := decodeTool[reviewView](t, call(t, asAlice, "update_review", map[string]any{"id": created.ID, "rating": 2, "comment": "更新しました"}))
	if updated.Rating != 2 || updated.Comment != "更新しました" {
		t.Fatalf("%+v", updated)
	}

	t.Run("他のユーザーのレビューは、更新も削除も API が 403 で拒否する", func(t *testing.T) {
		wantStatus, wantBody := direct(t, http.MethodPut, "/reviews/"+created.ID, bob.token, `{"review":{"rating":1,"comment":"乗っ取り"}}`)
		if wantStatus != http.StatusForbidden {
			t.Fatalf("API 自身が 403 を返すはず: %d %s", wantStatus, wantBody)
		}
		wantMessage := apiMessage(wantBody)
		for _, attempt := range []struct {
			tool string
			args map[string]any
		}{
			{"update_review", map[string]any{"id": created.ID, "rating": 1, "comment": "乗っ取り"}},
			{"delete_review", map[string]any{"id": created.ID}},
		} {
			out := call(t, asBob, attempt.tool, attempt.args)
			if !out.isError || !strings.Contains(out.text(), "HTTP 403") || !strings.Contains(out.text(), wantMessage) {
				t.Fatalf("%s: isError=%t %s（API のメッセージ: %s）", attempt.tool, out.isError, out.text(), wantMessage)
			}
		}
		if still := decodeTool[reviewView](t, call(t, asAlice, "get_review", map[string]any{"id": created.ID})); still.Rating != 2 {
			t.Fatalf("拒否されたのにレビューが変わった: %+v", still)
		}
	})

	if out := call(t, asAlice, "delete_review", map[string]any{"id": created.ID}); out.isError {
		t.Fatalf("本人の削除に失敗: %s", out.text())
	}
	gone := call(t, asAlice, "get_review", map[string]any{"id": created.ID})
	if !gone.isError || !strings.Contains(gone.text(), "HTTP 404") {
		t.Fatalf("削除後は 404 のはず: %s", gone.text())
	}
}

func TestIntegrationAPIErrorsPassThrough(t *testing.T) {
	alice := login(t, "alice@example.com")
	session := connect(t, integrationConfig(t, alice.token, true))
	shop := findShop(t, session, "Shake Shack 渋谷")
	const unknownID = "11111111-1111-4111-8111-111111111111"

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		method   string
		path     string
		body     string
		wantHTTP string
	}{
		{
			name: "評価が範囲外（422）", tool: "create_review",
			args:   map[string]any{"shop_id": shop.ID, "burger_name": "検証用", "rating": 99, "comment": "範囲外"},
			method: http.MethodPost, path: "/reviews",
			body:     fmt.Sprintf(`{"review":{"rating":99,"comment":"範囲外","shop_id":%q,"burger_name":"検証用"}}`, shop.ID),
			wantHTTP: "HTTP 422",
		},
		{
			name: "コメントが空（422）", tool: "create_review",
			args:   map[string]any{"shop_id": shop.ID, "burger_name": "検証用", "rating": 3, "comment": ""},
			method: http.MethodPost, path: "/reviews",
			body:     fmt.Sprintf(`{"review":{"rating":3,"comment":"","shop_id":%q,"burger_name":"検証用"}}`, shop.ID),
			wantHTTP: "HTTP 422",
		},
		{
			name: "存在しないショップへの投稿（404）", tool: "create_review",
			args:   map[string]any{"shop_id": unknownID, "burger_name": "検証用", "rating": 3, "comment": "存在しない店"},
			method: http.MethodPost, path: "/reviews",
			body:     fmt.Sprintf(`{"review":{"rating":3,"comment":"存在しない店","shop_id":%q,"burger_name":"検証用"}}`, unknownID),
			wantHTTP: "HTTP 404",
		},
		{
			name: "存在しないレビュー（404）", tool: "get_review", args: map[string]any{"id": unknownID},
			method: http.MethodGet, path: "/reviews/" + unknownID, wantHTTP: "HTTP 404",
		},
		{
			name: "UUID でない id（404）", tool: "get_shop", args: map[string]any{"id": "not-a-uuid"},
			method: http.MethodGet, path: "/shops/not-a-uuid", wantHTTP: "HTTP 404",
		},
		{
			name: "UUID でない user_id の絞り込み（422）", tool: "list_reviews", args: map[string]any{"user_id": "not-a-uuid"},
			method: http.MethodGet, path: "/reviews?user_id=not-a-uuid", wantHTTP: "HTTP 422",
		},
		{
			name: "page に -1 を渡しても握りつぶさず、可否は API に任せる", tool: "list_shops", args: map[string]any{"page": -1},
			method: http.MethodGet, path: "/shops?page=-1", wantHTTP: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantStatus, wantBody := direct(t, tt.method, tt.path, alice.token, tt.body)
			out := call(t, session, tt.tool, tt.args)
			if wantStatus >= 200 && wantStatus < 300 {
				if out.isError {
					t.Fatalf("API が %d を返すのに tool がエラー: %s", wantStatus, out.text())
				}
				return
			}
			if !out.isError || !strings.Contains(out.text(), fmt.Sprintf("HTTP %d", wantStatus)) {
				t.Fatalf("API は %d のはず。tool: isError=%t %s", wantStatus, out.isError, out.text())
			}
			if tt.wantHTTP != "" && !strings.Contains(out.text(), tt.wantHTTP) {
				t.Fatalf("%q を含むはず: %s", tt.wantHTTP, out.text())
			}
			if msg := apiMessage(wantBody); msg == "" || !strings.Contains(out.text(), msg) {
				t.Fatalf("API のメッセージ %q をそのまま渡すはず: %s", msg, out.text())
			}
		})
	}
}

func TestIntegrationTokenIsRequiredForWrites(t *testing.T) {
	session := connect(t, integrationConfig(t, "", true))
	shop := findShop(t, session, "Shake Shack 渋谷")
	out := call(t, session, "create_review", map[string]any{"shop_id": shop.ID, "burger_name": "匿名", "rating": 3, "comment": "トークンなし"})
	if !out.isError || !strings.Contains(out.text(), "HTTP 401") || !strings.Contains(out.text(), "HAMBURGER_API_TOKEN") {
		t.Fatalf("トークンなしの書き込みは 401 と、トークンの確認の案内になるはず: %s", out.text())
	}
}

func TestIntegrationSubmitShop(t *testing.T) {
	alice := login(t, "alice@example.com")
	session := connect(t, integrationConfig(t, alice.token, true))
	name := fmt.Sprintf("結合テストの店 %d", time.Now().UnixNano())
	submitted := decodeTool[struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}](t, call(t, session, "submit_shop", map[string]any{"name": name}))
	if submitted.ID == "" || submitted.Name != name || submitted.Status != "pending" {
		t.Fatalf("投稿した店は pending のはず: %+v", submitted)
	}
	if blank := call(t, session, "submit_shop", map[string]any{"name": ""}); !blank.isError || !strings.Contains(blank.text(), "HTTP 422") {
		t.Fatalf("店名が空なら API が 422 にするはず: %s", blank.text())
	}
}

func TestIntegrationAPIDownDoesNotCrashTheServer(t *testing.T) {
	t.Run("接続を拒否されても tool のエラーを返し続ける", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		dead := "http://" + listener.Addr().String()
		_ = listener.Close()
		session := connect(t, config{apiURL: dead, timeout: time.Second, maxResponseBytes: defaultMaxResponseBytes})
		for range 2 {
			if out := call(t, session, "list_shops", nil); !out.isError || !strings.Contains(out.text(), "Could not reach the API") {
				t.Fatalf("%s", out.text())
			}
		}
		if len(toolNames(t, session)) == 0 {
			t.Fatal("server が落ちた")
		}
	})

	t.Run("応答しない API はタイムアウトの tool エラーにする", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				t.Cleanup(func() { _ = conn.Close() })
			}
		}()
		session := connect(t, config{apiURL: "http://" + listener.Addr().String(), timeout: 500 * time.Millisecond, maxResponseBytes: defaultMaxResponseBytes})
		started := time.Now()
		out := call(t, session, "get_meta", nil)
		if !out.isError || !strings.Contains(out.text(), "timed out") || time.Since(started) > 5*time.Second {
			t.Fatalf("%s（%s）", out.text(), time.Since(started))
		}
		if len(toolNames(t, session)) == 0 {
			t.Fatal("server が落ちた")
		}
	})
}

// 実際に build した実行ファイルを stdio でつなぎ、Claude Code などの client と同じ経路を確かめる。
func TestIntegrationStdioBinary(t *testing.T) {
	alice := login(t, "alice@example.com")
	binary := filepath.Join(t.TempDir(), "hamburger-mcp")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd := exec.Command(binary)
	cmd.Env = []string{
		"HAMBURGER_API_URL=" + integrationAPIURL(t),
		"HAMBURGER_API_TOKEN=" + alice.token,
		"HAMBURGER_MCP_ALLOW_WRITE=true",
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("stdio で接続できない: %v\nstderr: %s", err, stderr.String())
	}
	defer session.Close()

	if got := len(toolNames(t, session)); got != len(readToolNames)+len(writeToolNames) {
		t.Fatalf("tool の数: %d", got)
	}
	shop := findShop(t, session, "Shake Shack 渋谷")
	if shop.ID == "" {
		t.Fatal("stdio 経由で実 API のデータが取れない")
	}
	_ = session.Close()
	if strings.Contains(stderr.String(), alice.token) {
		t.Fatalf("stderr にトークンが出ている: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "token=set") {
		t.Fatalf("起動ログにトークンの有無が出るはず: %s", stderr.String())
	}
}
