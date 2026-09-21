package handler_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// TestMe は GET /me を扱う：有効なトークンは現在のユーザー(can_moderate つき)を 200 で返し、
// 無効・期限切れ・欠落のトークンは 401 になる(frontend は、トークンの有効性を自分で判断せず、
// この応答でログイン状態を復元する)。
func TestMe(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "Password123!")
	root := repo.seed("root", "root@example.com", "Password123!")
	repo.users[root.ID].user.Admin = true
	aliceToken, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	rootToken, err := codec.Issue(root.ID)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	expiredToken, err := infra.NewJWTCodec(testJWTSecret, -time.Minute).Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	router := newTestRouterWith(t, okPinger, auth)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantBody   string
	}{
		{"一般のユーザーは can_moderate が false", "Bearer " + aliceToken, http.StatusOK,
			`{"id":"` + alice.ID + `","username":"alice","email":"alice@example.com","admin":false,"can_moderate":false}`},
		{"管理者は can_moderate が true", "Bearer " + rootToken, http.StatusOK,
			`{"id":"` + root.ID + `","username":"root","email":"root@example.com","admin":true,"can_moderate":true}`},
		{"トークンなしは 401", "", http.StatusUnauthorized, `{"error":"Unauthorized"}`},
		{"期限切れのトークンは 401", "Bearer " + expiredToken, http.StatusUnauthorized, `{"error":"Unauthorized"}`},
		{"改ざんされたトークンは 401", "Bearer " + aliceToken + "x", http.StatusUnauthorized, `{"error":"Unauthorized"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/me", "", tt.authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestMeta は GET /meta を扱う：認証なしで、domain の rating の範囲と、写真の上限(長辺・バイト数)を返し、
// キャッシュしてよいことを示す(frontend はこれらの値を複製しない)。
func TestMeta(t *testing.T) {
	_, auth, _ := newAuthKit()
	rec := do(newTestRouterWith(t, okPinger, auth), http.MethodGet, "/meta", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	want := `{"rating":{"min":` + strconv.Itoa(domain.MinRating) + `,"max":` + strconv.Itoa(domain.MaxRating) + `},` +
		`"photo":{"max_edge":` + strconv.Itoa(photo.MaxEdge) + `,"max_bytes":5242880}}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") || !strings.Contains(got, "public") {
		t.Errorf("Cache-Control = %q, want a public max-age", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// TestAuthResponsesCarryCanModerate は、login と signup の確認の応答も、GET /me と同じ
// can_moderate を返すことを固定する(frontend は admin から権限を導かない)。
func TestAuthResponsesCarryCanModerate(t *testing.T) {
	repo, auth, _ := newAuthKit()
	repo.seed("alice", "alice@example.com", "Password123!")
	root := repo.seed("root", "root@example.com", "Password123!")
	repo.users[root.ID].user.Admin = true
	router := newTestRouterWith(t, okPinger, auth)

	for _, tt := range []struct {
		name, email string
		want        bool
	}{{"一般のユーザーの login", "alice@example.com", false}, {"管理者の login", "root@example.com", true}} {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, "/login", `{"email":"`+tt.email+`","password":"Password123!"}`, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			want := `"can_moderate":` + strconv.FormatBool(tt.want)
			if !strings.Contains(rec.Body.String(), want) {
				t.Errorf("body = %s, want it to contain %s", rec.Body, want)
			}
		})
	}
	// signup の確認(POST /signup/confirm)の応答も、GET /me と同じ can_moderate を返す。
	// 確認で得たトークンで GET /me を呼ぶと、同じユーザー・同じ can_moderate になる(確認 → ログイン状態の復元)。
	t.Run("signup の確認", func(t *testing.T) {
		kit := newSignupKit(t)
		rec := do(kit.router, http.MethodPost, "/signup",
			`{"username":"carol","email":"carol@example.com","password":"Password123!","password_confirmation":"Password123!"}`, "")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("signup status = %d, want 202 (body %s)", rec.Code, rec.Body)
		}
		rec = do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(kit.mailer.lastToken(t)), "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("confirm status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"can_moderate":false`) {
			t.Errorf("confirm body = %s, want can_moderate false", rec.Body)
		}
		token := decodeAuthUser(t, rec.Body.Bytes()).Token
		me := do(kit.router, http.MethodGet, "/me", "", "Bearer "+token)
		if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"username":"carol"`) || !strings.Contains(me.Body.String(), `"can_moderate":false`) {
			t.Errorf("GET /me = %d %s, want 200 の carol(can_moderate false)", me.Code, me.Body)
		}
	})
}
