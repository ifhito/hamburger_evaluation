package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// OAuth の認可サーバーの規則。AI アプリ(クライアント)が、利用者の許可を得て、このアプリの
// API を使うための「許可の証(トークン)」を発行する仕組みの、業務上の決まりを集める。
// トークンの生成や署名などのプロトコルの細部は、認可ライブラリ(adapter/oauthserver)が担い、
// 何を許すか・いつまで有効か・どこへ送ってよいかの判断は、ここが持つ。

// OAuth の許可の範囲(scope)の名前。
const (
	// OAuthScopeRead は、ショップ・レビュー・プロフィールを読むだけの範囲である。
	OAuthScopeRead = "hamburger:read"
	// OAuthScopeWrite は、利用者の名前でレビューやショップの申請を書き込む範囲である。
	OAuthScopeWrite = "hamburger:write"
	// OAuthScopeAdmin は、ショップの申請を審査し、閉業・再開を行う(一覧・承認・却下・閉業・再開)ための
	// 範囲である。MCP の管理用のツールが使う。アプリが明示して要求しない限り、既定の範囲
	// (DefaultOAuthScopes)には含めない。
	OAuthScopeAdmin = "hamburger:admin"
)

// OAuth のトークンの有効期間。業務のルールで、環境ごとに変える設定ではない。
const (
	// OAuthAuthorizationCodeTTL は、認可コード(アプリが、許可の証と交換するための引換券)の有効期間である。
	// アプリは受け取ってすぐ交換するので、短くする。
	OAuthAuthorizationCodeTTL = 2 * time.Minute
	// OAuthAccessTokenTTL は、アクセストークン(API を呼ぶための許可の証)の有効期間である。
	// 盗まれたときの被害を小さくするため短くし、切れたら更新トークンで取り直す。
	OAuthAccessTokenTTL = 15 * time.Minute
	// OAuthRefreshTokenTTL は、更新トークン(アクセストークンを取り直すための証)の有効期間である。
	OAuthRefreshTokenTTL = 30 * 24 * time.Hour
)

// クライアント(AI アプリ)の情報に関する上限。
const (
	// MaxOAuthClientNameLength は、アプリの表示名の最大の文字数(Unicode のコードポイント数)である。
	// 同意画面と接続済みアプリの一覧に出るので、長すぎる名前で画面を崩されないようにする。
	MaxOAuthClientNameLength = 100
	// MaxOAuthRedirectURIs は、1 つのアプリが登録できる戻り先(redirect_uri)の最大の個数である。
	MaxOAuthRedirectURIs = 10
	// MaxOAuthURILength は、client_id の URL と戻り先の URL の最大の長さである。
	MaxOAuthURILength = 2048
	// MaxClientMetadataBytes は、アプリの説明の文書(CIMD)として読み込む本文の最大のバイト数である。
	MaxClientMetadataBytes = 64 << 10
	// ClientMetadataFetchTimeout は、アプリの説明の文書を取りに行くときの、待ち時間の上限である。
	ClientMetadataFetchTimeout = 5 * time.Second
	// ClientMetadataCacheTTL は、取得したアプリの説明の文書を、取り直さずに使う期間である。
	ClientMetadataCacheTTL = 5 * time.Minute
)

// OAuth に関する domain error。呼び出し側は errors.Is で照合する。
var (
	// ErrOAuthInvalidToken は、アクセストークンが、存在しない・改ざんされている・期限切れ・
	// 取り消し済み・宛先違い・持ち主が退会済みのいずれかで、使えないことを表す。呼び出し側に、
	// どれなのかは伝えない。
	ErrOAuthInvalidToken = errors.New("oauth access token is invalid")
	// ErrOAuthInsufficientScope は、トークンは有効だが、その操作に必要な範囲(scope)を許可されていないことを
	// 表す。
	ErrOAuthInsufficientScope = errors.New("oauth access token does not have the required scope")
	// ErrOAuthInvalidTarget は、要求された宛先(resource)が、この認可サーバーが発行できる宛先ではないことを
	// 表す。
	ErrOAuthInvalidTarget = errors.New("oauth resource is not supported")
	// ErrOAuthInvalidScope は、知らない範囲(scope)が含まれる、または範囲が空であることを表す。
	ErrOAuthInvalidScope = errors.New("oauth scope is invalid")
	// ErrOAuthClientInvalid は、アプリ(クライアント)の登録情報が、規則を満たさないことを表す。
	ErrOAuthClientInvalid = errors.New("oauth client is invalid")
	// ErrOAuthAuthorizeRequestInvalid は、認可の要求が、アプリへ結果を戻せない形で不正なことを表す
	// (知らないアプリ・登録されていない戻り先など)。結果を戻せる不正(範囲の誤りなど)は、これではなく、
	// アプリへの戻り先にエラーを付けて伝える。
	ErrOAuthAuthorizeRequestInvalid = errors.New("oauth authorization request is invalid")
	// ErrOAuthGrantNotFound は、指定された許可の記録が(その利用者のものとして)存在しないことを表す。
	ErrOAuthGrantNotFound = errors.New("oauth grant not found")
	// ErrOAuthTokenSessionNotFound は、指定されたトークンの記録が存在しないことを表す。
	ErrOAuthTokenSessionNotFound = errors.New("oauth token session not found")
	// ErrOAuthTokenSessionInactive は、指定されたトークンの記録が、すでに使用済み(または取り消し済み)で、
	// もう使えないことを表す。
	ErrOAuthTokenSessionInactive = errors.New("oauth token session is no longer active")
)

// InsufficientScopeError は、足りない範囲(scope)を持つ、ErrOAuthInsufficientScope である。
type InsufficientScopeError struct {
	Missing []string
}

func (e *InsufficientScopeError) Error() string {
	return "oauth access token does not have the required scope: " + strings.Join(e.Missing, " ")
}

// Is は errors.Is(err, ErrOAuthInsufficientScope) を成り立たせる。
func (e *InsufficientScopeError) Is(target error) bool { return target == ErrOAuthInsufficientScope }

// OAuthScope は、許可の範囲の名前と、同意画面に出す説明である。説明は、利用者が画面で読む文言で、
// 画面(frontend)が英語なので英語で書く。文言は domain が持ち、frontend は写さずに、API から受け取って表示する。
type OAuthScope struct {
	Name        string
	Description string
	// Writes は、この範囲が、利用者の名前でデータを書き込む(投稿・編集・削除・申請)ものかである。
	// 画面は、この値で書き込みの範囲を強調して見せる。名前を比べて決めない(規則は domain が持つ)。
	Writes bool
}

var oauthScopes = []OAuthScope{
	{Name: OAuthScopeRead, Description: "View shops, reviews and profiles"},
	{Name: OAuthScopeWrite, Description: "Post, edit and delete reviews, and submit shops, on your behalf", Writes: true},
	{Name: OAuthScopeAdmin, Description: "Moderate shop submissions: list, approve, reject, close and reopen them", Writes: true},
}

// OAuthScopes は、許可できる範囲の一覧を、同意画面に出す順番で返す。
func OAuthScopes() []OAuthScope { return slices.Clone(oauthScopes) }

// OAuthPKCEMethod は、認可コードの横取りを防ぐ確認(PKCE)で、唯一許す方式である。challenge は、確認用の
// 文字列(verifier)の SHA-256 で、平文(plain)は許さない(平文では、横取りした側も challenge から
// verifier を知れてしまうため)。
const OAuthPKCEMethod = "S256"

// ValidateOAuthPKCE は、認可の要求の PKCE の値(challenge と method)を確かめる。challenge は必須で、
// method は S256 だけを許す。challenge は、SHA-256 を base64url にした 43 文字である。満たさなければ、
// 理由を書いたエラーを返す。
func ValidateOAuthPKCE(challenge, method string) error {
	if challenge == "" {
		return errors.New("PKCE is required: code_challenge is missing")
	}
	if method != OAuthPKCEMethod {
		return fmt.Errorf("PKCE method must be %s", OAuthPKCEMethod)
	}
	if len(challenge) != 43 {
		return errors.New("code_challenge must be the 43 characters of a base64url encoded SHA-256")
	}
	for _, r := range challenge {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return errors.New("code_challenge must be base64url characters")
		}
	}
	return nil
}

// DefaultOAuthScopes は、アプリが範囲を指定しなかったときに許可を求める範囲である。最も小さい
// 範囲(読むだけ)にして、書き込みは、アプリが明示したときだけ求める。
func DefaultOAuthScopes() []string { return []string{OAuthScopeRead} }

// OAuthScopeByName は、名前から範囲を探す。知らない名前は ok=false を返す。
func OAuthScopeByName(name string) (OAuthScope, bool) {
	for _, s := range oauthScopes {
		if s.Name == name {
			return s, true
		}
	}
	return OAuthScope{}, false
}

// ValidateOAuthScopes は、names が 1 つ以上で、すべて知っている範囲であることを確かめる。
// 満たさなければ、(wrap された)ErrOAuthInvalidScope を返す。
func ValidateOAuthScopes(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("%w: no scope", ErrOAuthInvalidScope)
	}
	for _, n := range names {
		if _, ok := OAuthScopeByName(n); !ok {
			return fmt.Errorf("%w: unknown scope %q", ErrOAuthInvalidScope, n)
		}
	}
	return nil
}

// MergeOAuthScopes は、a と b の範囲を重複なしでまとめ、一覧の順番(OAuthScopes)に並べて返す。
// 知らない名前は、名前の昇順で最後に付ける(保存済みの値を落とさないため)。
func MergeOAuthScopes(a, b []string) []string {
	seen := map[string]bool{}
	for _, n := range a {
		seen[n] = true
	}
	for _, n := range b {
		seen[n] = true
	}
	var merged []string
	for _, s := range oauthScopes {
		if seen[s.Name] {
			merged = append(merged, s.Name)
			delete(seen, s.Name)
		}
	}
	unknown := make([]string, 0, len(seen))
	for n := range seen {
		unknown = append(unknown, n)
	}
	slices.Sort(unknown)
	return append(merged, unknown...)
}

// OAuthConsentRequired は、同意画面を出す必要があるかを返す。すでに許可した範囲(granted)が、
// 今回要求された範囲(requested)を全部含んでいれば、出し直さない。1 つでも含まれない範囲があれば
// (許可が初めての場合も含めて)出す。
func OAuthConsentRequired(granted, requested []string) bool {
	for _, n := range requested {
		if !slices.Contains(granted, n) {
			return true
		}
	}
	return false
}

// ResolveOAuthResource は、アプリが要求した宛先(resource。トークンを、どのサーバーに使うか)を、
// この認可サーバーが発行できる宛先 allowed に確かめる。要求がなければ、唯一の allowed を使う。
// 要求があるときは、すべてが allowed と完全に同じでなければならず、違えば(wrap された)
// ErrOAuthInvalidTarget を返す。
func ResolveOAuthResource(requested []string, allowed string) (string, error) {
	for _, r := range requested {
		if r != allowed {
			return "", fmt.Errorf("%w: %q", ErrOAuthInvalidTarget, r)
		}
	}
	return allowed, nil
}

// OAuthAccessToken は、検証を通ったアクセストークンが表す内容である。トークンの文字列そのものは持たない。
type OAuthAccessToken struct {
	UserID    string
	ClientID  string
	Scopes    []string
	Audience  []string
	ExpiresAt time.Time
}

// Check は、このトークンを、宛先 resource の、必要な範囲 required の操作に使ってよいかを判断する。
// 宛先が違えば ErrOAuthInvalidToken(ほかのサーバー宛てのトークンを受け付けない)、範囲が足りなければ
// 足りない範囲を持つ *InsufficientScopeError を返す。
func (t OAuthAccessToken) Check(resource string, required ...string) error {
	if !slices.Contains(t.Audience, resource) {
		return ErrOAuthInvalidToken
	}
	var missing []string
	for _, n := range required {
		if !slices.Contains(t.Scopes, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return &InsufficientScopeError{Missing: missing}
	}
	return nil
}

// OAuthClient は、トークンを求める側のアプリ(クライアント)の登録情報である。秘密の鍵を持てない
// アプリ(公開クライアント)だけを扱い、認可コードの横取りは PKCE で防ぐ。
type OAuthClient struct {
	// ID はアプリの識別子である。固定で登録したアプリは任意の文字列、アプリが自分の説明を公開して
	// いるもの(CIMD)は、その文書の URL である。
	ID string
	// Name は同意画面に出す表示名である。
	Name string
	// RedirectURIs は、認可の結果(認可コード)を返してよい戻り先である。
	RedirectURIs []string
}

// Validate は、登録情報が規則を満たすかを確かめる。満たさなければ、(wrap された)
// ErrOAuthClientInvalid を返す。
func (c OAuthClient) Validate() error {
	if c.ID == "" || utf8.RuneCountInString(c.ID) > MaxOAuthURILength {
		return fmt.Errorf("%w: client id must be 1..%d characters", ErrOAuthClientInvalid, MaxOAuthURILength)
	}
	if strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > MaxOAuthClientNameLength {
		return fmt.Errorf("%w: client name must be 1..%d characters", ErrOAuthClientInvalid, MaxOAuthClientNameLength)
	}
	if len(c.RedirectURIs) == 0 || len(c.RedirectURIs) > MaxOAuthRedirectURIs {
		return fmt.Errorf("%w: redirect_uris must have 1..%d entries", ErrOAuthClientInvalid, MaxOAuthRedirectURIs)
	}
	for _, r := range c.RedirectURIs {
		if err := ValidateOAuthRedirectURI(r); err != nil {
			return err
		}
	}
	return nil
}

// ValidateOAuthRedirectURI は、戻り先として登録してよい URL かを確かめる。https の URL か、
// 同じ端末の別のプログラムに返すための http のループバック(127.0.0.1・[::1]・localhost)だけを許す。
// 断片(#…)と認証情報(利用者名:パスワード@)は許さない。
//
// 要求された戻り先と登録済みの戻り先の照合は、認可ライブラリが行う: 完全に同じ文字列だけを受け付け、
// 例外は、127.0.0.1 と [::1] のループバックで、ポート番号だけが違うものを受け付ける
// (ポートは起動のたびに変わるため。RFC 8252)。localhost はポートまで完全に同じでなければならない。
func ValidateOAuthRedirectURI(raw string) error {
	if raw == "" || utf8.RuneCountInString(raw) > MaxOAuthURILength {
		return fmt.Errorf("%w: redirect_uri must be 1..%d characters", ErrOAuthClientInvalid, MaxOAuthURILength)
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("%w: redirect_uri must be an absolute URL", ErrOAuthClientInvalid)
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("%w: redirect_uri must not have a fragment", ErrOAuthClientInvalid)
	}
	if u.User != nil {
		return fmt.Errorf("%w: redirect_uri must not have credentials", ErrOAuthClientInvalid)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if host := u.Hostname(); host == "127.0.0.1" || host == "::1" || host == "localhost" {
			return nil
		}
	}
	return fmt.Errorf("%w: redirect_uri must be https, or http on a loopback address", ErrOAuthClientInvalid)
}

// ValidateClientMetadataURL は、アプリが自分の説明を公開している URL(client_id として使う)の形を確かめる。
// サーバーがこの URL を取りに行くので、内部のサーバーへ向けさせる攻撃(SSRF)を、まず形で防ぐ:
// https だけ、標準のポート(443)だけ、IP アドレスの直接指定は不可、認証情報・断片は不可、
// 経路(パス)は必須で、. と .. の部分は不可。宛先の IP アドレスの確認は、接続のときに行う
// (IsPublicAddress)。
func ValidateClientMetadataURL(raw string) error {
	bad := func(reason string) error {
		return fmt.Errorf("%w: client_id URL %s", ErrOAuthClientInvalid, reason)
	}
	if raw == "" || utf8.RuneCountInString(raw) > MaxOAuthURILength {
		return bad(fmt.Sprintf("must be 1..%d characters", MaxOAuthURILength))
	}
	u, err := url.Parse(raw)
	if err != nil {
		return bad("is not a valid URL")
	}
	if u.Scheme != "https" {
		return bad("must use https")
	}
	if u.User != nil {
		return bad("must not have credentials")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return bad("must not have a fragment")
	}
	host := u.Hostname()
	if host == "" {
		return bad("must have a host")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return bad("must use a host name, not an IP address")
	}
	if p := u.Port(); p != "" && p != "443" {
		return bad("must use the default https port")
	}
	if u.Path == "" || u.Path == "/" {
		return bad("must have a path")
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "." || seg == ".." {
			return bad("must not contain dot segments")
		}
	}
	return nil
}

// ParseClientMetadataDocument は、アプリが公開している説明の文書(CIMD。JSON)を、登録情報にする。
// 文書の client_id は、取りに行った URL(clientID)と完全に同じでなければならない(他人の URL の
// 文書になりすませない)。秘密の鍵を使う認証方式(token_endpoint_auth_method が none 以外)は、
// 公開クライアントだけを扱うので断る。表示名がなければ、URL のホスト名を使う。
func ParseClientMetadataDocument(clientID string, doc []byte) (OAuthClient, error) {
	var d struct {
		ClientID                string   `json:"client_id"`
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		return OAuthClient{}, fmt.Errorf("%w: client metadata is not valid JSON", ErrOAuthClientInvalid)
	}
	if d.ClientID != clientID {
		return OAuthClient{}, fmt.Errorf("%w: client metadata client_id does not match its URL", ErrOAuthClientInvalid)
	}
	if d.TokenEndpointAuthMethod != "" && d.TokenEndpointAuthMethod != "none" {
		return OAuthClient{}, fmt.Errorf("%w: only public clients (token_endpoint_auth_method none) are supported", ErrOAuthClientInvalid)
	}
	name := strings.TrimSpace(d.ClientName)
	if name == "" {
		if u, err := url.Parse(clientID); err == nil {
			name = u.Hostname()
		}
	}
	if utf8.RuneCountInString(name) > MaxOAuthClientNameLength {
		name = string([]rune(name)[:MaxOAuthClientNameLength])
	}
	c := OAuthClient{ID: clientID, Name: name, RedirectURIs: d.RedirectURIs}
	if err := c.Validate(); err != nil {
		return OAuthClient{}, err
	}
	return c, nil
}

// 公開のインターネットではないアドレスの範囲。netip の判定(ループバック・プライベート・リンクローカル・
// マルチキャスト・未指定)に含まれないものを足す。
var nonPublicPrefixes = func() []netip.Prefix {
	var ps []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",       // このネットワーク
		"100.64.0.0/10",   // 事業者内の共有アドレス(CGNAT)
		"192.0.0.0/24",    // IETF の予約
		"192.0.2.0/24",    // 文書用
		"198.18.0.0/15",   // 性能試験用
		"198.51.100.0/24", // 文書用
		"203.0.113.0/24",  // 文書用
		"240.0.0.0/4",     // 予約
		"::/96",           // IPv4 互換(廃止された形。内部の IPv4 を指せる)
		"64:ff9b::/96",    // IPv4 を IPv6 に変換する範囲(内部の IPv4 を指せる)
		"100::/64",        // 破棄用
		"2001::/32",       // Teredo(IPv4 を包む)
		"2001:db8::/32",   // 文書用
		"2002::/16",       // 6to4(IPv4 を包む)
	} {
		ps = append(ps, netip.MustParsePrefix(s))
	}
	return ps
}()

// IsPublicAddress は、a が公開のインターネットのアドレスかを返す。ループバック・プライベート・
// リンクローカル(クラウドの情報取得用の 169.254.169.254 を含む)・マルチキャスト・未指定・予約の範囲は
// 公開ではない。サーバーが利用者の指定した URL を取りに行くとき、内部のサーバーへ向かわないように、
// 接続先の判断に使う。IPv4 を包んだ IPv6 の形(::ffff:10.0.0.1)は、IPv4 として判断する。
func IsPublicAddress(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsUnspecified() ||
		a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() || a.IsMulticast() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
