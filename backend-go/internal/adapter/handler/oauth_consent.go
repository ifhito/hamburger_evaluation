package handler

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// OAuth は、OAuth の認可サーバーに関する窓口をまとめたものである。NewRouter に nil を渡すと、
// 認可サーバーは無効で、これらの URL は登録されない。
type OAuth struct {
	// Endpoints は、認可・トークン・取り消し・認可サーバーの情報の窓口である。
	Endpoints OAuthEndpoints
	// Consents は、利用者に許可を尋ねる画面(frontend)が使う API の use case である。
	Consents *usecase.OAuthConsents
	// Apps は、許可したアプリの一覧と取り消しの use case である。
	Apps *usecase.ConnectedApps
}

type oauthScopeResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func scopeResponses(scopes []domain.OAuthScope) []oauthScopeResponse {
	out := make([]oauthScopeResponse, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, oauthScopeResponse{Name: s.Name, Description: s.Description})
	}
	return out
}

// namedScopeResponses は、範囲の名前の一覧を、説明つきの形にする(知らない名前は、説明なしで残す)。
func namedScopeResponses(names []string) []oauthScopeResponse {
	out := make([]oauthScopeResponse, 0, len(names))
	for _, n := range names {
		desc := ""
		if s, ok := domain.OAuthScopeByName(n); ok {
			desc = s.Description
		}
		out = append(out, oauthScopeResponse{Name: n, Description: desc})
	}
	return out
}

type consentClientResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type consentResponse struct {
	Client          consentClientResponse `json:"client"`
	Scopes          []oauthScopeResponse  `json:"scopes"`
	ConsentRequired bool                  `json:"consent_required"`
}

// handleOAuthAuthorizeRequest は GET /oauth/authorize/request を処理する:許可の画面が、認可の URL の
// 値(URL の `?` のあと)をそのまま付けて呼び、画面に出す内容(アプリの名前・求められた範囲と説明・
// 尋ねる必要があるか)を受け取る(200)。要求がアプリへ結果を戻せない形で不正なときは 422。
func handleOAuthAuthorizeRequest(consents *usecase.OAuthConsents) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		view, err := consents.Describe(r.Context(), viewer.ID, r.URL.Query())
		if err != nil {
			writeOAuthError(w, "describe authorize request", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, consentResponse{
			Client:          consentClientResponse{ID: view.ClientID, Name: view.ClientName},
			Scopes:          scopeResponses(view.Scopes),
			ConsentRequired: view.ConsentRequired,
		})
	}
}

type decisionRequest struct {
	// Query は、認可の URL の値(URL の `?` のあとの文字列)である。
	Query string `json:"query"`
	// Approve は、許可するか(true)拒否するか(false)である。必須。
	Approve *bool `json:"approve"`
}

type decisionResponse struct {
	// RedirectTo は、画面が利用者のブラウザで開く、アプリへの戻り先である(認可コードまたはエラーが付く)。
	RedirectTo string `json:"redirect_to"`
}

// handleOAuthDecision は POST /oauth/authorize/decision を処理する:利用者の許可・拒否を受けて、アプリへ戻す
// URL を返す(200)。許可なら、許可の記録を残して認可コードを発行する。要求は、ここでも検証し直す。
// 本文の不備は 422、アプリへ結果を戻せない不正な要求も 422。
func handleOAuthDecision(consents *usecase.OAuthConsents) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		var body decisionRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Approve == nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"Approve is required"}})
			return
		}
		params, err := usecase.ParseAuthorizeQuery(body.Query)
		if err != nil {
			writeOAuthError(w, "decide", err)
			return
		}
		redirectTo, err := consents.Decide(r.Context(), viewer.ID, params, *body.Approve)
		if err != nil {
			writeOAuthError(w, "decide", err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, decisionResponse{RedirectTo: redirectTo})
	}
}

type grantResponse struct {
	ID         string               `json:"id"`
	ClientID   string               `json:"client_id"`
	ClientName string               `json:"client_name"`
	Scopes     []oauthScopeResponse `json:"scopes"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// handleListOAuthGrants は GET /oauth/grants を処理する:閲覧者が許可したアプリの一覧を、最近使ったものから順に、
// ページ送りで返す(200)。page / per_page は、既存の一覧(GET /shops・GET /reviews)と同じ契約で、整数でなければ
// 422、範囲外の整数は補正される。次のページがあるかは、レスポンスヘッダー X-Has-More(true / false)で返す。
func handleListOAuthGrants(apps *usecase.ConnectedApps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		page, perPage, ok := pageParams(w, r)
		if !ok {
			return
		}
		grants, hasMore, err := apps.List(r.Context(), viewer.ID, page, perPage)
		if err != nil {
			writeOAuthError(w, "list grants", err)
			return
		}
		out := make([]grantResponse, 0, len(grants))
		for _, g := range grants {
			out = append(out, grantResponse{
				ID: g.ID, ClientID: g.ClientID, ClientName: g.ClientName,
				Scopes: namedScopeResponses(g.Scopes), CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		setHasMore(w, hasMore)
		writeJSON(w, http.StatusOK, out)
	}
}

// handleRevokeOAuthGrant は DELETE /oauth/grants/{id} を処理する:閲覧者本人の許可を取り消す(204)。そのアプリの
// トークンは、すぐに使えなくなる。正規形でない id・存在しない許可・別の利用者の許可は、区別できない同一の 404。
func handleRevokeOAuthGrant(apps *usecase.ConnectedApps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		if err := apps.Revoke(r.Context(), viewer.ID, r.PathValue("id")); err != nil {
			writeOAuthError(w, "revoke grant", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeOAuthError は、OAuth の許可に関する use case のエラーを、HTTP の応答にする。
func writeOAuthError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid), errors.Is(err, domain.ErrOAuthInvalidScope):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrOAuthGrantNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		log.Printf("handler: oauth %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
