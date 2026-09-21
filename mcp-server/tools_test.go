package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var readToolNames = []string{"get_meta", "get_review", "get_shop", "get_user", "list_reviews", "list_shops"}
var writeToolNames = []string{"create_review", "delete_review", "submit_shop", "update_review"}

func sorted(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}

func TestToolsByWriteMode(t *testing.T) {
	api := newFakeAPI(t, jsonReply(200, `{}`))

	t.Run("書き込みが無効なら、読み取りの tool だけを公開する", func(t *testing.T) {
		got := sorted(toolNames(t, connect(t, testConfig(api.URL))))
		if !reflect.DeepEqual(got, readToolNames) {
			t.Fatalf("got %v, want %v", got, readToolNames)
		}
	})

	t.Run("書き込みが有効なら、書き込みの tool も公開する", func(t *testing.T) {
		cfg := testConfig(api.URL)
		cfg.allowWrite = true
		got := sorted(toolNames(t, connect(t, cfg)))
		want := sorted(append(append([]string(nil), readToolNames...), writeToolNames...))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("ログイン・アカウント・管理の tool は、書き込みが有効でも公開しない", func(t *testing.T) {
		cfg := testConfig(api.URL)
		cfg.allowWrite = true
		for _, name := range toolNames(t, connect(t, cfg)) {
			for _, forbidden := range []string{"login", "logout", "signup", "account", "admin", "approve", "reject", "photo", "delete_user", "update_user"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("tool %q は公開してはいけない", name)
				}
			}
		}
	})
}

func TestToolMetadata(t *testing.T) {
	api := newFakeAPI(t, jsonReply(200, `{}`))
	cfg := testConfig(api.URL)
	cfg.allowWrite = true
	res, err := connect(t, cfg).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		isWrite := contains(writeToolNames, tool.Name)
		if tool.Description == "" || tool.Annotations == nil {
			t.Errorf("%s: 説明と annotations が必要", tool.Name)
			continue
		}
		if isWrite {
			if tool.Annotations.ReadOnlyHint {
				t.Errorf("%s: 書き込みの tool を読み取り専用にしてはいけない", tool.Name)
			}
			if !strings.Contains(tool.Description, "WRITE") || !strings.Contains(tool.Description, "really") {
				t.Errorf("%s: 説明に、実際に投稿・変更・削除することを書くこと: %q", tool.Name, tool.Description)
			}
		} else if !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: 読み取りの tool は読み取り専用にする", tool.Name)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func TestReadToolsForwardRequests(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		args      map[string]any
		wantPath  string
		wantQuery map[string][]string
	}{
		{name: "一覧は条件なしでも呼べる", tool: "list_shops", args: map[string]any{}, wantPath: "/shops", wantQuery: map[string][]string{}},
		{
			name: "ショップ一覧は keyword と page と per_page を渡す", tool: "list_shops",
			args:     map[string]any{"keyword": "下北沢", "page": 2, "per_page": 5},
			wantPath: "/shops", wantQuery: map[string][]string{"keyword": {"下北沢"}, "page": {"2"}, "per_page": {"5"}},
		},
		{name: "ショップ詳細", tool: "get_shop", args: map[string]any{"id": "shop-1"}, wantPath: "/shops/shop-1", wantQuery: map[string][]string{}},
		{
			name: "レビュー一覧は絞り込みをすべて渡す", tool: "list_reviews",
			args:     map[string]any{"shop_id": "s1", "user_id": "u1", "rating": 4, "keyword": "肉汁", "page": 3, "per_page": 10},
			wantPath: "/reviews",
			wantQuery: map[string][]string{
				"shop_id": {"s1"}, "user_id": {"u1"}, "rating": {"4"}, "keyword": {"肉汁"}, "page": {"3"}, "per_page": {"10"},
			},
		},
		{
			name: "rating が 0 でも握りつぶさず API に渡す（可否は API が決める）", tool: "list_reviews",
			args: map[string]any{"rating": 0}, wantPath: "/reviews", wantQuery: map[string][]string{"rating": {"0"}},
		},
		{name: "レビュー詳細", tool: "get_review", args: map[string]any{"id": "r1"}, wantPath: "/reviews/r1", wantQuery: map[string][]string{}},
		{name: "ユーザーのプロフィール", tool: "get_user", args: map[string]any{"id": "u1"}, wantPath: "/users/u1", wantQuery: map[string][]string{}},
		{name: "API の規則", tool: "get_meta", args: map[string]any{}, wantPath: "/meta", wantQuery: map[string][]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, jsonReply(200, `[]`))
			out := call(t, connect(t, testConfig(api.URL)), tt.tool, tt.args)
			if out.isError {
				t.Fatalf("エラーになった: %s", out.text())
			}
			got := api.only(t)
			if got.method != http.MethodGet || got.escapedPath != tt.wantPath {
				t.Fatalf("got %s %s, want GET %s", got.method, got.escapedPath, tt.wantPath)
			}
			if !reflect.DeepEqual(got.query, tt.wantQuery) {
				t.Fatalf("query: got %v, want %v", got.query, tt.wantQuery)
			}
			if got.auth != "Bearer test-token" {
				t.Fatalf("Authorization: got %q", got.auth)
			}
		})
	}
}

func TestReadWithoutTokenSendsNoAuthorization(t *testing.T) {
	api := newFakeAPI(t, jsonReply(200, `{}`))
	cfg := testConfig(api.URL)
	cfg.token = ""
	if out := call(t, connect(t, cfg), "get_meta", nil); out.isError {
		t.Fatalf("トークンなしでも読み取りはできる: %s", out.text())
	}
	if got := api.only(t).auth; got != "" {
		t.Fatalf("Authorization を送ってはいけない: %q", got)
	}
}

func TestPathIDsAreEscaped(t *testing.T) {
	api := newFakeAPI(t, jsonReply(404, `{"error":"not found"}`))
	call(t, connect(t, testConfig(api.URL)), "get_shop", map[string]any{"id": "../admin/shops?x=1"})
	if got := api.only(t).escapedPath; got != "/shops/..%2Fadmin%2Fshops%3Fx=1" {
		t.Fatalf("id の / や ? は path の一部としてエスケープされるはず: %q", got)
	}
}

func TestListReturnsHasMore(t *testing.T) {
	for _, header := range []string{"true", "false"} {
		t.Run("X-Has-More が "+header+" のとき", func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Has-More", header)
				_, _ = io.WriteString(w, `[{"id":"s1","name":"店","status":"approved"}]`)
			})
			out := call(t, connect(t, testConfig(api.URL)), "list_shops", nil)
			var got struct {
				HasMore bool             `json:"has_more"`
				Items   []map[string]any `json:"items"`
			}
			if err := json.Unmarshal([]byte(out.text()), &got); err != nil {
				t.Fatalf("結果が JSON でない: %v\n%s", err, out.text())
			}
			if got.HasMore != (header == "true") || len(got.Items) != 1 || got.Items[0]["id"] != "s1" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestWriteToolsSendRequests(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		args        map[string]any
		status      int
		reply       string
		wantMethod  string
		wantPath    string
		wantBody    string
		wantOutText string
	}{
		{
			name: "burger_name でレビューを投稿する", tool: "create_review",
			args:   map[string]any{"shop_id": "s1", "burger_name": "チーズバーガー", "rating": 4, "comment": "うまい"},
			status: 201, reply: `{"id":"r1"}`, wantMethod: "POST", wantPath: "/reviews",
			wantBody: `{"review":{"rating":4,"comment":"うまい","shop_id":"s1","burger_name":"チーズバーガー"}}`, wantOutText: `"id":"r1"`,
		},
		{
			name: "burger_id でレビューを投稿する", tool: "create_review",
			args:   map[string]any{"shop_id": "s1", "burger_id": "b1", "rating": 5, "comment": "最高"},
			status: 201, reply: `{"id":"r2"}`, wantMethod: "POST", wantPath: "/reviews",
			wantBody: `{"review":{"rating":5,"comment":"最高","shop_id":"s1","burger_id":"b1"}}`, wantOutText: `"id":"r2"`,
		},
		{
			name: "レビューを更新する", tool: "update_review",
			args:   map[string]any{"id": "r1", "rating": 3, "comment": "普通"},
			status: 200, reply: `{"id":"r1"}`, wantMethod: "PUT", wantPath: "/reviews/r1",
			wantBody: `{"review":{"rating":3,"comment":"普通"}}`, wantOutText: `"id":"r1"`,
		},
		{
			name: "レビューを削除する（本文なしの 204）", tool: "delete_review", args: map[string]any{"id": "r1"},
			status: 204, reply: ``, wantMethod: "DELETE", wantPath: "/reviews/r1", wantBody: ``, wantOutText: "OK (HTTP 204",
		},
		{
			name: "ショップを投稿する", tool: "submit_shop", args: map[string]any{"name": "新しい店"},
			status: 201, reply: `{"id":"s9","status":"pending"}`, wantMethod: "POST", wantPath: "/shops",
			wantBody: `{"shop":{"name":"新しい店"}}`, wantOutText: `"status":"pending"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, jsonReply(tt.status, tt.reply))
			cfg := testConfig(api.URL)
			cfg.allowWrite = true
			out := call(t, connect(t, cfg), tt.tool, tt.args)
			if out.isError || !strings.Contains(out.text(), tt.wantOutText) {
				t.Fatalf("isError=%t out=%s", out.isError, out.text())
			}
			got := api.only(t)
			if got.method != tt.wantMethod || got.escapedPath != tt.wantPath || got.auth != "Bearer test-token" {
				t.Fatalf("got %s %s auth=%q", got.method, got.escapedPath, got.auth)
			}
			if tt.wantBody == "" {
				if got.body != "" {
					t.Fatalf("body は空のはず: %q", got.body)
				}
				return
			}
			if got.contentType != "application/json" {
				t.Fatalf("Content-Type: %q", got.contentType)
			}
			var gotJSON, wantJSON any
			if err := json.Unmarshal([]byte(got.body), &gotJSON); err != nil {
				t.Fatalf("body が JSON でない: %v", err)
			}
			_ = json.Unmarshal([]byte(tt.wantBody), &wantJSON)
			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("body: got %s, want %s", got.body, tt.wantBody)
			}
		})
	}
}

func TestMissingRequiredArgumentsNeverReachTheAPI(t *testing.T) {
	api := newFakeAPI(t, jsonReply(200, `{}`))
	session := connect(t, testConfig(api.URL))
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_shop", Arguments: map[string]any{}})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("必須の id がなければ拒否されるはず: %+v", res)
	}
	if api.count() != 0 {
		t.Fatalf("API は呼ばれてはいけない")
	}
}

func TestAPIErrorsPassThrough(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantSubs []string
	}{
		{name: "401 は、トークンの確認を促す", status: 401, body: `{"error":"Unauthorized"}`, wantSubs: []string{"HTTP 401", "Unauthorized", "HAMBURGER_API_TOKEN"}},
		{name: "403 は API のメッセージをそのまま渡す", status: 403, body: `{"error":"You are not authorized to perform this action"}`, wantSubs: []string{"HTTP 403", "You are not authorized to perform this action"}},
		{name: "404", status: 404, body: `{"error":"Review not found"}`, wantSubs: []string{"HTTP 404", "Review not found"}},
		{name: "422 は errors の全メッセージを渡す", status: 422, body: `{"errors":["Rating must be between 1 and 5","Comment can't be blank"]}`, wantSubs: []string{"HTTP 422", "Rating must be between 1 and 5", "Comment can't be blank"}},
		{name: "JSON でない 500 も本文を渡す", status: 500, body: "boom", wantSubs: []string{"HTTP 500", "boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, jsonReply(tt.status, tt.body))
			out := call(t, connect(t, testConfig(api.URL)), "get_review", map[string]any{"id": "r1"})
			if !out.isError {
				t.Fatalf("tool のエラーになるはず: %s", out.text())
			}
			for _, sub := range tt.wantSubs {
				if !strings.Contains(out.text(), sub) {
					t.Errorf("%q を含むはず: %s", sub, out.text())
				}
			}
		})
	}
}

func TestTokenNeverAppearsInOutput(t *testing.T) {
	const token = "s3cr3t-token-value-0123456789"
	echo := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			leaked := r.Header.Get("Authorization")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":"`+leaked+`","errors":["`+leaked+`"],"echo":"`+leaked+`"}`)
		}
	}
	for _, status := range []int{200, 401, 422, 500} {
		t.Run("API がトークンを本文に含めて返しても伏せる: "+http.StatusText(status), func(t *testing.T) {
			api := newFakeAPI(t, echo(status))
			cfg := testConfig(api.URL)
			cfg.token = token
			session := connect(t, cfg)
			for _, tool := range []struct {
				name string
				args map[string]any
			}{{"get_meta", nil}, {"list_shops", nil}, {"get_shop", map[string]any{"id": "x"}}} {
				out := call(t, session, tool.name, tool.args)
				if strings.Contains(out.text(), token) {
					t.Fatalf("%s の出力にトークンが含まれている: %s", tool.name, out.text())
				}
				if !strings.Contains(out.text(), redacted) {
					t.Fatalf("%s: 伏せた印がない: %s", tool.name, out.text())
				}
			}
		})
	}

	t.Run("接続できないときのエラーにも出さない", func(t *testing.T) {
		api := newFakeAPI(t, jsonReply(200, `{}`))
		deadURL := api.URL
		api.Close()
		cfg := testConfig(deadURL)
		cfg.token = token
		out := call(t, connect(t, cfg), "get_meta", nil)
		if !out.isError || strings.Contains(out.text(), token) {
			t.Fatalf("isError=%t out=%s", out.isError, out.text())
		}
	})
}

func TestOversizedResponsesAreTruncated(t *testing.T) {
	// 日本語（3 バイト）を並べ、上限がちょうど文字の途中に来ても壊れないことも確かめる。
	big := `[{"name":"` + strings.Repeat("ハンバーガー", 100) + `"}]`
	api := newFakeAPI(t, jsonReply(200, big))
	cfg := testConfig(api.URL)
	cfg.maxResponseBytes = 200

	t.Run("上限を超えたら切り捨て、続きの取り方を添える", func(t *testing.T) {
		out := call(t, connect(t, cfg), "list_shops", nil)
		if out.isError || len(out.blocks) != 2 {
			t.Fatalf("本文と注記の 2 ブロックのはず: isError=%t blocks=%d", out.isError, len(out.blocks))
		}
		if len(out.blocks[0]) > 200 || !utf8.ValidString(out.blocks[0]) {
			t.Fatalf("本文は上限以内の有効な UTF-8 のはず: len=%d", len(out.blocks[0]))
		}
		if !strings.HasPrefix(out.blocks[0], `{"has_more":false,"items":`) {
			t.Fatalf("has_more は先頭に置くので、切り捨てても残る: %q", out.blocks[0])
		}
		for _, sub := range []string{"[truncated]", "200-byte", "per_page"} {
			if !strings.Contains(out.blocks[1], sub) {
				t.Errorf("注記に %q を含むはず: %s", sub, out.blocks[1])
			}
		}
	})

	t.Run("ショップ詳細の注記は、レビューを別に取る方法を案内する", func(t *testing.T) {
		out := call(t, connect(t, cfg), "get_shop", map[string]any{"id": "s1"})
		if len(out.blocks) != 2 || !strings.Contains(out.blocks[1], "list_reviews") {
			t.Fatalf("blocks=%v", out.blocks)
		}
	})

	t.Run("上限以内なら切り捨てず、注記も付けない", func(t *testing.T) {
		small := newFakeAPI(t, jsonReply(200, `{"rating":{"min":1,"max":5}}`))
		out := call(t, connect(t, testConfig(small.URL)), "get_meta", nil)
		if len(out.blocks) != 1 || out.blocks[0] != `{"rating":{"min":1,"max":5}}` {
			t.Fatalf("blocks=%v", out.blocks)
		}
	})

	t.Run("上限を大きく超える応答でも、最後までは読み込まない", func(t *testing.T) {
		const total = 64 << 20
		var sent atomic.Int64
		huge := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
			chunk := []byte(strings.Repeat("a", 64<<10))
			for sent.Load() < total {
				if _, err := w.Write(chunk); err != nil {
					return
				}
				sent.Add(int64(len(chunk)))
			}
		})
		hugeCfg := cfg
		hugeCfg.apiURL = huge.URL
		out := call(t, connect(t, hugeCfg), "get_meta", nil)
		if out.isError || len(out.blocks) != 2 {
			t.Fatalf("isError=%t blocks=%d", out.isError, len(out.blocks))
		}
		if sent.Load() >= total {
			t.Fatalf("応答を最後まで読み込んでいる（送信 %d バイト）", sent.Load())
		}
	})
}

func TestSlowAPITimesOutWithoutCrashing(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/meta" {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		_, _ = io.WriteString(w, `{"id":"s1"}`)
	})
	cfg := testConfig(api.URL)
	cfg.timeout = 100 * time.Millisecond
	session := connect(t, cfg)

	out := call(t, session, "get_meta", nil)
	if !out.isError || !strings.Contains(out.text(), "timed out") {
		t.Fatalf("タイムアウトの tool エラーになるはず: isError=%t %s", out.isError, out.text())
	}
	if out := call(t, session, "get_shop", map[string]any{"id": "s1"}); out.isError {
		t.Fatalf("タイムアウトの後も server は使える: %s", out.text())
	}
}

func TestUnreachableAPIReturnsToolError(t *testing.T) {
	api := newFakeAPI(t, jsonReply(200, `{}`))
	deadURL := api.URL
	api.Close()
	session := connect(t, testConfig(deadURL))
	for range 2 {
		out := call(t, session, "list_shops", nil)
		if !out.isError || !strings.Contains(out.text(), "Could not reach the API") {
			t.Fatalf("接続できないことを知らせる tool エラーになるはず: isError=%t %s", out.isError, out.text())
		}
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	other := newFakeAPI(t, jsonReply(200, `{}`))
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	})
	out := call(t, connect(t, testConfig(api.URL)), "get_meta", nil)
	if !out.isError || !strings.Contains(out.text(), "HTTP 302") {
		t.Fatalf("isError=%t %s", out.isError, out.text())
	}
	if other.count() != 0 {
		t.Fatalf("別ホストへ redirect を追ってはいけない（トークンが渡る）")
	}
}

func TestAPIMessageIsTrimmedWhenLong(t *testing.T) {
	api := newFakeAPI(t, jsonReply(500, strings.Repeat("x", 5000)))
	out := call(t, connect(t, testConfig(api.URL)), "get_meta", nil)
	if !out.isError || len(out.text()) > 700 {
		t.Fatalf("長い本文は切り詰める: len=%d", len(out.text()))
	}
}
