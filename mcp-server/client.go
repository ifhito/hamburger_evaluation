package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const redacted = "[REDACTED]"

// apiClient はアプリの HTTP API を呼ぶだけの薄いクライアントである。
// 入力の検証・権限の判断・状態遷移・計算は一切しない（ドメインの規則は backend だけが持つ）。
// ここが持つ責務は、通信（タイムアウト・応答サイズの上限）と、トークンを出力に出さないことだけである。
type apiClient struct {
	baseURL  string
	token    string
	maxBytes int
	http     *http.Client
}

type apiRequest struct {
	method string
	path   string // path の各要素は url.PathEscape 済み
	query  url.Values
	body   any
}

type apiResponse struct {
	status  int
	body    []byte
	hasMore bool
}

func newAPIClient(cfg config) *apiClient {
	return &apiClient{
		baseURL:  cfg.apiURL,
		token:    cfg.token,
		maxBytes: cfg.maxResponseBytes,
		http: &http.Client{
			Timeout: cfg.timeout,
			// API は redirect しない。追いかけると、別ホストへトークンを渡しかねない。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// do は API を 1 回呼ぶ。2xx 以外と通信の失敗は error で返す。その文言はトークンを伏せてある。
func (c *apiClient) do(ctx context.Context, r apiRequest) (apiResponse, error) {
	var body io.Reader
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return apiResponse{}, err
		}
		body = bytes.NewReader(encoded)
	}
	target := c.baseURL + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return apiResponse{}, c.transportError(err)
	}
	req.Header.Set("Accept", "application/json")
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return apiResponse{}, c.transportError(err)
	}
	defer resp.Body.Close()

	// 上限を 1 バイト超えるまでしか読まない（巨大な応答を丸ごと溜め込まない）。
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBytes)+1))
	if err != nil {
		return apiResponse{}, c.transportError(err)
	}
	out := apiResponse{status: resp.StatusCode, body: raw, hasMore: resp.Header.Get("X-Has-More") == "true"}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiResponse{}, c.statusError(out)
	}
	return out, nil
}

func (c *apiClient) transportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return errors.New("The API request timed out. Check that the API is running and reachable at " + c.baseURL)
	}
	return errors.New(c.redact("Could not reach the API at " + c.baseURL + ": " + err.Error()))
}

// statusError は API のエラーメッセージ（{"error":"..."} か {"errors":[...]}）をそのまま渡す。
func (c *apiClient) statusError(resp apiResponse) error {
	msg := apiMessage(resp.body)
	text := fmt.Sprintf("API returned HTTP %d %s", resp.status, http.StatusText(resp.status))
	if msg != "" {
		text += ": " + msg
	}
	if resp.status == http.StatusUnauthorized {
		text += " (Check that " + envAPIToken + " is set to a valid token.)"
	}
	return errors.New(c.redact(text))
}

func apiMessage(body []byte) string {
	var parsed struct {
		Error  string   `json:"error"`
		Errors []string `json:"errors"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if len(parsed.Errors) > 0 {
			return strings.Join(parsed.Errors, "; ")
		}
	}
	return cutUTF8(strings.TrimSpace(string(body)), 500)
}

func (c *apiClient) redact(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, redacted)
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
}

// render は API の応答を tool の結果にする。list のときは、次のページの有無
// （X-Has-More）を has_more として先頭に置く。上限を超える分は切り捨て、続きの取り方を hint で添える。
func (c *apiClient) render(resp apiResponse, list bool, hint string) *mcp.CallToolResult {
	text := string(resp.body)
	switch {
	case list:
		text = fmt.Sprintf(`{"has_more":%t,"items":%s}`, resp.hasMore, text)
	case text == "":
		text = fmt.Sprintf("OK (HTTP %d, empty response)", resp.status)
	}
	text = c.redact(text)
	content := []mcp.Content{}
	if len(text) > c.maxBytes {
		text = cutUTF8(text, c.maxBytes)
		content = append(content, &mcp.TextContent{Text: text}, &mcp.TextContent{
			Text: fmt.Sprintf("[truncated] The response was cut at the %d-byte limit. %s", c.maxBytes, hint),
		})
		return &mcp.CallToolResult{Content: content}
	}
	return &mcp.CallToolResult{Content: append(content, &mcp.TextContent{Text: text})}
}

// cutUTF8 は s を n バイト以内に切る。多バイト文字の途中では切らない。
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
