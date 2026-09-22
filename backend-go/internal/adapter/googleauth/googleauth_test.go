package googleauth_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/googleauth"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/fakeoidc"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	testClientID     = "test-client-id"
	testClientSecret = "test-client-secret-value" // テスト用の使い捨ての値(本物の秘密ではない)
	testRedirectURL  = "http://localhost:8080/auth/google/callback"
)

// newProvider は、代役の提供元に向けた Provider を返す。
func newProvider(t *testing.T) (*googleauth.Provider, *fakeoidc.Server) {
	t.Helper()
	idp := fakeoidc.New(t, testClientID, testClientSecret)
	idp.SetUser(fakeoidc.User{Sub: "sub-alice", Email: "alice@example.com", EmailVerified: true, Name: "Alice"})
	return googleauth.New(googleauth.Config{ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: testRedirectURL, Issuer: idp.URL}), idp
}

// authorize は、手続きを始めて、代役の認可の画面が自動で承認したときに戻ってくる、認可コードと state を返す。
func authorize(t *testing.T, p *googleauth.Provider) (code string, secrets usecase.GoogleFlowSecrets) {
	t.Helper()
	authURL, secrets, err := p.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(authURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("認可の画面が戻さなかった: status %d, location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if loc.Query().Get("state") != secrets.State {
		t.Fatalf("state が戻ってこない: %q", loc.Query().Get("state"))
	}
	return loc.Query().Get("code"), secrets
}

func TestBeginBuildsAuthorizationRequestWithPKCEAndNonce(t *testing.T) {
	p, idp := newProvider(t)
	authURL, secrets, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(authURL, idp.URL+"/authorize?") {
		t.Fatalf("認可の URL が提供元のものではない: %s", authURL)
	}
	checks := map[string]string{
		"client_id": testClientID, "redirect_uri": testRedirectURL, "response_type": "code",
		"state": secrets.State, "nonce": secrets.Nonce, "code_challenge_method": "S256", "prompt": "select_account",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	for _, scope := range []string{"openid", "email", "profile"} {
		if !strings.Contains(" "+q.Get("scope")+" ", " "+scope+" ") {
			t.Errorf("scope に %s がない: %q", scope, q.Get("scope"))
		}
	}
	if q.Get("code_challenge") == "" || strings.Contains(authURL, secrets.Verifier) {
		t.Error("code_challenge がない、または検証値そのものが URL に載っている")
	}
	if strings.Contains(authURL, testClientSecret) {
		t.Error("クライアントの秘密が URL に載っている")
	}
	if secrets.State == "" || secrets.Nonce == "" || secrets.Verifier == "" || secrets.State == secrets.Nonce {
		t.Errorf("秘密の値が不十分: %+v", secrets)
	}
	_, other, _ := p.Begin(context.Background())
	if other.State == secrets.State || other.Nonce == secrets.Nonce || other.Verifier == secrets.Verifier {
		t.Error("手続きのたびに、違う値が作られていない")
	}
}

func TestCompleteReturnsVerifiedIdentity(t *testing.T) {
	p, _ := newProvider(t)
	code, secrets := authorize(t, p)
	got, err := p.Complete(context.Background(), code, secrets)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := domain.ExternalIdentity{Provider: domain.ProviderGoogle, ProviderUserID: "sub-alice", Email: "alice@example.com", EmailVerified: true, Name: "Alice"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCompleteRejectsInvalidExchangesAndTokens(t *testing.T) {
	cases := []struct {
		name  string
		setup func(idp *fakeoidc.Server)
		// mutate は、コードの交換の前に、手続きの秘密の値やコードを変える。
		mutate func(code *string, secrets *usecase.GoogleFlowSecrets)
	}{
		{name: "宛先(aud)が別のクライアントの ID トークンは、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Aud: "another-client"}) }},
		{name: "発行者(iss)が違う ID トークンは、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Iss: "https://evil.example"}) }},
		{name: "期限が切れた ID トークンは、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{ExpiredAgo: time.Hour}) }},
		{name: "公開鍵と別の鍵で署名された ID トークンは、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{WrongKey: true}) }},
		{name: "ID トークンの nonce が、手続きのものと違えば、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Nonce: "someone-elses-nonce"}) }},
		{name: "手続きの側の nonce が違えば(使い回しの ID トークン)、拒否する", mutate: func(_ *string, s *usecase.GoogleFlowSecrets) { s.Nonce = "different-nonce" }},
		{name: "手続きの側の nonce が空でも、拒否する", mutate: func(_ *string, s *usecase.GoogleFlowSecrets) { s.Nonce = "" }},
		{name: "コードの交換の応答に ID トークンがなければ、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{NoIDToken: true}) }},
		{name: "コードの交換がエラーで失敗すれば、拒否する", setup: func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{TokenStatus: http.StatusInternalServerError}) }},
		{name: "PKCE の検証値が違えば(認可コードを横取りされた場合)、提供元が交換を断り、拒否する", mutate: func(_ *string, s *usecase.GoogleFlowSecrets) { s.Verifier = strings.Repeat("x", 43) }},
		{name: "知らない認可コードは、拒否する", mutate: func(c *string, _ *usecase.GoogleFlowSecrets) { *c = "unknown-code" }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p, idp := newProvider(t)
			if tt.setup != nil {
				tt.setup(idp)
			}
			code, secrets := authorize(t, p)
			if tt.mutate != nil {
				tt.mutate(&code, &secrets)
			}
			id, err := p.Complete(context.Background(), code, secrets)
			if err == nil {
				t.Fatalf("拒否されるはずが、成功した: %+v", id)
			}
			msg := err.Error()
			for _, secret := range []string{testClientSecret, code, secrets.Verifier, "eyJ"} {
				if secret != "" && strings.Contains(msg, secret) {
					t.Errorf("エラーの文言に秘密の値が含まれている: %q", msg)
				}
			}
		})
	}

	t.Run("認可コードは 1 回しか使えない", func(t *testing.T) {
		p, _ := newProvider(t)
		code, secrets := authorize(t, p)
		if _, err := p.Complete(context.Background(), code, secrets); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Complete(context.Background(), code, secrets); err == nil {
			t.Fatal("同じコードを 2 回交換できてしまった")
		}
	})

	t.Run("クライアントの秘密が違えば、提供元が交換を断り、拒否する(エラーに秘密を含めない)", func(t *testing.T) {
		_, idp := newProvider(t)
		bad := googleauth.New(googleauth.Config{ClientID: testClientID, ClientSecret: "wrong-secret-value", RedirectURL: testRedirectURL, Issuer: idp.URL})
		code, secrets := authorize(t, bad)
		_, err := bad.Complete(context.Background(), code, secrets)
		if err == nil {
			t.Fatal("拒否されるはずが、成功した")
		}
		if strings.Contains(err.Error(), "wrong-secret-value") {
			t.Fatalf("エラーの文言に、クライアントの秘密が含まれている: %v", err)
		}
	})
}

func TestCompleteFailsWithoutLeakingSecretsWhenProviderGoesDown(t *testing.T) {
	p, idp := newProvider(t)
	code, secrets := authorize(t, p)
	idp.Close() // コードの交換の前に、提供元が落ちる(探索の情報は、すでに覚えている)
	_, err := p.Complete(context.Background(), code, secrets)
	if err == nil {
		t.Fatal("提供元が落ちているのに、成功した")
	}
	for _, secret := range []string{testClientSecret, code, secrets.Verifier} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("エラーの文言に秘密の値が含まれている: %q", err.Error())
		}
	}
}

func TestBeginFailsWhenProviderIsUnreachable(t *testing.T) {
	p := googleauth.New(googleauth.Config{ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: testRedirectURL, Issuer: "http://127.0.0.1:1"})
	if _, _, err := p.Begin(context.Background()); err == nil {
		t.Fatal("提供元につながらないのに、手続きを始められてしまった")
	}
}

// TestDiscoveryDoesNotSerializeRequests は、提供元の探索の情報を取得する通信が、要求を 1 件ずつ待たせないことを
// 確かめる。探索の通信は、最初に必要になったときに行うので、提供元の窓口が遅い・落ちているとき、同時に来た
// 開始・戻りの要求が、その通信を、順番に待たされて(それぞれが、最大 10 秒)、全体が止まってはならない。
func TestDiscoveryDoesNotSerializeRequests(t *testing.T) {
	t.Run("探索の通信が遅くて、待っている要求が取り消されたら、通信の完了を待たずに、その要求は終わる", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetTweaks(fakeoidc.Tweaks{DiscoveryDelay: 600 * time.Millisecond})
		go func() { _, _, _ = p.Begin(context.Background()) }() // 探索の通信を始める(遅い)
		time.Sleep(100 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, _, err := p.Begin(ctx)

		if err == nil {
			t.Fatal("取り消された要求が、通信の完了まで待って、成功した")
		}
		if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
			t.Fatalf("取り消された要求が、%v も待たされた(通信の完了を待っている)", elapsed)
		}
	})

	t.Run("提供元の窓口が落ちているとき、同時に来た要求は、探索の通信を 1 回だけ行い、まとめて失敗する(順番に何度も行わない)", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetTweaks(fakeoidc.Tweaks{DiscoveryDelay: 200 * time.Millisecond, DiscoveryStatus: http.StatusInternalServerError})
		const n = 5
		errs := make([]error, n)
		var wg sync.WaitGroup
		start := time.Now()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, errs[i] = p.Begin(context.Background())
			}()
		}
		wg.Wait()

		for i, err := range errs {
			if err == nil {
				t.Fatalf("%d 件目が成功した", i)
			}
		}
		if hits := idp.DiscoveryRequests(); hits != 1 {
			t.Fatalf("探索の通信 = %d 回, want 1 回(同時の要求は、1 回の通信を共有する)", hits)
		}
		if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
			t.Fatalf("全体で %v かかった(順番に待たされている)", elapsed)
		}
	})

	t.Run("最初の要求が取り消されても、探索の通信を待っているほかの要求は、道連れで失敗せず、結果を受け取る", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetTweaks(fakeoidc.Tweaks{DiscoveryDelay: 300 * time.Millisecond})
		firstCtx, cancelFirst := context.WithCancel(context.Background())
		go func() { _, _, _ = p.Begin(firstCtx) }() // 探索の通信を始める(最初の要求)
		time.Sleep(60 * time.Millisecond)

		done := make(chan error, 1)
		go func() {
			_, _, err := p.Begin(context.Background()) // 待つ要求
			done <- err
		}()
		time.Sleep(60 * time.Millisecond)
		cancelFirst() // 最初の要求が取り消される(利用者が画面を閉じた、など)

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("待っていた要求が、最初の要求の取り消しで失敗した: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("待っていた要求が終わらない")
		}
		if hits := idp.DiscoveryRequests(); hits != 1 {
			t.Fatalf("探索の通信 = %d 回, want 1 回", hits)
		}
	})

	t.Run("成功した探索の情報は覚えて、以後の要求は、通信しない", func(t *testing.T) {
		p, idp := newProvider(t)
		for i := 0; i < 4; i++ {
			if _, _, err := p.Begin(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if hits := idp.DiscoveryRequests(); hits != 1 {
			t.Fatalf("探索の通信 = %d 回, want 1 回", hits)
		}
	})

	t.Run("失敗したあと、RetryInterval の間は、通信せずに失敗し(窓口が回復していても)、過ぎたら、再び通信して、回復していれば成功する", func(t *testing.T) {
		idp := fakeoidc.New(t, testClientID, testClientSecret)
		p := googleauth.New(googleauth.Config{
			ClientID: testClientID, ClientSecret: testClientSecret, RedirectURL: testRedirectURL, Issuer: idp.URL,
			DiscoveryRetryInterval: 400 * time.Millisecond,
		})
		idp.SetTweaks(fakeoidc.Tweaks{DiscoveryStatus: http.StatusInternalServerError})
		if _, _, err := p.Begin(context.Background()); err == nil {
			t.Fatal("窓口が落ちているのに成功した")
		}
		idp.SetTweaks(fakeoidc.Tweaks{}) // 窓口が回復した

		if _, _, err := p.Begin(context.Background()); err == nil {
			t.Fatal("失敗を覚えている間に、成功した(通信している)")
		}
		if hits := idp.DiscoveryRequests(); hits != 1 {
			t.Fatalf("失敗を覚えている間の通信 = %d 回, want 1 回(通信しない)", hits)
		}

		time.Sleep(450 * time.Millisecond)
		if _, _, err := p.Begin(context.Background()); err != nil {
			t.Fatalf("時間が過ぎたあと、回復した窓口に対して失敗した: %v", err)
		}
		if hits := idp.DiscoveryRequests(); hits != 2 {
			t.Fatalf("探索の通信 = %d 回, want 2 回(失敗 1 回・回復後 1 回)", hits)
		}
	})
}
