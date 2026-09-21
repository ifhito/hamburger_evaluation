package handler

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
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

// googleFlowCookiePrefix は、1 回のサインインの手続きの間だけ持ち回る値を入れる cookie の名前の前置きである。
// **手続きごとに別の cookie**(名前に state のハッシュを付ける)にするので、複数のタブで手続きを並行しても、互いを
// 上書きせず、使い終わった手続きの cookie だけを消せる(Path も、戻りの要求だけに絞れる)。
const googleFlowCookiePrefix = "google_login_flow_"

// googleHandoffCookiePrefix は、「画面へ渡すコード」を使える相手を確かめる「結び付けの値」を入れる cookie の名前の
// 前置きである(コードごとに別の cookie)。
const googleHandoffCookiePrefix = "google_login_handoff_"

// googleFlowCookieTTL は、手続きの間の cookie の有効期間である(Google の画面での操作を待てる長さ)。
const googleFlowCookieTTL = 10 * time.Minute

// googleFlowCookieInfo は、cookie の暗号鍵を、JWT の秘密から導くときの用途の名前である(鍵の使い回しを避ける)。
const googleFlowCookieInfo = "google-login-flow-cookie-v1"

// googleCallbackSuffix・googleExchangeSuffix は、Google からの戻り先の path の末尾(この API の /auth/google/callback)と、
// 結果との交換の path の末尾(/auth/google/exchange)である。公開の path には、前に接頭辞(たとえば /api)が付くことが
// あるが、末尾は変わらない。
const (
	googleCallbackSuffix = "/auth/google/callback"
	googleExchangeSuffix = "/auth/google/exchange"
)

// GoogleLoginConfig は、Google でのサインインの窓口の設定である。
type GoogleLoginConfig struct {
	// AppBaseURL は、frontend の URL である(末尾に / を付けない)。手続きの結果は、この下の
	// /auth/google/complete へ戻して渡す。
	AppBaseURL string
	// RedirectURL は、Google が認可のあとに利用者を戻す URL(この API の /auth/google/callback の公開 URL)である。
	// path は /auth/google/callback で終わらなければならない(手続きの cookie の Path と、結果との交換の cookie の
	// Path を、ここから決める)。Secure の判断(https なら付ける)にも使う。
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

// NewGoogleLogin は GoogleLogin を組み立てる。cfg が不正(戻り先が URL でない・path が /auth/google/callback で終わらない、
// 秘密が空)なときはエラーを返す。
func NewGoogleLogin(logins *usecase.GoogleLogins, cfg GoogleLoginConfig) (*GoogleLogin, error) {
	redirect, err := url.Parse(cfg.RedirectURL)
	if err != nil || redirect.Host == "" {
		return nil, errors.New("google login: redirect URL is invalid")
	}
	if !strings.HasSuffix(redirect.Path, googleCallbackSuffix) {
		return nil, errors.New("google login: redirect URL path must end with " + googleCallbackSuffix)
	}
	if cfg.CookieSecret == "" {
		return nil, errors.New("google login: cookie secret is empty")
	}
	key, err := hkdf.Key(sha256.New, []byte(cfg.CookieSecret), nil, googleFlowCookieInfo, 32)
	if err != nil {
		return nil, errors.New("google login: derive cookie key")
	}
	prefix := strings.TrimSuffix(redirect.Path, googleCallbackSuffix)
	return &GoogleLogin{
		logins:  logins,
		appBase: strings.TrimRight(cfg.AppBaseURL, "/"),
		cookie: flowCookie{
			key:          key,
			callbackPath: redirect.Path,
			exchangePath: prefix + googleExchangeSuffix,
			secure:       redirect.Scheme == "https",
		},
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

// flowCookie は、進行中の手続き(state・nonce・PKCE の検証値・戻り先・結び付ける利用者)を、暗号化して cookie に
// 封じる。**手続きごとに 1 つの cookie**(名前は state から決まる)で、Path は戻りの要求(callbackPath)だけに絞る
// (開始の要求には、送られない)。HttpOnly(画面の JavaScript からは読めない)・SameSite=Lax(Google からの戻りの
// 移動では付く)で、使い終わった手続きの cookie は、その場で消す。サーバー側にセッションは持たない。
//
// もう 1 種類の cookie(結び付けの値)は、戻りの応答で、手続きを終えたブラウザに設定し、結果との交換
// (exchangePath)の要求にだけ送られる。「画面へ渡すコード」は URL に載って渡るので、コードだけでは、交換できない。
type flowCookie struct {
	key          []byte
	callbackPath string
	exchangePath string
	secure       bool
}

// flowEntry は、進行中の 1 つの手続きである。
type flowEntry struct {
	State      string `json:"s"`
	Nonce      string `json:"n"`
	Verifier   string `json:"v"`
	ReturnTo   string `json:"r,omitempty"`
	LinkUserID string `json:"l,omitempty"`
	ExpiresAt  int64  `json:"e"`
}

func (e flowEntry) flow() usecase.GoogleFlow {
	return usecase.GoogleFlow{
		Secrets:    usecase.GoogleFlowSecrets{State: e.State, Nonce: e.Nonce, Verifier: e.Verifier},
		ReturnTo:   e.ReturnTo,
		LinkUserID: e.LinkUserID,
	}
}

// cookieName は、prefix と、秘密でない識別の値(state・コード)から、cookie の名前を決める(値そのものは名前に出さず、
// ハッシュの先頭を使う。要求の値がそのまま名前になって、想定外の名前を引かないためでもある)。
func cookieName(prefix, id string) string {
	sum := sha256.Sum256([]byte(id))
	return prefix + hex.EncodeToString(sum[:12])
}

func (c flowCookie) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal は entry を暗号化して、cookie の値にする。cookie の名前を認証に含める(別の名前の cookie に、値を移し替えても、
// 開けない)。改ざんは、復号のときに検出される。
func (c flowCookie) seal(name string, entry flowEntry) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entry); err != nil {
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
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, buf.Bytes(), []byte(name))), nil
}

// open は、cookie の値を復号して、期限内の手続きを返す。改ざん・壊れた値・別の名前の cookie の値・期限切れは、
// 見つからなかったことを返す。
func (c flowCookie) open(name, value string, now time.Time) (flowEntry, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return flowEntry{}, false
	}
	gcm, err := c.aead()
	if err != nil || len(raw) < gcm.NonceSize() {
		return flowEntry{}, false
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(name))
	if err != nil {
		return flowEntry{}, false
	}
	var e flowEntry
	if json.Unmarshal(plain, &e) != nil || now.Unix() > e.ExpiresAt {
		return flowEntry{}, false
	}
	return e, true
}

func (c flowCookie) newCookie(name, value, path string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, HttpOnly: true, Secure: c.secure, SameSite: http.SameSiteLaxMode}
}

// set は、新しい手続きを、その手続き専用の cookie として応答に設定する(ほかの手続きの cookie には触れない)。
func (c flowCookie) set(w http.ResponseWriter, flow usecase.GoogleFlow, now time.Time) error {
	name := cookieName(googleFlowCookiePrefix, flow.Secrets.State)
	value, err := c.seal(name, flowEntry{
		State: flow.Secrets.State, Nonce: flow.Secrets.Nonce, Verifier: flow.Secrets.Verifier,
		ReturnTo: flow.ReturnTo, LinkUserID: flow.LinkUserID, ExpiresAt: now.Add(googleFlowCookieTTL).Unix(),
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, c.newCookie(name, value, c.callbackPath, int(googleFlowCookieTTL.Seconds())))
	return nil
}

// take は、state に対応する手続きを取り出し、その手続きの cookie を消す(1 回の手続きにだけ使う。ほかのタブの
// 手続きの cookie は残る)。cookie がない・壊れている・別の秘密で封じた・期限切れ・state が合わないときは、
// 見つからなかったことを返す(cookie があれば、消す)。
func (c flowCookie) take(w http.ResponseWriter, r *http.Request, state string, now time.Time) (usecase.GoogleFlow, bool) {
	if state == "" {
		return usecase.GoogleFlow{}, false
	}
	name := cookieName(googleFlowCookiePrefix, state)
	cookie, err := r.Cookie(name)
	if err != nil {
		return usecase.GoogleFlow{}, false
	}
	http.SetCookie(w, c.newCookie(name, "", c.callbackPath, -1))
	e, ok := c.open(name, cookie.Value, now)
	if !ok || subtle.ConstantTimeCompare([]byte(e.State), []byte(state)) != 1 {
		return usecase.GoogleFlow{}, false
	}
	return e.flow(), true
}

// setBinder は、「画面へ渡すコード」を使える相手を確かめる「結び付けの値」を、手続きを終えたブラウザに設定する
// (結果との交換の要求にだけ送られる。有効期間は、コードと同じ)。
func (c flowCookie) setBinder(w http.ResponseWriter, code, binder string) {
	http.SetCookie(w, c.newCookie(cookieName(googleHandoffCookiePrefix, code), binder, c.exchangePath, int(domain.LoginHandoffTTL.Seconds())))
}

// binder は、このコードの「結び付けの値」を、要求の cookie から返す(なければ空)。
func (c flowCookie) binder(r *http.Request, code string) string {
	cookie, err := r.Cookie(cookieName(googleHandoffCookiePrefix, code))
	if err != nil {
		return ""
	}
	return cookie.Value
}

// clearBinder は、使い終わったコードの「結び付けの値」の cookie を消す。
func (c flowCookie) clearBinder(w http.ResponseWriter, code string) {
	http.SetCookie(w, c.newCookie(cookieName(googleHandoffCookiePrefix, code), "", c.exchangePath, -1))
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
// query の return_to(手続きのあとに戻る先。アプリの中のパスだけ)を受ける。**サインイン・新規登録の手続き専用**で、
// 結び付けは、認証つきの POST /me/identities/google/link から始める(この URL に、結び付ける利用者を伝える経路はない)。
// 始められなかったときは、frontend の結果の画面へ、コードなしで送る(画面が、API から失敗の文言を受け取る)。
func (g *GoogleLogin) HandleStart(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	begin, err := g.logins.Begin(r.Context(), r.URL.Query().Get("return_to"))
	if err == nil {
		err = g.cookie.set(w, begin.Flow, time.Now())
	}
	if err != nil {
		log.Printf("google login: start: %v", err)
		http.Redirect(w, r, g.completeURL(""), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, begin.AuthURL, http.StatusFound)
}

// HandleCallback は GET /auth/google/callback を処理する: Google から戻ってきた要求を処理し、結果を入れた
// 「画面へ渡すコード」を付けて、frontend の結果の画面へ 303 で送る。同時に、コードを使える相手を確かめる「結び付けの値」を、
// このブラウザの cookie に設定する(コードだけでは、交換できない)。使った手続きの cookie は、成否にかかわらず消す。
//
// **手続きを始めたブラウザの cookie がない(または、合わない)要求では、何も保存せず**、コードなしで結果の画面へ送る
// (手続きを始めていない要求で、DB に書き込ませない)。
func (g *GoogleLogin) HandleCallback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	q := r.URL.Query()
	flow, ok := g.cookie.take(w, r, q.Get("state"), time.Now())
	if !ok {
		http.Redirect(w, r, g.completeURL(""), http.StatusSeeOther)
		return
	}
	issued, err := g.logins.Complete(r.Context(), flow, usecase.GoogleCallback{
		Code: q.Get("code"), State: q.Get("state"), ProviderError: q.Get("error"),
	})
	if err != nil {
		log.Printf("google login: callback: %v", err)
		http.Redirect(w, r, g.completeURL(""), http.StatusSeeOther)
		return
	}
	g.cookie.setBinder(w, issued.Code, issued.Binder)
	http.Redirect(w, r, g.completeURL(issued.Code), http.StatusSeeOther)
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

// googleFailureResponse は、手続きの失敗(409・400)の応答である。検証済みの戻り先(return_to)を含める(AI アプリの
// 許可の画面から来た利用者が、失敗のあと、元の要求へ戻れるように。画面は、これをサインインの画面へ渡すだけである)。
type googleFailureResponse struct {
	Errors   []string `json:"errors"`
	ReturnTo string   `json:"return_to"`
}

// HandleExchange は POST /auth/google/exchange を処理する: 「画面へ渡すコード」を 1 回だけ使って、手続きの結果を返す。
// サインインの成功は 200(POST /login と同じ本文 + return_to。JWT はここでだけ返す)、結び付けの成功は 200、
// 重複などは 409、失敗は 400(どちらも、errors と return_to)、無効なコードは 400 である。
//
// コードは、手続きを終えたブラウザの cookie にある「結び付けの値」と一緒でなければ使えない(cookie がない・合わない
// 要求は、無効なコードと同じ 400 になり、コードは消費されない)。コードだけを別のブラウザへ持ち込んでも、
// 交換できない(ログイン CSRF の防止)。
func (g *GoogleLogin) HandleExchange(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var req googleExchangeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := g.logins.Redeem(r.Context(), req.Code, g.cookie.binder(r, req.Code))
	if err != nil {
		if errors.Is(err, domain.ErrLoginHandoffInvalid) {
			g.cookie.clearBinder(w, req.Code)
			writeJSON(w, http.StatusBadRequest, errorsResponse{Errors: []string{googleCodeInvalidMessage}})
			return
		}
		log.Printf("google login: exchange: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	g.cookie.clearBinder(w, req.Code) // コードは使い切った
	fail := func(status int, message string) {
		writeJSON(w, status, googleFailureResponse{Errors: []string{message}, ReturnTo: res.ReturnTo})
	}
	switch res.Outcome {
	case domain.OutcomeSignedIn:
		writeJSON(w, http.StatusOK, googleSignedInResponse{authUserResponse: newAuthUserResponse(res.User, res.Token), ReturnTo: res.ReturnTo})
	case domain.OutcomeLinked:
		writeJSON(w, http.StatusOK, googleLinkedResponse{Linked: true, ReturnTo: res.ReturnTo})
	case domain.OutcomeAccountExists:
		fail(http.StatusConflict, googleAccountExistsMessage)
	case domain.OutcomeIdentityTaken:
		fail(http.StatusConflict, googleIdentityTakenMessage)
	case domain.OutcomeAlreadyLinked:
		fail(http.StatusConflict, googleAlreadyLinkedMessage)
	default:
		fail(http.StatusBadRequest, googleSignInFailedMessage)
	}
}

type googleLinkStartRequest struct {
	ReturnTo string `json:"return_to"`
}

type googleLinkStartResponse struct {
	// RedirectURL は、利用者のブラウザを送る、Google の認可の画面の URL である。
	RedirectURL string `json:"redirect_url"`
}

// HandleLinkStart は POST /me/identities/google/link を処理する(RequireAuth の背後): 結び付けの手続きを、**認証つきの
// 要求を出したブラウザ**で始める。結び付ける利用者は、要求の認証(Bearer)から決まり、手続きの cookie(応答の
// Set-Cookie で、この要求を出したブラウザにだけ設定される)に封じられる。応答は、Google の認可の画面の URL で、画面は、
// そこへ移動する。cookie を持たないブラウザ(たとえば、この URL を別のブラウザに開かせた被害者)では、戻ってきても
// state が合わず、失敗する(被害者の Google が攻撃者のアカウントに結び付けられることはない)。body の return_to
// (手続きのあとに戻る先)は省略できる。
func (g *GoogleLogin) HandleLinkStart(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	viewer, ok := requireViewer(w, r)
	if !ok {
		return
	}
	var req googleLinkStartRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	begin, err := g.logins.BeginLink(r.Context(), viewer, req.ReturnTo)
	if err == nil {
		err = g.cookie.set(w, begin.Flow, time.Now())
	}
	if err != nil {
		log.Printf("google login: link start: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, googleLinkStartResponse{RedirectURL: begin.AuthURL})
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
