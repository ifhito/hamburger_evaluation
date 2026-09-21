package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// textUnits は、上限が文字数（コードポイント数）で数えられることを確かめる文字の種類である。
// 日本語は 3 バイト、絵文字は 4 バイトなので、バイト数で数えていれば上限ちょうどの値が弾かれる。
var textUnits = []struct{ name, unit string }{
	{"ASCII", "a"},
	{"日本語", "あ"},
	{"絵文字", "🍔"},
}

func tooLong(label string, max int) string {
	return fmt.Sprintf(`{"errors":["%s is too long (maximum is %d characters)"]}`, label, max)
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal %q: %v", s, err)
	}
	return string(b)
}

// TestReviewTextLimits は S21 の AC1・AC3・AC5 を、review の投稿と編集で、JSON と multipart の
// 両方の経路について固定する。コメントは上限ちょうどなら 201/200、1 文字超えると 422 で、
// 422 のときは review も burger も増えない（永続化の前に検証される）。
func TestReviewTextLimits(t *testing.T) {
	over := tooLong("Comment", domain.MaxCommentChars)
	for _, u := range textUnits {
		exact := strings.Repeat(u.unit, domain.MaxCommentChars)
		tooBig := strings.Repeat(u.unit, domain.MaxCommentChars+1)

		t.Run("JSON の投稿: 上限ちょうどの"+u.name+"は 201、超えると 422 で何も作られない", func(t *testing.T) {
			repo := seedReviewWorld(uid.N(1))
			router, aliceAuth, _, _ := newReviewsRouter(t, repo)
			body := func(comment string) string {
				return fmt.Sprintf(`{"review":{"rating":4,"comment":%s,"shop_id":%q,"burger_id":%q}}`, jsonString(t, comment), activeShopID, cheeseBurgerID)
			}
			if rec := do(router, http.MethodPost, "/reviews", body(exact), aliceAuth); rec.Code != http.StatusCreated {
				t.Fatalf("上限ちょうど: status = %d, want 201 (body %.200s)", rec.Code, rec.Body)
			}
			before := len(repo.reviews)
			rec := do(router, http.MethodPost, "/reviews", body(tooBig), aliceAuth)
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != over {
				t.Errorf("超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, over)
			}
			if len(repo.reviews) != before {
				t.Errorf("422 なのに review が増えた: %d → %d", before, len(repo.reviews))
			}
		})

		t.Run("multipart の投稿: 上限ちょうどの"+u.name+"は 201、超えると 422 で何も作られない", func(t *testing.T) {
			repo := seedReviewWorld(uid.N(1))
			router, aliceAuth, _, _ := newReviewsRouter(t, repo)
			post := func(comment string) (code int, body string) {
				form, ct := multipartBody(t, map[string]string{
					"rating": "4", "comment": comment,
					"shop_id": activeShopID, "burger_id": cheeseBurgerID,
				})
				rec := doMultipart(router, http.MethodPost, "/reviews", form, ct, aliceAuth)
				return rec.Code, rec.Body.String()
			}
			if code, body := post(exact); code != http.StatusCreated {
				t.Fatalf("上限ちょうど: status = %d, want 201 (body %.200s)", code, body)
			}
			before := len(repo.reviews)
			if code, body := post(tooBig); code != http.StatusUnprocessableEntity || body != over {
				t.Errorf("超過: status/body = %d %s, want 422 %s", code, body, over)
			}
			if len(repo.reviews) != before {
				t.Errorf("422 なのに review が増えた: %d → %d", before, len(repo.reviews))
			}
		})

		t.Run("PUT: 上限ちょうどの"+u.name+"は 200、超えると 422 で内容が変わらない", func(t *testing.T) {
			router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(uid.N(1)))
			created := do(router, http.MethodPost, "/reviews",
				fmt.Sprintf(`{"review":{"rating":4,"comment":"first","shop_id":%q,"burger_id":%q}}`, activeShopID, cheeseBurgerID), aliceAuth)
			id, _ := decodePhotoURL(t, created.Body.Bytes())
			path := fmt.Sprintf("/reviews/%s", id)
			edit := func(comment string) (int, string) {
				rec := do(router, http.MethodPut, path, fmt.Sprintf(`{"review":{"rating":5,"comment":%s}}`, jsonString(t, comment)), aliceAuth)
				return rec.Code, rec.Body.String()
			}
			if code, body := edit(exact); code != http.StatusOK {
				t.Fatalf("上限ちょうど: status = %d, want 200 (body %.200s)", code, body)
			}
			if code, body := edit(tooBig); code != http.StatusUnprocessableEntity || body != over {
				t.Errorf("超過: status/body = %d %s, want 422 %s", code, body, over)
			}
			form, ct := multipartBody(t, map[string]string{"rating": "5", "comment": tooBig})
			if rec := doMultipart(router, http.MethodPut, path, form, ct, aliceAuth); rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != over {
				t.Errorf("multipart の超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, over)
			}
			detail := do(router, http.MethodGet, path, "", "")
			var got struct {
				Comment string `json:"comment"`
			}
			if err := json.Unmarshal(detail.Body.Bytes(), &got); err != nil || got.Comment != exact {
				t.Errorf("422 のあと、コメントが上限ちょうどの値のまま変わらないはず: err=%v len=%d", err, len([]rune(got.Comment)))
			}
		})
	}

	t.Run("burger_name の経路: 上限ちょうどは 201、超えると 422 で burger も review も作られない", func(t *testing.T) {
		repo := seedReviewWorld(uid.N(1))
		router, aliceAuth, _, _ := newReviewsRouter(t, repo)
		body := func(name string) string {
			return fmt.Sprintf(`{"review":{"rating":4,"comment":"ok","shop_id":%q,"burger_name":%s}}`, activeShopID, jsonString(t, name))
		}
		if rec := do(router, http.MethodPost, "/reviews", body(strings.Repeat("あ", domain.MaxBurgerNameChars)), aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("上限ちょうど: status = %d, want 201 (body %.200s)", rec.Code, rec.Body)
		}
		burgers, reviews := len(repo.burgers), len(repo.reviews)
		rec := do(router, http.MethodPost, "/reviews", body(strings.Repeat("あ", domain.MaxBurgerNameChars+1)), aliceAuth)
		if want := tooLong("Burger name", domain.MaxBurgerNameChars); rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
			t.Errorf("超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, want)
		}
		if len(repo.burgers) != burgers || len(repo.reviews) != reviews {
			t.Errorf("422 なのに burger/review が増えた: burgers %d→%d, reviews %d→%d", burgers, len(repo.burgers), reviews, len(repo.reviews))
		}
	})

	t.Run("複数の違反は 1 つの応答に列挙され、rating の違反が先に並ぶ", func(t *testing.T) {
		router, aliceAuth, _, _ := newReviewsRouter(t, seedReviewWorld(uid.N(1)))
		body := fmt.Sprintf(`{"review":{"rating":9,"comment":%s,"shop_id":%q,"burger_id":%q}}`,
			jsonString(t, strings.Repeat("a", domain.MaxCommentChars+1)), activeShopID, cheeseBurgerID)
		rec := do(router, http.MethodPost, "/reviews", body, aliceAuth)
		want := `{"errors":["Rating must be in 1..5","Comment is too long (maximum is 2000 characters)"]}`
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != want {
			t.Errorf("status/body = %d %s, want 422 %s", rec.Code, rec.Body, want)
		}
	})
}

// TestShopTextLimits は S21 の AC2・AC5 を shop で固定する：投稿と管理者の名称変更は名前 100 文字、
// 却下は moderation note 500 文字まで。超えると 422 で、shop は増えず、変更もされない。
func TestShopTextLimits(t *testing.T) {
	nameOver := tooLong("Name", domain.MaxShopNameChars)
	noteOver := tooLong("Moderation note", domain.MaxModerationNoteChars)

	t.Run("POST /shops: 名前は上限ちょうどなら 201、超えると 422 で shop が増えない", func(t *testing.T) {
		repo := seedShops(uid.N(1))
		router, aliceAuth, _, _ := newShopsRouter(t, repo)
		body := func(name string) string { return `{"shop":{"name":` + jsonString(t, name) + `}}` }
		if rec := do(router, http.MethodPost, "/shops", body(strings.Repeat("あ", domain.MaxShopNameChars)), aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("上限ちょうど: status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		before := len(repo.shops)
		rec := do(router, http.MethodPost, "/shops", body(strings.Repeat("あ", domain.MaxShopNameChars+1)), aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != nameOver {
			t.Errorf("超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, nameOver)
		}
		if len(repo.shops) != before {
			t.Errorf("422 なのに shop が増えた: %d → %d", before, len(repo.shops))
		}
	})

	t.Run("管理者の名称変更: 上限ちょうどは 200、超えると 422 で名前が変わらない", func(t *testing.T) {
		repo := seedShops(uid.N(1))
		router, _, adminAuth, _ := newShopsRouter(t, repo)
		put := func(name string) (int, string) {
			rec := do(router, http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":`+jsonString(t, name)+`}}`, adminAuth)
			return rec.Code, rec.Body.String()
		}
		exact := strings.Repeat("🍔", domain.MaxShopNameChars)
		if code, body := put(exact); code != http.StatusOK {
			t.Fatalf("上限ちょうど: status = %d, want 200 (body %s)", code, body)
		}
		if code, body := put(strings.Repeat("🍔", domain.MaxShopNameChars+1)); code != http.StatusUnprocessableEntity || body != nameOver {
			t.Errorf("超過: status/body = %d %s, want 422 %s", code, body, nameOver)
		}
		if got := repo.shops[1].Name; got != exact {
			t.Errorf("422 のあと、名前が上限ちょうどの値のまま変わらないはず: %d 文字", len([]rune(got)))
		}
	})

	t.Run("却下: note は上限ちょうどなら 200、超えると 422 で status も note も変わらない", func(t *testing.T) {
		repo := seedShops(uid.N(1))
		router, _, adminAuth, _ := newShopsRouter(t, repo)
		reject := func(note string) (int, string) {
			rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(2)+"/reject", `{"moderation_note":`+jsonString(t, note)+`}`, adminAuth)
			return rec.Code, rec.Body.String()
		}
		if code, body := reject(strings.Repeat("あ", domain.MaxModerationNoteChars+1)); code != http.StatusUnprocessableEntity || body != noteOver {
			t.Errorf("超過: status/body = %d %s, want 422 %s", code, body, noteOver)
		}
		if got := repo.shops[1]; got.Status != domain.ShopStatusPending || got.ModerationNote != nil {
			t.Errorf("422 なのに shop が変わった: status=%s note=%v", got.Status, got.ModerationNote)
		}
		exact := strings.Repeat("あ", domain.MaxModerationNoteChars)
		if code, body := reject(exact); code != http.StatusOK {
			t.Fatalf("上限ちょうど: status = %d, want 200 (body %.200s)", code, body)
		}
		if got := repo.shops[1]; got.Status != domain.ShopStatusRejected || got.ModerationNote == nil || *got.ModerationNote != exact {
			t.Errorf("却下が反映されていない: %+v", got)
		}
	})

	t.Run("却下: 一般ユーザーは、note が長くても先に 403 になる", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
		rec := do(router, http.MethodPost, "/admin/shops/"+uid.N(2)+"/reject",
			`{"moderation_note":`+jsonString(t, strings.Repeat("a", domain.MaxModerationNoteChars+1))+`}`, aliceAuth)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403 (body %s)", rec.Code, rec.Body)
		}
	})
}

// TestUserTextLimits は S21 の AC2・AC5 を、サインアップと PUT /users/{id} で固定する：
// ユーザー名は 50 文字、メールは 254 文字まで。超えると 422 で、ユーザーは作られず、変更もされない。
func TestUserTextLimits(t *testing.T) {
	longEmail := func(n int) string { return strings.Repeat("a", n-len("@example.com")) + "@example.com" }
	usernameOver := tooLong("Username", domain.MaxUsernameChars)
	emailOver := tooLong("Email", domain.MaxEmailChars)

	t.Run("サインアップ: 上限ちょうどは 202 で確認メールが頼まれ、確認するとユーザーができる。超えると 422 で確認待ちもメールもできない", func(t *testing.T) {
		kit := newSignupKit(t)
		signup := func(username, email string) (int, string) {
			body := fmt.Sprintf(`{"username":%s,"email":%s,"password":"Password123!"}`, jsonString(t, username), jsonString(t, email))
			rec := do(kit.router, http.MethodPost, "/signup", body, "")
			return rec.Code, rec.Body.String()
		}
		exactName, exactEmail := strings.Repeat("あ", domain.MaxUsernameChars), longEmail(domain.MaxEmailChars)
		if code, body := signup(exactName, exactEmail); code != http.StatusAccepted || body != signupAcceptedBody {
			t.Fatalf("上限ちょうど: status = %d, want 202 (body %.200s)", code, body)
		}
		if rec := do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(kit.mailer.lastToken(t)), ""); rec.Code != http.StatusCreated {
			t.Fatalf("上限ちょうどの確認: status = %d, want 201 (body %.200s)", rec.Code, rec.Body)
		}
		if len(kit.users.users) != 1 {
			t.Fatalf("確認後のユーザー = %d 人, want 1", len(kit.users.users))
		}
		pendingBefore := len(kit.store.rows)
		confirmationsBefore, _ := kit.mailer.counts()
		if code, body := signup(strings.Repeat("あ", domain.MaxUsernameChars+1), "new1@example.com"); code != http.StatusUnprocessableEntity || body != usernameOver {
			t.Errorf("ユーザー名の超過: status/body = %d %s, want 422 %s", code, body, usernameOver)
		}
		if code, body := signup("new2", longEmail(domain.MaxEmailChars+1)); code != http.StatusUnprocessableEntity || body != emailOver {
			t.Errorf("メールの超過: status/body = %d %s, want 422 %s", code, body, emailOver)
		}
		wantBoth := `{"errors":["Username is too long (maximum is 50 characters)","Email is too long (maximum is 254 characters)"]}`
		if code, body := signup(strings.Repeat("a", domain.MaxUsernameChars+1), longEmail(domain.MaxEmailChars+1)); code != http.StatusUnprocessableEntity || body != wantBoth {
			t.Errorf("両方の超過: status/body = %d %s, want 422 %s", code, body, wantBoth)
		}
		if confirmationsAfter, _ := kit.mailer.counts(); len(kit.store.rows) != pendingBefore || confirmationsAfter != confirmationsBefore {
			t.Errorf("422 なのに確認待ち・確認メールが増えた: 確認待ち %d → %d、確認メール %d → %d", pendingBefore, len(kit.store.rows), confirmationsBefore, confirmationsAfter)
		}
	})

	t.Run("PUT /users: 上限ちょうどは 200、超えると 422 で変更されない。送らない項目は検証されない", func(t *testing.T) {
		repo, router, token := newSeededUsersRouter(t)
		put := func(body string) (int, string) {
			rec := do(router, http.MethodPut, "/users/"+uid.N(1), body, token(uid.N(1)))
			return rec.Code, rec.Body.String()
		}
		if code, body := put(`{"user":{"username":` + jsonString(t, strings.Repeat("🍔", domain.MaxUsernameChars)) + `,"email":` + jsonString(t, longEmail(domain.MaxEmailChars)) + `}}`); code != http.StatusOK {
			t.Fatalf("上限ちょうど: status = %d, want 200 (body %.200s)", code, body)
		}
		before := repo.users[uid.N(1)].user
		if code, body := put(`{"user":{"username":` + jsonString(t, strings.Repeat("a", domain.MaxUsernameChars+1)) + `}}`); code != http.StatusUnprocessableEntity || body != usernameOver {
			t.Errorf("ユーザー名の超過: status/body = %d %s, want 422 %s", code, body, usernameOver)
		}
		if code, body := put(`{"user":{"email":` + jsonString(t, longEmail(domain.MaxEmailChars+1)) + `}}`); code != http.StatusUnprocessableEntity || body != emailOver {
			t.Errorf("メールの超過: status/body = %d %s, want 422 %s", code, body, emailOver)
		}
		if repo.users[uid.N(1)].user != before {
			t.Errorf("422 なのにユーザーが変わった: %+v → %+v", before, repo.users[uid.N(1)].user)
		}
		if code, body := put(`{"user":{"username":"short"}}`); code != http.StatusOK {
			t.Errorf("送らない email は検証されず、更新できるはず: status = %d (body %s)", code, body)
		}
	})
}

// TestTextLimitsIntegration は、本物の PostgreSQL・repository・router を通して、
// 上限が Go のコードポイント数と PostgreSQL の char_length で同じに数えられること（絵文字 2,000 個の
// コメントが保存できる）と、422 のとき DB に何も書かれないことを固定する（S21 AC3・AC5）。
func TestTextLimitsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)
	_, token := signupUser(t, router, "limits", "limits@example.com", "Password123!")

	var shopID, burgerID string
	if err := conn.QueryRow(ctx, "INSERT INTO shops (name, status) VALUES ('Limit Diner', 1) RETURNING id").Scan(&shopID); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	if err := conn.QueryRow(ctx, "INSERT INTO burgers (name) VALUES ('Limit Burger') RETURNING id").Scan(&burgerID); err != nil {
		t.Fatalf("insert burger: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)", shopID, burgerID); err != nil {
		t.Fatalf("link burger: %v", err)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	postReview := func(comment string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":%s,"shop_id":%q,"burger_id":%q}}`, jsonString(t, comment), shopID, burgerID)
		return do(router, http.MethodPost, "/reviews", body, token)
	}

	exact := strings.Repeat("🍔", domain.MaxCommentChars)
	rec := postReview(exact)
	if rec.Code != http.StatusCreated {
		t.Fatalf("絵文字 %d 個のコメント: status = %d, want 201 (body %.200s)", domain.MaxCommentChars, rec.Code, rec.Body)
	}
	id, _ := decodePhotoURL(t, rec.Body.Bytes())
	var stored int
	if err := conn.QueryRow(ctx, "SELECT char_length(comment) FROM reviews WHERE id = $1", id).Scan(&stored); err != nil || stored != domain.MaxCommentChars {
		t.Errorf("保存された comment の char_length = %d (err %v), want %d", stored, err, domain.MaxCommentChars)
	}

	reviews, users := count("reviews"), count("users")
	if rec := postReview(exact + "🍔"); rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != tooLong("Comment", domain.MaxCommentChars) {
		t.Errorf("超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, tooLong("Comment", domain.MaxCommentChars))
	}
	signup := do(router, http.MethodPost, "/signup",
		fmt.Sprintf(`{"username":%s,"email":"over@example.com","password":"Password123!"}`, jsonString(t, strings.Repeat("a", domain.MaxUsernameChars+1))), "")
	if signup.Code != http.StatusUnprocessableEntity {
		t.Errorf("ユーザー名の超過: status = %d, want 422 (body %s)", signup.Code, signup.Body)
	}
	if got := count("reviews"); got != reviews {
		t.Errorf("422 なのに reviews が増えた: %d → %d", reviews, got)
	}
	if got := count("users"); got != users {
		t.Errorf("422 なのに users が増えた: %d → %d", users, got)
	}
}
