package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 画面(SPA)へ渡す、手続きの結果の文言である。文言は API が決め、frontend は、返されたものをそのまま出す。
// 原因の詳細(検証のどこで失敗したか)は、どれにも含めない。
const (
	googleSignInFailedMessage  = "Google sign-in failed. Please try again."
	googleCodeInvalidMessage   = "The Google sign-in link is invalid or has expired. Please try again."
	googleAccountExistsMessage = "An account with this email address already exists. Sign in with your password, then connect Google from your profile."
	googleIdentityTakenMessage = "This Google account is already connected to another account."
	googleAlreadyLinkedMessage = "Your account is already connected to a Google account. Disconnect it first."
	googleCannotUnlinkMessage  = "Google is your only way to sign in. Add a password before disconnecting it."
)

// googleFlowCookieName は、1 回のサインインの手続きの間だけ持ち回る値を入れる cookie の名前である。
const googleFlowCookieName = "google_login_flow"

// googleFlowCookieTTL は、手続きの間の cookie の有効期間である(Google の画面での操作を待てる長さ)。
const googleFlowCookieTTL = 10 * time.Minute

// googleFlowCookieInfo は、cookie の暗号鍵を、JWT の秘密から導くときの用途の名前である(鍵の使い回しを避ける)。
const googleFlowCookieInfo = "google-login-flow-cookie-v1"

// GoogleLoginConfig は、Google でのサインインの窓口の設定である。
type GoogleLoginConfig struct {
	// AppBaseURL は、frontend の URL である(末尾に / を付けない)。手続きの結果は、この下の
	// /auth/google/complete へ戻して渡す。
	AppBaseURL string
	// RedirectURL は、Google が認可のあとに利用者を戻す URL(この API の /auth/google/callback の公開 URL)である。
	// cookie の path と、Secure の判断(https なら付ける)に使う。
	RedirectURL string
	// CookieSecret は、cookie の暗号鍵を導く元になる秘密である(JWT の秘密)。ログに出さない。
	CookieSecret string
}

// GoogleLogin は、Google のアカウントでのサインイン・新規登録・結び付けの HTTP の窓口である。
type GoogleLogin struct {
	logins  *usecase.GoogleLogins
	appBase string
	cookie  flowCookie
}

// NewGoogleLogin は GoogleLogin を組み立てる。cfg が不正(戻り先が URL でない、秘密が空)なときはエラーを返す。
func NewGoogleLogin(logins *usecase.GoogleLogins, cfg GoogleLoginConfig) (*GoogleLogin, error) {
	redirect, err := url.Parse(cfg.RedirectURL)
	if err != nil || redirect.Host == "" {
		return nil, errors.New("google login: redirect URL is invalid")
	}
	if cfg.CookieSecret == "" {
		return nil, errors.New("google login: cookie secret is empty")
	}
	key, err := hkdf.Key(sha256.New, []byte(cfg.CookieSecret), nil, googleFlowCookieInfo, 32)
	if err != nil {
		return nil, errors.New("google login: derive cookie key")
	}
	path := redirect.Path
	if path == "" {
		path = "/"
	}
	return &GoogleLogin{
		logins:  logins,
		appBase: strings.TrimRight(cfg.AppBaseURL, "/"),
		cookie:  flowCookie{key: key, path: path, secure: redirect.Scheme == "https"},
	}, nil
}

// LoginProviders は、使えるサインイン方法の名前である(GET /meta が返す)。
func (g *GoogleLogin) LoginProviders() []string {
	if g == nil {
		return []string{}
	}
	return []string{domain.ProviderGoogle}
}

// ---- 手続きの間の cookie ----

// flowCookie は、手続きの秘密の値(state・nonce・PKCE の検証値・戻り先・結び付ける利用者)を、暗号化して
// cookie に封じる。HttpOnly(画面の JavaScript からは読めない)・SameSite=Lax(Google からの戻りの移動では付く)で、
// path を戻り先の path に限り、使い終わったら消す。サーバー側にセッションは持たない。
type flowCookie struct {
	key    []byte
	path   string
	secure bool
}

type flowPayload struct {
	State      string `json:"s"`
	Nonce      string `json:"n"`
	Verifier   string `json:"v"`
	ReturnTo   string `json:"r,omitempty"`
	LinkUserID string `json:"l,omitempty"`
	ExpiresAt  int64  `json:"e"`
}

func (c flowCookie) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal は flow を暗号化して、cookie の値にする。改ざんは、復号のときに検出される。
func (c flowCookie) seal(flow usecase.GoogleFlow, now time.Time) (string, error) {
	plain, err := json.Marshal(flowPayload{
		State: flow.Secrets.State, Nonce: flow.Secrets.Nonce, Verifier: flow.Secrets.Verifier,
		ReturnTo: flow.ReturnTo, LinkUserID: flow.LinkUserID, ExpiresAt: now.Add(googleFlowCookieTTL).Unix(),
	})
	if err != nil {
		return "", err
	}
	gcm, err := c.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, []byte(googleFlowCookieName))), nil
}

// open は、cookie の値を復号して flow に戻す。改ざん・期限切れ・壊れた値は false を返す。
func (c flowCookie) open(value string, now time.Time) (usecase.GoogleFlow, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return usecase.GoogleFlow{}, false
	}
	gcm, err := c.aead()
	if err != nil || len(raw) < gcm.NonceSize() {
		return usecase.GoogleFlow{}, false
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(googleFlowCookieName))
	if err != nil {
		return usecase.GoogleFlow{}, false
	}
	var p flowPayload
	if json.Unmarshal(plain, &p) != nil || now.Unix() > p.ExpiresAt {
		return usecase.GoogleFlow{}, false
	}
	return usecase.GoogleFlow{
		Secrets:    usecase.GoogleFlowSecrets{State: p.State, Nonce: p.Nonce, Verifier: p.Verifier},
		ReturnTo:   p.ReturnTo,
		LinkUserID: p.LinkUserID,
	}, true
}

func (c flowCookie) set(w http.ResponseWriter, flow usecase.GoogleFlow) error {
	value, err := c.seal(flow, time.Now())
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: googleFlowCookieName, Value: value, Path: c.path, MaxAge: int(googleFlowCookieTTL.Seconds()),
		HttpOnly: true, Secure: c.secure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// take は cookie の中身を読み、同時に cookie を消す(1 回の手続きにだけ使う)。読めなければ、ゼロ値を返す。
func (c flowCookie) take(w http.ResponseWriter, r *http.Request) usecase.GoogleFlow {
	http.SetCookie(w, &http.Cookie{
		Name: googleFlowCookieName, Value: "", Path: c.path, MaxAge: -1,
		HttpOnly: true, Secure: c.secure, SameSite: http.SameSiteLaxMode,
	})
	cookie, err := r.Cookie(googleFlowCookieName)
	if err != nil {
		return usecase.GoogleFlow{}
	}
	flow, _ := c.open(cookie.Value, time.Now())
	return flow
}

// ---- 窓口 ----

// noStore は、秘密の値を含む応答を、キャッシュさせず、参照元(Referer)にも漏らさないようにする。
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// completeURL は、手続きの結果を渡す、frontend の画面の URL である。code が空なら、コードなし(失敗)。
func (g *GoogleLogin) completeURL(code string) string {
	dest := g.appBase + "/auth/google/complete"
	if code != "" {
		dest += "?code=" + url.QueryEscape(code)
	}
	return dest
}

// HandleStart は GET /auth/google/start を処理する: 手続きの秘密の値を cookie に封じて、Google の認可の画面へ 302 で送る。
// query の return_to(手続きのあとに戻る先。アプリの中のパスだけ)と、link_code(結び付けの開始のコード)を受ける。
// 始められなかったときは、frontend の結果の画面へ、コードなしで送る(画面が、API から失敗の文言を受け取る)。
func (g *GoogleLogin) HandleStart(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	q := r.URL.Query()
	var (
		begin usecase.GoogleBegin
		err   error
	)
	if link := q.Get("link_code"); link != "" {
		begin, err = g.logins.BeginLink(r.Context(), link, q.Get("return_to"))
	} else {
		begin, err = g.logins.Begin(r.Context(), q.Get("return_to"))
	}
	if err == nil {
		err = g.cookie.set(w, begin.Flow)
	}
	if err != nil {
		if !errors.Is(err, domain.ErrLoginHandoffInvalid) {
			log.Printf("google login: start: %v", err)
		}
		http.Redirect(w, r, g.completeURL(""), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, begin.AuthURL, http.StatusFound)
}

// HandleCallback は GET /auth/google/callback を処理する: Google から戻ってきた要求を処理し、結果を入れた
// 「画面へ渡すコード」を付けて、frontend の結果の画面へ 303 で送る。cookie は、成否にかかわらず消す。
func (g *GoogleLogin) HandleCallback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	flow := g.cookie.take(w, r)
	q := r.URL.Query()
	code, err := g.logins.Complete(r.Context(), flow, usecase.GoogleCallback{
		Code: q.Get("code"), State: q.Get("state"), ProviderError: q.Get("error"),
	})
	if err != nil {
		log.Printf("google login: callback: %v", err)
		code = ""
	}
	http.Redirect(w, r, g.completeURL(code), http.StatusSeeOther)
}

type googleExchangeRequest struct {
	Code string `json:"code"`
}

// googleSignedInResponse は、サインインの成功の応答である(POST /login と同じ形に、戻り先を足したもの)。
type googleSignedInResponse struct {
	authUserResponse
	ReturnTo string `json:"return_to"`
}

type googleLinkedResponse struct {
	Linked   bool   `json:"linked"`
	ReturnTo string `json:"return_to"`
}

// HandleExchange は POST /auth/google/exchange を処理する: 「画面へ渡すコード」を 1 回だけ使って、手続きの結果を返す。
// サインインの成功は 200(POST /login と同じ本文 + return_to。JWT はここでだけ返す)、結び付けの成功は 200、
// 重複などは 409、失敗・無効なコードは 400。
func (g *GoogleLogin) HandleExchange(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var req googleExchangeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := g.logins.Redeem(r.Context(), req.Code)
	if err != nil {
		if errors.Is(err, domain.ErrLoginHandoffInvalid) {
			writeJSON(w, http.StatusBadRequest, errorsResponse{Errors: []string{googleCodeInvalidMessage}})
			return
		}
		log.Printf("google login: exchange: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	switch res.Outcome {
	case domain.OutcomeSignedIn:
		writeJSON(w, http.StatusOK, googleSignedInResponse{authUserResponse: newAuthUserResponse(res.User, res.Token), ReturnTo: res.ReturnTo})
	case domain.OutcomeLinked:
		writeJSON(w, http.StatusOK, googleLinkedResponse{Linked: true, ReturnTo: res.ReturnTo})
	case domain.OutcomeAccountExists:
		writeJSON(w, http.StatusConflict, errorsResponse{Errors: []string{googleAccountExistsMessage}})
	case domain.OutcomeIdentityTaken:
		writeJSON(w, http.StatusConflict, errorsResponse{Errors: []string{googleIdentityTakenMessage}})
	case domain.OutcomeAlreadyLinked:
		writeJSON(w, http.StatusConflict, errorsResponse{Errors: []string{googleAlreadyLinkedMessage}})
	default:
		writeJSON(w, http.StatusBadRequest, errorsResponse{Errors: []string{googleSignInFailedMessage}})
	}
}

type googleLinkIntentResponse struct {
	LinkCode string `json:"link_code"`
}

// HandleLinkIntent は POST /me/identities/google/link を処理する(RequireAuth の背後): 結び付けの手続きを始めるための、
// 1 回だけ使える短命のコードを返す。画面は、このコードを付けて /auth/google/start へ移動する。
func (g *GoogleLogin) HandleLinkIntent(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	viewer, ok := requireViewer(w, r)
	if !ok {
		return
	}
	code, err := g.logins.IssueLinkIntent(r.Context(), viewer)
	if err != nil {
		log.Printf("google login: link intent: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, googleLinkIntentResponse{LinkCode: code})
}

type identityResponse struct {
	Provider    string    `json:"provider"`
	Email       string    `json:"email"`
	ConnectedAt time.Time `json:"connected_at"`
	// CanUnlink は、解除してよいかである(解除したあとも、サインインする方法が残るか。判断は domain)。
	CanUnlink bool `json:"can_unlink"`
}

type identitiesResponse struct {
	Identities []identityResponse `json:"identities"`
}

// HandleListIdentities は GET /me/identities を処理する(RequireAuth の背後): 外部のサービスとの結び付きを返す。
func (g *GoogleLogin) HandleListIdentities(w http.ResponseWriter, r *http.Request) {
	viewer, ok := requireViewer(w, r)
	if !ok {
		return
	}
	views, err := g.logins.ListIdentities(r.Context(), viewer)
	if err != nil {
		log.Printf("google login: list identities: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	resp := identitiesResponse{Identities: make([]identityResponse, 0, len(views))}
	for _, v := range views {
		resp.Identities = append(resp.Identities, identityResponse{Provider: v.Provider, Email: v.Email, ConnectedAt: v.CreatedAt, CanUnlink: v.CanUnlink})
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleUnlinkGoogle は DELETE /me/identities/google を処理する(RequireAuth の背後): 結び付きを解除して 204。
// 解除するとサインインする方法がなくなるときは 422、結び付きがなければ 404。
func (g *GoogleLogin) HandleUnlinkGoogle(w http.ResponseWriter, r *http.Request) {
	viewer, ok := requireViewer(w, r)
	if !ok {
		return
	}
	err := g.logins.Unlink(r.Context(), viewer, domain.ProviderGoogle)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, domain.ErrCannotUnlinkIdentity):
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{googleCannotUnlinkMessage}})
	case errors.Is(err, domain.ErrIdentityNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		log.Printf("google login: unlink: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
