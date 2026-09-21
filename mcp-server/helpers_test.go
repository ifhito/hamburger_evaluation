package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect は cfg の server を in-memory transport で立て、接続済みの client を返す。
func connect(t *testing.T, cfg config) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(cfg).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

type toolOutput struct {
	blocks  []string
	isError bool
}

func (o toolOutput) text() string { return strings.Join(o.blocks, "\n") }

// call は tool を呼ぶ。引数の不正など、protocol のエラーになったときは失敗にする。
func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) toolOutput {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	out := toolOutput{isError: res.IsError}
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			out.blocks = append(out.blocks, text.Text)
		}
	}
	return out
}

func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

type recordedRequest struct {
	method      string
	escapedPath string
	query       map[string][]string
	auth        string
	contentType string
	body        string
}

// fakeAPI は受けた request を記録し、respond で応答する偽の API である。
type fakeAPI struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func newFakeAPI(t *testing.T, respond http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			method:      r.Method,
			escapedPath: r.URL.EscapedPath(),
			query:       r.URL.Query(),
			auth:        r.Header.Get("Authorization"),
			contentType: r.Header.Get("Content-Type"),
			body:        string(body),
		})
		f.mu.Unlock()
		respond(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPI) only(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 1 {
		t.Fatalf("API へのリクエストが %d 件（1 件のはず）: %+v", len(f.requests), f.requests)
	}
	return f.requests[0]
}

func (f *fakeAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func jsonReply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func testConfig(apiURL string) config {
	return config{apiURL: apiURL, token: "test-token", timeout: 5 * time.Second, maxResponseBytes: defaultMaxResponseBytes}
}
