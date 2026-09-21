// Package fakeoidc は、テストや隔離した確認のために、本物の Google につながずに使える、OpenID Connect の
// 提供元の代役(認可コードの流れ + PKCE)を提供する。探索の情報・公開鍵・認可の画面(自動で承認する)・
// コードの交換を、ループバックの HTTP サーバーで受け付ける。鍵は、サーバーごとに作る。
// 本番のコードから import してはならない。
package fakeoidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// User は、次の認可で、ID トークンに入れる利用者の情報である。
type User struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
}

// Tweaks は、ID トークンやコードの交換を、わざと不正にするための指定である(ゼロ値は、正常)。
type Tweaks struct {
	// Aud は、ID トークンの宛先(aud)を上書きする。
	Aud string
	// Iss は、ID トークンの発行者(iss)を上書きする。
	Iss string
	// Nonce は、ID トークンの nonce を上書きする(空でなければ)。
	Nonce string
	// ExpiredAgo が正なら、その時間だけ前に期限が切れた ID トークンを出す。
	ExpiredAgo time.Duration
	// WrongKey が true なら、公開鍵と別の鍵で署名した ID トークンを出す。
	WrongKey bool
	// NoIDToken が true なら、コードの交換の応答に ID トークンを入れない。
	NoIDToken bool
	// TokenStatus が 0 でなければ、コードの交換が、そのステータスのエラーで失敗する。
	TokenStatus int
	// DenyAuthorize が true なら、認可の画面は、利用者が拒否した(error=access_denied)として戻す。
	DenyAuthorize bool
}

type issued struct {
	nonce       string
	challenge   string
	redirectURI string
	user        User
	used        bool
}

// Server は、提供元の代役である。
type Server struct {
	// URL は、発行者(issuer)の URL である(ループバックの http)。
	URL          string
	ClientID     string
	ClientSecret string

	srv      *httptest.Server
	key      *rsa.PrivateKey
	otherKey *rsa.PrivateKey

	mu     sync.Mutex
	user   User
	tweaks Tweaks
	codes  map[string]*issued
}

const keyID = "fakeoidc-key"

// New は、提供元の代役を起動する。テストの終了時に閉じる。
func New(t testing.TB, clientID, clientSecret string) *Server {
	t.Helper()
	s := NewServer(clientID, clientSecret)
	t.Cleanup(s.Close)
	return s
}

// NewServer は、テストの外(隔離した確認のプログラム)からも使えるように、*testing.T なしで起動する。
// 使い終わったら Close する。
func NewServer(clientID, clientSecret string) *Server {
	s := newUnstarted(clientID, clientSecret)
	s.srv = httptest.NewServer(s.mux())
	s.URL = s.srv.URL
	return s
}

func newUnstarted(clientID, clientSecret string) *Server {
	s := &Server{ClientID: clientID, ClientSecret: clientSecret, codes: map[string]*issued{}}
	s.key = mustKey()
	s.otherKey = mustKey()
	s.user = User{Sub: "sub-default", Email: "default@example.com", EmailVerified: true, Name: "Default User"}
	return s
}

func (s *Server) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("GET /keys", s.handleKeys)
	mux.HandleFunc("GET /authorize", s.handleAuthorize)
	mux.HandleFunc("POST /token", s.handleToken)
	return mux
}

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func (s *Server) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.URL,
		"authorization_endpoint":                s.URL + "/authorize",
		"token_endpoint":                        s.URL + "/token",
		"jwks_uri":                              s.URL + "/keys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (s *Server) handleKeys(w http.ResponseWriter, _ *http.Request) {
	pub := s.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// handleAuthorize は、認可の画面の代役である。利用者を、設定された User として、自動で承認して戻す。
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect := q.Get("redirect_uri")
	if q.Get("client_id") != s.ClientID || q.Get("response_type") != "code" || redirect == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		http.Error(w, "bad authorization request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	tw, user := s.tweaks, s.user
	s.mu.Unlock()
	back, err := url.Parse(redirect)
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	params := back.Query()
	params.Set("state", q.Get("state"))
	if tw.DenyAuthorize {
		params.Set("error", "access_denied")
	} else {
		code := randomCode()
		s.mu.Lock()
		s.codes[code] = &issued{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirectURI: redirect, user: user}
		s.mu.Unlock()
		params.Set("code", code)
	}
	back.RawQuery = params.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func randomCode() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

// handleToken は、コードの交換である。クライアントの認証・redirect_uri・PKCE の検証値を確かめ、コードは 1 回しか使えない。
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	tw := s.tweaks
	s.mu.Unlock()
	if tw.TokenStatus != 0 {
		writeJSON(w, tw.TokenStatus, map[string]string{"error": "server_error"})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	id, secret, hasBasic := r.BasicAuth()
	if !hasBasic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id, _ = url.QueryUnescape(id); id != s.ClientID || secret != s.ClientSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	s.mu.Lock()
	entry, ok := s.codes[r.PostForm.Get("code")]
	if ok && entry.used {
		ok = false
	}
	if ok {
		entry.used = true
	}
	s.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || r.PostForm.Get("grant_type") != "authorization_code" ||
		r.PostForm.Get("redirect_uri") != entry.redirectURI ||
		base64.RawURLEncoding.EncodeToString(sum[:]) != entry.challenge {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	resp := map[string]any{"access_token": "fake-access-token", "token_type": "Bearer", "expires_in": 3600}
	if !tw.NoIDToken {
		resp["id_token"] = s.signIDToken(entry, tw)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) signIDToken(e *issued, tw Tweaks) string {
	iss, aud, nonce := s.URL, s.ClientID, e.nonce
	if tw.Iss != "" {
		iss = tw.Iss
	}
	if tw.Aud != "" {
		aud = tw.Aud
	}
	if tw.Nonce != "" {
		nonce = tw.Nonce
	}
	exp := time.Now().Add(time.Hour)
	if tw.ExpiredAgo > 0 {
		exp = time.Now().Add(-tw.ExpiredAgo)
	}
	claims := jwt.MapClaims{
		"iss": iss, "sub": e.user.Sub, "aud": aud, "exp": exp.Unix(), "iat": time.Now().Add(-time.Minute).Unix(),
		"nonce": nonce, "email": e.user.Email, "email_verified": e.user.EmailVerified, "name": e.user.Name,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = keyID
	key := s.key
	if tw.WrongKey {
		key = s.otherKey
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		panic(err)
	}
	return signed
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
