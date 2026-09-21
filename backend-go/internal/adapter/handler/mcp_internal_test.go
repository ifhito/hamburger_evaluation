package handler

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ツールを足したのに範囲の表(mcpToolScopes)に書き忘れると、そのツールは範囲の確認を素通りして
// 誰にでも呼べてしまう。逆に表にだけあって登録がなければ、範囲の確認が空振りする。
// 登録されたツールの一覧と表が、名前も範囲も一致することを確かめる。
func TestMCPToolScopesMatchRegisteredTools(t *testing.T) {
	server := (&MCPServer{}).newToolServer(domain.User{ID: "00000000-0000-4000-8000-000000000001"}, nil, domain.LangEN)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var registered []string
	for _, tool := range res.Tools {
		registered = append(registered, tool.Name)
		scope, ok := mcpToolScopes[tool.Name]
		if !ok {
			t.Errorf("tool %s is registered but has no entry in mcpToolScopes (anyone could call it)", tool.Name)
			continue
		}
		if !strings.Contains(tool.Description, "必要な許可の範囲: "+scope) {
			t.Errorf("tool %s description = %q, want the scope %s from mcpToolScopes", tool.Name, tool.Description, scope)
		}
	}
	var declared []string
	for name := range mcpToolScopes {
		declared = append(declared, name)
	}
	sort.Strings(registered)
	sort.Strings(declared)
	if strings.Join(registered, ",") != strings.Join(declared, ",") {
		t.Errorf("registered tools = %v, mcpToolScopes = %v, want them to match", registered, declared)
	}
}

func TestRequiredScopes(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"読み取りのツールの呼び出し":       {`{"method":"tools/call","params":{"name":"get_shop"}}`, domain.OAuthScopeRead},
		"書き込みのツールの呼び出し":       {`{"method":"tools/call","params":{"name":"create_review"}}`, domain.OAuthScopeWrite},
		"ツールを呼ばない要求は範囲を要求しない": {`{"method":"tools/list"}`, ""},
		"知らないツールは範囲を要求しない":    {`{"method":"tools/call","params":{"name":"no_such_tool"}}`, ""},
		"読めない本文は範囲を要求しない":     {`not json`, ""},
		"複数の要求は、必要な範囲を全部集める":  {`[{"method":"tools/call","params":{"name":"get_shop"}},{"method":"tools/call","params":{"name":"delete_review"}}]`, domain.OAuthScopeRead + " " + domain.OAuthScopeWrite},
		"空の本文は範囲を要求しない":       {``, ""},
	} {
		if got := strings.Join(requiredScopes([]byte(tc.body)), " "); got != tc.want {
			t.Errorf("%s: requiredScopes(%q) = %q, want %q", name, tc.body, got, tc.want)
		}
	}
}

// validArguments は、各ツールの、入力の検査(SDK が、ツールの前に行う)を通る最小の引数である。
var validArguments = func() map[string]map[string]any {
	id := "00000000-0000-4000-8000-000000000001"
	return map[string]map[string]any{
		"get_meta": {}, "list_shops": {}, "list_reviews": {},
		"get_shop": {"shop_id": id}, "get_review": {"review_id": id}, "get_user": {"user_id": id},
		"create_review": {"shop_id": id, "rating": 4, "comment": "x"},
		"update_review": {"review_id": id, "rating": 4, "comment": "x"},
		"delete_review": {"review_id": id},
		"submit_shop":   {"name": "x"},
	}
}()

// 要求の入口が本文から読み取った範囲の確認は、SDK が本文を別の読み方で解釈すると、すり抜けうる。
// そのため、ツールを実行する直前にも、実際に実行されるツールの名前で、範囲を確かめる。ここでは、入口を
// 通らずに、ツールを直接呼んで、範囲が足りなければ、中身(usecase。ここでは nil で、呼べば panic する)に
// 届く前に断られることを確かめる。
func TestMCPToolsCheckTheScopeRightBeforeRunning(t *testing.T) {
	call := func(t *testing.T, scopes []string, tool string, lang ...domain.Lang) *mcp.CallToolResult {
		t.Helper()
		language := domain.LangEN
		if len(lang) > 0 {
			language = lang[0]
		}
		server := (&MCPServer{}).newToolServer(domain.User{ID: "00000000-0000-4000-8000-000000000001"}, scopes, language)
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		ctx := context.Background()
		if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
			t.Fatalf("connect server: %v", err)
		}
		session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatalf("connect client: %v", err)
		}
		defer session.Close()
		args, ok := validArguments[tool]
		if !ok {
			t.Fatalf("validArguments has no entry for tool %s: add one", tool)
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("call %s: %v", tool, err)
		}
		return res
	}
	text := func(res *mcp.CallToolResult) string {
		return res.Content[0].(*mcp.TextContent).Text
	}

	for tool, scope := range mcpToolScopes {
		other := domain.OAuthScopeWrite
		if scope == domain.OAuthScopeWrite {
			other = domain.OAuthScopeRead
		}
		for name, scopes := range map[string][]string{"範囲が空": nil, "別の範囲だけ": {other}} {
			res := call(t, scopes, tool)
			if !res.IsError || text(res) != "Insufficient scope: "+scope {
				t.Errorf("%s (%s): got %q (isError=%v), want it to be refused with the missing scope", tool, name, text(res), res.IsError)
			}
		}
	}
	if res := call(t, nil, "get_meta", domain.LangJA); !res.IsError || text(res) != "許可の範囲が足りません: hamburger:read" {
		t.Errorf("日本語の要求で範囲が足りないとき: got %q (isError=%v), want 日本語の文言", text(res), res.IsError)
	}
	if res := call(t, []string{domain.OAuthScopeRead}, "get_meta"); res.IsError || !strings.Contains(text(res), `"rating"`) {
		t.Errorf("get_meta with the read scope = %q (isError=%v), want it to run", text(res), res.IsError)
	}
}
