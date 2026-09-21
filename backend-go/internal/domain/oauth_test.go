package domain_test

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestValidateOAuthScopes(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{"読み取りだけを指定すると、有効な範囲として通る", []string{domain.OAuthScopeRead}, false},
		{"読み取りと書き込みの両方を指定すると、有効な範囲として通る", []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}, false},
		{"範囲が空だと、エラーになる", nil, true},
		{"知らない範囲が 1 つでも混ざると、エラーになる", []string{domain.OAuthScopeRead, "admin"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateOAuthScopes(tt.scopes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrOAuthInvalidScope) {
				t.Errorf("err = %v, want ErrOAuthInvalidScope", err)
			}
		})
	}
}

func TestMergeOAuthScopes(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{"重複する範囲は 1 つにまとまる", []string{domain.OAuthScopeRead}, []string{domain.OAuthScopeRead}, []string{domain.OAuthScopeRead}},
		{"入力の順番に関係なく、同意画面に出す順番(読み取り、書き込み)に並ぶ", []string{domain.OAuthScopeWrite}, []string{domain.OAuthScopeRead}, []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}},
		{"知らない範囲は、保存済みの値を落とさないよう、最後に名前の昇順で付く", []string{"zeta", "alpha"}, []string{domain.OAuthScopeRead}, []string{domain.OAuthScopeRead, "alpha", "zeta"}},
		{"どちらも空なら、空のまま", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.MergeOAuthScopes(tt.a, tt.b)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeOAuthScopes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOAuthConsentRequired(t *testing.T) {
	read, write := domain.OAuthScopeRead, domain.OAuthScopeWrite
	tests := []struct {
		name               string
		granted, requested []string
		want               bool
	}{
		{"まだ何も許可していないアプリが読み取りを求めると、同意画面を出す", nil, []string{read}, true},
		{"読み取りを許可済みのアプリが読み取りだけを求めると、同意画面を出し直さない", []string{read}, []string{read}, false},
		{"読み取りを許可済みのアプリが書き込みも求めて範囲が広がると、同意画面を出し直す", []string{read}, []string{read, write}, true},
		{"読み取りと書き込みを許可済みのアプリが読み取りだけを求めると、範囲が狭まるだけなので出し直さない", []string{read, write}, []string{read}, false},
		{"書き込みだけを許可済みのアプリが読み取りを求めると、許可していない範囲なので出し直す", []string{write}, []string{read}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.OAuthConsentRequired(tt.granted, tt.requested); got != tt.want {
				t.Errorf("OAuthConsentRequired(%v, %v) = %v, want %v", tt.granted, tt.requested, got, tt.want)
			}
		})
	}
}

func TestResolveOAuthResource(t *testing.T) {
	const allowed = "https://example.com/mcp"
	tests := []struct {
		name      string
		requested []string
		want      string
		wantErr   bool
	}{
		{"宛先を指定しないと、この認可サーバーの宛先が使われる", nil, allowed, false},
		{"認可サーバーの宛先と同じ宛先を指定すると、そのまま通る", []string{allowed}, allowed, false},
		{"別のサーバーの宛先を指定すると、宛先の誤りになる", []string{"https://other.example.com/mcp"}, "", true},
		{"末尾のスラッシュが違うだけの宛先も、完全に同じではないので誤りになる", []string{allowed + "/"}, "", true},
		{"正しい宛先に別の宛先が混ざると、誤りになる", []string{allowed, "https://other.example.com/mcp"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ResolveOAuthResource(tt.requested, allowed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrOAuthInvalidTarget) {
				t.Errorf("err = %v, want ErrOAuthInvalidTarget", err)
			}
			if got != tt.want {
				t.Errorf("resource = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOAuthAccessTokenCheck(t *testing.T) {
	const resource = "https://example.com/mcp"
	token := domain.OAuthAccessToken{
		UserID:   "u",
		Scopes:   []string{domain.OAuthScopeRead},
		Audience: []string{resource},
	}
	t.Run("宛先が同じで、必要な範囲を許可されたトークンは、通る", func(t *testing.T) {
		if err := token.Check(resource, domain.OAuthScopeRead); err != nil {
			t.Errorf("Check = %v, want nil", err)
		}
	})
	t.Run("必要な範囲を指定しない確認は、宛先だけを見て通る", func(t *testing.T) {
		if err := token.Check(resource); err != nil {
			t.Errorf("Check = %v, want nil", err)
		}
	})
	t.Run("別のサーバー宛てのトークンは、範囲が足りていても、無効なトークンとして断る", func(t *testing.T) {
		err := token.Check("https://other.example.com/mcp", domain.OAuthScopeRead)
		if !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("Check = %v, want ErrOAuthInvalidToken", err)
		}
	})
	t.Run("書き込みの範囲がないトークンで書き込みを求めると、足りない範囲を示して断る", func(t *testing.T) {
		err := token.Check(resource, domain.OAuthScopeRead, domain.OAuthScopeWrite)
		if !errors.Is(err, domain.ErrOAuthInsufficientScope) {
			t.Fatalf("Check = %v, want ErrOAuthInsufficientScope", err)
		}
		var scopeErr *domain.InsufficientScopeError
		if !errors.As(err, &scopeErr) || !reflect.DeepEqual(scopeErr.Missing, []string{domain.OAuthScopeWrite}) {
			t.Errorf("missing = %+v, want [%s]", scopeErr, domain.OAuthScopeWrite)
		}
	})
}

func TestValidateOAuthRedirectURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"https の URL は、戻り先として登録できる", "https://app.example.com/callback", false},
		{"127.0.0.1 の http は、ポート付きでも登録できる", "http://127.0.0.1:8123/callback", false},
		{"IPv6 のループバック([::1])の http は登録できる", "http://[::1]:8123/callback", false},
		{"localhost の http は登録できる(ポートの違いは受け付けない照合になる)", "http://localhost:3000/callback", false},
		{"ループバック以外の http は、盗み見られうるので登録できない", "http://app.example.com/callback", true},
		{"独自スキームは、ほかのアプリに横取りされうるので登録できない", "myapp://callback", true},
		{"断片(#)を含む URL は登録できない", "https://app.example.com/callback#frag", true},
		{"末尾が # だけの URL も、断片の指定として登録できない", "https://app.example.com/callback#", true},
		{"認証情報(利用者名:パスワード@)を含む URL は登録できない", "https://user:pass@app.example.com/callback", true},
		{"相対 URL は登録できない", "/callback", true},
		{"空文字は登録できない", "", true},
		{"長すぎる URL は登録できない", "https://app.example.com/" + strings.Repeat("a", domain.MaxOAuthURILength), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateOAuthRedirectURI(tt.uri)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrOAuthClientInvalid) {
				t.Errorf("err = %v, want ErrOAuthClientInvalid", err)
			}
		})
	}
}

func TestOAuthClientValidate(t *testing.T) {
	valid := domain.OAuthClient{ID: "app", Name: "テストアプリ", RedirectURIs: []string{"https://app.example.com/cb"}}
	tests := []struct {
		name    string
		mutate  func(c *domain.OAuthClient)
		wantErr bool
	}{
		{"必要な項目がそろっていれば、通る", func(*domain.OAuthClient) {}, false},
		{"識別子が空だと、通らない", func(c *domain.OAuthClient) { c.ID = "" }, true},
		{"表示名が空白だけだと、通らない", func(c *domain.OAuthClient) { c.Name = "  " }, true},
		{"表示名が上限(100 文字)を超えると、通らない", func(c *domain.OAuthClient) { c.Name = strings.Repeat("あ", domain.MaxOAuthClientNameLength+1) }, true},
		{"表示名がちょうど上限(100 文字)なら、通る", func(c *domain.OAuthClient) { c.Name = strings.Repeat("あ", domain.MaxOAuthClientNameLength) }, false},
		{"戻り先が 1 つもないと、通らない", func(c *domain.OAuthClient) { c.RedirectURIs = nil }, true},
		{"戻り先が上限(10 個)を超えると、通らない", func(c *domain.OAuthClient) {
			c.RedirectURIs = nil
			for i := 0; i <= domain.MaxOAuthRedirectURIs; i++ {
				c.RedirectURIs = append(c.RedirectURIs, "https://app.example.com/cb")
			}
		}, true},
		{"登録できない戻り先が 1 つでも混ざると、通らない", func(c *domain.OAuthClient) {
			c.RedirectURIs = append(c.RedirectURIs, "http://evil.example.com/cb")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			c.RedirectURIs = append([]string(nil), valid.RedirectURIs...)
			tt.mutate(&c)
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrOAuthClientInvalid) {
				t.Errorf("err = %v, want ErrOAuthClientInvalid", err)
			}
		})
	}
}

func TestValidateClientMetadataURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https のホスト名と経路がある URL は、説明の文書の URL として通る", "https://app.example.com/oauth/client.json", false},
		{"標準のポート(443)を明示しても通る", "https://app.example.com:443/oauth/client.json", false},
		{"http は、通信が改ざんされうるので通らない", "http://app.example.com/oauth/client.json", true},
		{"標準以外のポートは、内部のサービスを探る足がかりになるので通らない", "https://app.example.com:8443/oauth/client.json", true},
		{"IPv4 アドレスの直接指定は通らない", "https://93.184.216.34/client.json", true},
		{"内部向けの IPv4 アドレスの直接指定は通らない", "https://169.254.169.254/latest/meta-data", true},
		{"IPv6 アドレスの直接指定は通らない", "https://[2001:4860:4860::8888]/client.json", true},
		{"認証情報を含む URL は通らない", "https://user:pass@app.example.com/client.json", true},
		{"断片を含む URL は通らない", "https://app.example.com/client.json#x", true},
		{"経路がない URL は通らない", "https://app.example.com", true},
		{"経路がルート(/)だけの URL は通らない", "https://app.example.com/", true},
		{"経路に .. を含む URL は通らない", "https://app.example.com/a/../client.json", true},
		{"経路に . を含む URL は通らない", "https://app.example.com/./client.json", true},
		{"ホストがない URL は通らない", "https:///client.json", true},
		{"空文字は通らない", "", true},
		{"長すぎる URL は通らない", "https://app.example.com/" + strings.Repeat("a", domain.MaxOAuthURILength), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateClientMetadataURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrOAuthClientInvalid) {
				t.Errorf("err = %v, want ErrOAuthClientInvalid", err)
			}
		})
	}
}

func TestParseClientMetadataDocument(t *testing.T) {
	const id = "https://app.example.com/oauth/client.json"
	tests := []struct {
		name     string
		doc      string
		wantName string
		wantErr  bool
	}{
		{"必要な項目がそろった文書は、登録情報として読める",
			`{"client_id":"` + id + `","client_name":"Example App","redirect_uris":["https://app.example.com/cb"],"token_endpoint_auth_method":"none","extra":1}`,
			"Example App", false},
		{"認証方式を省略した文書も、公開クライアントとして読める",
			`{"client_id":"` + id + `","client_name":"Example App","redirect_uris":["https://app.example.com/cb"]}`,
			"Example App", false},
		{"表示名がない文書は、URL のホスト名を表示名にする",
			`{"client_id":"` + id + `","redirect_uris":["https://app.example.com/cb"]}`,
			"app.example.com", false},
		{"上限より長い表示名は、上限の文字数で切って読む",
			`{"client_id":"` + id + `","client_name":"` + strings.Repeat("あ", domain.MaxOAuthClientNameLength+20) + `","redirect_uris":["https://app.example.com/cb"]}`,
			strings.Repeat("あ", domain.MaxOAuthClientNameLength), false},
		{"文書の client_id が取得した URL と違うと、他人の URL になりすませてしまうので断る",
			`{"client_id":"https://evil.example.com/client.json","client_name":"x","redirect_uris":["https://app.example.com/cb"]}`, "", true},
		{"秘密の鍵で認証する方式の文書は、公開クライアントだけを扱うので断る",
			`{"client_id":"` + id + `","client_name":"x","redirect_uris":["https://app.example.com/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, "", true},
		{"登録できない戻り先(ループバック以外の http)を含む文書は断る",
			`{"client_id":"` + id + `","client_name":"x","redirect_uris":["http://evil.example.com/cb"]}`, "", true},
		{"戻り先がない文書は断る", `{"client_id":"` + id + `","client_name":"x"}`, "", true},
		{"JSON ではない本文は断る", `<html>not json</html>`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := domain.ParseClientMetadataDocument(id, []byte(tt.doc))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				if !errors.Is(err, domain.ErrOAuthClientInvalid) {
					t.Errorf("err = %v, want ErrOAuthClientInvalid", err)
				}
				return
			}
			if c.ID != id || c.Name != tt.wantName {
				t.Errorf("client = %+v, want ID %q name %q", c, id, tt.wantName)
			}
		})
	}
}

func TestIsPublicAddress(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"公開の IPv4 アドレスは、接続してよい", "93.184.216.34", true},
		{"公開の IPv6 アドレスは、接続してよい", "2606:2800:220:1:248:1893:25c8:1946", true},
		{"IPv4 のループバック(127.0.0.1)は、自分自身を指すので接続しない", "127.0.0.1", false},
		{"IPv6 のループバック(::1)は接続しない", "::1", false},
		{"プライベートアドレス(10.0.0.1)は接続しない", "10.0.0.1", false},
		{"プライベートアドレス(172.16.0.1)は接続しない", "172.16.0.1", false},
		{"プライベートアドレス(192.168.1.1)は接続しない", "192.168.1.1", false},
		{"クラウドの情報取得用のアドレス(169.254.169.254)は、認証情報を盗まれうるので接続しない", "169.254.169.254", false},
		{"IPv6 のリンクローカル(fe80::1)は接続しない", "fe80::1", false},
		{"IPv6 のユニークローカル(fd00::1)は接続しない", "fd00::1", false},
		{"事業者内の共有アドレス(100.64.0.1)は接続しない", "100.64.0.1", false},
		{"未指定のアドレス(0.0.0.0)は接続しない", "0.0.0.0", false},
		{"マルチキャスト(224.0.0.1)は接続しない", "224.0.0.1", false},
		{"予約されたアドレス(240.0.0.1)は接続しない", "240.0.0.1", false},
		{"文書用のアドレス(192.0.2.1)は接続しない", "192.0.2.1", false},
		{"IPv4 を包んだ IPv6 の形でも、内部の IPv4(::ffff:10.0.0.1)は接続しない", "::ffff:10.0.0.1", false},
		{"IPv4 を包んだ IPv6 の形のループバック(::ffff:127.0.0.1)は接続しない", "::ffff:127.0.0.1", false},
		{"IPv4 を IPv6 に変換する範囲(64:ff9b::a00:1)は、内部の IPv4 を指せるので接続しない", "64:ff9b::a00:1", false},
		{"6to4 の範囲(2002:a00:1::)は、内部の IPv4 を包めるので接続しない", "2002:a00:1::", false},
		{"IPv4 互換の形(::a00:1)は接続しない", "::a00:1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.IsPublicAddress(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("IsPublicAddress(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
	t.Run("無効なアドレス(ゼロ値)は接続しない", func(t *testing.T) {
		if domain.IsPublicAddress(netip.Addr{}) {
			t.Error("IsPublicAddress(zero) = true, want false")
		}
	})
}

func TestValidateOAuthPKCE(t *testing.T) {
	const good = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" // RFC 7636 の付録の例
	tests := []struct {
		name              string
		challenge, method string
		wantErr           bool
	}{
		{"S256 と、SHA-256 を base64url にした challenge がそろっていれば、通る", good, "S256", false},
		{"challenge がないと、通らない", "", "S256", true},
		{"方式が plain だと、challenge から確認用の文字列が分かってしまうので、通らない", good, "plain", true},
		{"方式を指定しないと、通らない", good, "", true},
		{"challenge が短すぎると、通らない", good[:20], "S256", true},
		{"challenge が長すぎると、通らない", good + "a", "S256", true},
		{"challenge に base64url ではない文字が含まれると、通らない", good[:42] + "+", "S256", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := domain.ValidateOAuthPKCE(tt.challenge, tt.method); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestOAuthScopeWrites(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope string
		want  bool
	}{
		{"読み取りの範囲は、書き込みを伴わない", domain.OAuthScopeRead, false},
		{"書き込みの範囲は、書き込みを伴う", domain.OAuthScopeWrite, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scope, ok := domain.OAuthScopeByName(tt.scope)
			if !ok {
				t.Fatalf("scope %q not found", tt.scope)
			}
			if scope.Writes != tt.want {
				t.Errorf("Writes = %v, want %v", scope.Writes, tt.want)
			}
		})
	}
}
