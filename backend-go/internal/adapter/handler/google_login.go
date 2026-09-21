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
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"sort"
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

// googleFlowMaxFlows は、1 つのブラウザが、同時に進められる手続きの数の上限である(複数のタブで、続けて始めても、
// 後発が先発を上書きしないため)。超えたら、いちばん古いものから捨てる。
const googleFlowMaxFlows = 5

// googleFlowCookieMaxValueBytes は、封じた cookie の値の大きさの上限である(ブラウザの上限は、名前・値・属性を含めて
// 約 4,096 バイト)。超えるときは、いちばん古い手続きから捨てる。
const googleFlowCookieMaxValueBytes = 3800

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
	path := googleFlowCookiePath(redirect.Path)
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

// googleCallbackSuffix は、Google からの戻り先の path の末尾である(この API の /auth/google/callback)。
const googleCallbackSuffix = "/auth/google/callback"

// googleFlowCookiePath は、手続きの cookie の Path を、戻り先の path から決める。**開始(/auth/google/start・
// /me/identities/google/link)と戻り(/auth/google/callback)の、すべての要求に、cookie が送られる**ように、
// 戻り先から /auth/google/callback を除いた共通の親にする(例: 戻り先が /api/auth/google/callback なら /api、
// API 専用のホストで /auth/google/callback なら /)。ブラウザは、Path が合わない要求には cookie を送らないので、
// 戻り先の path だけに絞ると、開始の要求に、先発の手続きの cookie が届かず、後発が先発を上書きしてしまう。
// 戻り先が /auth/google/callback で終わらないとき(想定外の設定)は、戻り先の path にする(手続きは 1 つだけ持てる)。
func googleFlowCookiePath(redirectPath string) string {
	if !strings.HasSuffix(redirectPath, googleCallbackSuffix) {
		if redirectPath == "" {
			return "/"
		}
		return redirectPath
	}
	if parent := strings.TrimSuffix(redirectPath, googleCallbackSuffix); parent != "" {
		return parent
	}
	return "/"
}

// flowCookie は、進行中の手続き(state・nonce・PKCE の検証値・戻り先・結び付ける利用者)を、暗号化して cookie に
// 封じる。1 つのブラウザが、複数のタブで続けて手続きを始めても、それぞれが残るように、手続きを最大
// googleFlowMaxFlows 件まで、1 つの cookie に並べて持つ(state で取り出す)。HttpOnly(画面の JavaScript からは
// 読めない)・SameSite=Lax(Google からの戻りの移動では付く)で、path を API の入口(開始・結び付け・戻りの共通の親)に限り、使い終わった手続きは
// 取り除き、空になったら cookie を消す。サーバー側にセッションは持たない。
type flowCookie struct {
	key    []byte
	path   string
	secure bool
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

func (c flowCookie) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal は entries を暗号化して、cookie の値にする。改ざんは、復号のときに検出される。
func (c flowCookie) seal(entries []flowEntry) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // 戻り先の "&" などを、6 バイトに膨らませない(cookie の大きさの上限のため)
	if err := enc.Encode(entries); err != nil {
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
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, buf.Bytes(), []byte(googleFlowCookieName))), nil
}

// open は、cookie の値を復号して、期限内の手続きを返す。改ざん・壊れた値は、空を返す。
func (c flowCookie) open(value string, now time.Time) []flowEntry {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil
	}
	gcm, err := c.aead()
	if err != nil || len(raw) < gcm.NonceSize() {
		return nil
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(googleFlowCookieName))
	if err != nil {
		return nil
	}
	var entries []flowEntry
	if json.Unmarshal(plain, &entries) != nil {
		return nil
	}
	live := entries[:0]
	for _, e := range entries {
		if now.Unix() <= e.ExpiresAt {
			live = append(live, e)
		}
	}
	return live
}

// read は、リクエストの cookie から、進行中の手続き(期限内)を、始めた順に返す。**同じ名前の cookie が複数
// 送られてきたときは、すべてを合わせて読む**(重複は state で除く)。ブラウザは、Path が違う同名の cookie
// (たとえば、cookie の Path を変えたデプロイの直後に、古い Path のまま残っているもの)を、Path が長いものを先に
// 並べて、すべて送る。先頭の 1 つだけを読むと、新しい cookie の手続きが見つからず、手続きが失敗してしまう。
func (c flowCookie) read(r *http.Request, now time.Time) []flowEntry {
	var entries []flowEntry
	seen := map[string]bool{}
	for _, cookie := range r.Cookies() {
		if cookie.Name != googleFlowCookieName {
			continue
		}
		for _, e := range c.open(cookie.Value, now) {
			if !seen[e.State] {
				seen[e.State] = true
				entries = append(entries, e)
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ExpiresAt < entries[j].ExpiresAt })
	return entries
}

// write は、entries を cookie として応答に設定する。件数が googleFlowMaxFlows を超えるとき、または封じた値が
// googleFlowCookieMaxValueBytes を超えるときは、いちばん古い手続きから捨てる。空なら、cookie を消す。
// 1 件だけでも大きすぎるときは、エラーを返す。
func (c flowCookie) write(w http.ResponseWriter, entries []flowEntry) error {
	if len(entries) > googleFlowMaxFlows {
		entries = entries[len(entries)-googleFlowMaxFlows:]
	}
	if len(entries) == 0 {
		http.SetCookie(w, c.cookie("", -1))
		return nil
	}
	for {
		value, err := c.seal(entries)
		if err != nil {
			return err
		}
		if len(value) <= googleFlowCookieMaxValueBytes {
			http.SetCookie(w, c.cookie(value, int(googleFlowCookieTTL.Seconds())))
			return nil
		}
		if len(entries) == 1 {
			return errors.New("google login: flow cookie is too large")
		}
		entries = entries[1:]
	}
}

func (c flowCookie) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: googleFlowCookieName, Value: value, Path: c.path, MaxAge: maxAge,
		HttpOnly: true, Secure: c.secure, SameSite: http.SameSiteLaxMode,
	}
}

// add は、新しい手続きを、cookie にすでにある手続きに足して、応答に設定する(先発の手続きは残る)。
func (c flowCookie) add(w http.ResponseWriter, r *http.Request, flow usecase.GoogleFlow, now time.Time) error {
	entries := c.read(r, now)
	entries = append(entries, flowEntry{
		State: flow.Secrets.State, Nonce: flow.Secrets.Nonce, Verifier: flow.Secrets.Verifier,
		ReturnTo: flow.ReturnTo, LinkUserID: flow.LinkUserID, ExpiresAt: now.Add(googleFlowCookieTTL).Unix(),
	})
	return c.write(w, entries)
}

// take は、state に対応する手続きを取り出し、cookie からその手続きだけを取り除く(1 回の手続きにだけ使う。
// ほかのタブの手続きは残す)。対応する手続きがなければ、見つからなかったことを返し、cookie は変えない。
func (c flowCookie) take(w http.ResponseWriter, r *http.Request, state string, now time.Time) (usecase.GoogleFlow, bool) {
	entries := c.read(r, now)
	for i, e := range entries {
		if state != "" && subtle.ConstantTimeCompare([]byte(e.State), []byte(state)) == 1 {
			rest := append(append([]flowEntry{}, entries[:i]...), entries[i+1:]...)
			if err := c.write(w, rest); err != nil {
				http.SetCookie(w, c.cookie("", -1))
			}
			return e.flow(), true
		}
	}
	return usecase.GoogleFlow{}, false
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
		err = g.cookie.add(w, r, begin.Flow, time.Now())
	}
	if err != nil {
		log.Printf("google login: start: %v", err)
		http.Redirect(w, r, g.completeURL(""), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, begin.AuthURL, http.StatusFound)
}

// HandleCallback は GET /auth/google/callback を処理する: Google から戻ってきた要求を処理し、結果を入れた
// 「画面へ渡すコード」を付けて、frontend の結果の画面へ 303 で送る。使った手続きは、成否にかかわらず cookie から取り除く。
func (g *GoogleLogin) HandleCallback(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	q := r.URL.Query()
	// state に対応する手続きだけを取り出す(ほかのタブの手続きは、cookie に残る)。なければ、空の手続きになり、失敗になる。
	flow, _ := g.cookie.take(w, r, q.Get("state"), time.Now())
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
		err = g.cookie.add(w, r, begin.Flow, time.Now())
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
