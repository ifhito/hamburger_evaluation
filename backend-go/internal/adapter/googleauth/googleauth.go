package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// DefaultIssuer は、Google の OpenID Connect の提供元(発行者)の URL である。
const DefaultIssuer = "https://accounts.google.com"

// httpTimeout は、Google への 1 回の HTTP 要求(探索の情報・公開鍵・コードの交換)の待ち時間の上限である。
const httpTimeout = 10 * time.Second

// stateBytes は、state と nonce の乱数のバイト数である(256 ビット)。
const stateBytes = 32

// Config は、Google の OAuth クライアントの設定である。
type Config struct {
	// ClientID と ClientSecret は、Google Cloud Console で作った OAuth クライアントの値である。ClientSecret は
	// 秘密で、ログ・エラーの文言に含めない。
	ClientID     string
	ClientSecret string
	// RedirectURL は、Google が認可のあとに利用者を戻す URL(この API の /auth/google/callback)である。Google Cloud
	// Console に登録した「承認済みのリダイレクト URI」と、完全に一致しなければならない。
	RedirectURL string
	// Issuer は、OpenID Connect の提供元の URL である。空なら DefaultIssuer(Google)。テストや隔離した確認で、
	// 別の提供元に差し替えるためにある(https、または、ループバックの http だけを許す検査は、設定を読む側にある)。
	Issuer string
}

// Provider は usecase.GoogleProvider の実装である。提供元の探索の情報(認可・トークンの窓口と公開鍵の場所)は、
// 最初に必要になったときに取得して覚える(起動時に Google へつながらなくても、API は起動できる)。
type Provider struct {
	cfg    Config
	client *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
}

var _ usecase.GoogleProvider = (*Provider)(nil)

// New は cfg の Provider を返す。
func New(cfg Config) *Provider {
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: httpTimeout}}
}

// discover は、提供元の探索の情報を返す(取得済みならそれを返し、まだなら取得する。失敗は覚えず、次の呼び出しで再び試す)。
func (p *Provider) discover(ctx context.Context) (*oidc.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return p.provider, nil
	}
	prov, err := oidc.NewProvider(oidc.ClientContext(ctx, p.client), p.cfg.Issuer)
	if err != nil {
		return nil, errors.New("googleauth: discovery failed")
	}
	p.provider = prov
	return prov, nil
}

func (p *Provider) oauth2Config(prov *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		RedirectURL:  p.cfg.RedirectURL,
		Endpoint:     prov.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

func randomString() (string, error) {
	buf := make([]byte, stateBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("googleauth: generate random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Begin は、Google の認可の画面の URL と、この手続きの秘密の値(state・nonce・PKCE の検証値)を作る。
// PKCE は S256 を使う。
func (p *Provider) Begin(ctx context.Context) (string, usecase.GoogleFlowSecrets, error) {
	prov, err := p.discover(ctx)
	if err != nil {
		return "", usecase.GoogleFlowSecrets{}, err
	}
	state, err := randomString()
	if err != nil {
		return "", usecase.GoogleFlowSecrets{}, err
	}
	nonce, err := randomString()
	if err != nil {
		return "", usecase.GoogleFlowSecrets{}, err
	}
	verifier := oauth2.GenerateVerifier()
	authURL := p.oauth2Config(prov).AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oidc.Nonce(nonce),
		// 複数の Google アカウントを持つ利用者が、使うアカウントを選べるようにする。
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
	return authURL, usecase.GoogleFlowSecrets{State: state, Nonce: nonce, Verifier: verifier}, nil
}

// idClaims は、ID トークンから読む、利用者の情報である。
type idClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// Complete は、認可コードを Google と交換し、ID トークンを検証して、Google が確かめた利用者の情報を返す。
// 検証は、署名(Google の公開鍵)・発行者・宛先(このアプリのクライアント ID)・有効期限・nonce である。
// エラーの文言は、原因の種類だけを表す固定の文言で、トークン・認可コード・秘密の鍵を含まない。
func (p *Provider) Complete(ctx context.Context, code string, secrets usecase.GoogleFlowSecrets) (domain.ExternalIdentity, error) {
	prov, err := p.discover(ctx)
	if err != nil {
		return domain.ExternalIdentity{}, err
	}
	ctx = oidc.ClientContext(ctx, p.client)
	tok, err := p.oauth2Config(prov).Exchange(ctx, code, oauth2.VerifierOption(secrets.Verifier))
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil {
			return domain.ExternalIdentity{}, fmt.Errorf("googleauth: token exchange failed: status %d %s", re.Response.StatusCode, re.ErrorCode)
		}
		return domain.ExternalIdentity{}, errors.New("googleauth: token exchange failed")
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return domain.ExternalIdentity{}, errors.New("googleauth: id_token is missing")
	}
	idToken, err := prov.Verifier(&oidc.Config{ClientID: p.cfg.ClientID}).Verify(ctx, rawID)
	if err != nil {
		// go-oidc のエラーは、トークンそのものを含まない(署名・発行者・宛先・期限の不一致を説明するだけ)。
		return domain.ExternalIdentity{}, fmt.Errorf("googleauth: id_token verification failed: %w", err)
	}
	if secrets.Nonce == "" || idToken.Nonce != secrets.Nonce {
		return domain.ExternalIdentity{}, errors.New("googleauth: nonce mismatch")
	}
	var claims idClaims
	if err := idToken.Claims(&claims); err != nil {
		return domain.ExternalIdentity{}, errors.New("googleauth: id_token claims are invalid")
	}
	return domain.ExternalIdentity{
		Provider:       domain.ProviderGoogle,
		ProviderUserID: idToken.Subject,
		Email:          claims.Email,
		EmailVerified:  claims.EmailVerified,
		Name:           claims.Name,
	}, nil
}
