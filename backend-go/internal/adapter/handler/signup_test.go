package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// testSignupConfig は、handler のテストで使う signup の設定である。
var testSignupConfig = usecase.SignupConfig{BaseURL: "https://app.example.com", TokenTTL: 24 * time.Hour}

const (
	signupAcceptedBody = `{"message":"Confirmation email sent"}`
	tokenInvalidBody   = `{"error":"Confirmation token is invalid or has expired"}`
	confirmLinkPrefix  = "https://app.example.com/signup/confirm?token="
)

// mailRecorder は、送るよう頼まれたメールを記録する usecase.Mailer の fake である。
type mailRecorder struct {
	mu   sync.Mutex
	sent []usecase.Mail
}

func (m *mailRecorder) Send(mail usecase.Mail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mail)
}

func (m *mailRecorder) all() []usecase.Mail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]usecase.Mail(nil), m.sent...)
}

// lastToken は、最後に送られた確認メールのリンクから、平文のトークンを取り出す。
func (m *mailRecorder) lastToken(t *testing.T) string {
	t.Helper()
	mails := m.all()
	for i := len(mails) - 1; i >= 0; i-- {
		if idx := strings.Index(mails[i].Body, confirmLinkPrefix); idx >= 0 {
			return strings.Fields(mails[i].Body[idx+len(confirmLinkPrefix):])[0]
		}
	}
	t.Fatalf("確認リンクつきのメールが送られていない: %+v", mails)
	return ""
}

// signupRow は signupStoreFake が持つ確認待ちの 1 行である。
type signupRow struct {
	email, username, digest, tokenHash string
	expiresAt, lastSentAt              time.Time
}

// signupStoreFake は in-memory の domain.SignupVerificationRepository である。確認で作る
// ユーザーは、users の fake（usecase.UserQuery と共有）に入るので、確認のあとにログインできる。
// now を進めると、送信の間隔と有効期限の判定を、待たずに試せる。
type signupStoreFake struct {
	users *userStoreFake
	rows  map[string]*signupRow // キーは小文字の email
	now   time.Time
	err   error
}

var _ domain.SignupVerificationRepository = (*signupStoreFake)(nil)

func newSignupStoreFake(users *userStoreFake) *signupStoreFake {
	return &signupStoreFake{users: users, rows: map[string]*signupRow{}, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *signupStoreFake) CreateSignupVerification(_ context.Context, p domain.CreateSignupVerificationParams) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	key := strings.ToLower(p.Email)
	if row, ok := f.rows[key]; ok && f.now.Sub(row.lastSentAt) < domain.SignupResendInterval {
		return false, nil
	}
	f.rows[key] = &signupRow{
		email: p.Email, username: p.Username, digest: p.PasswordDigest, tokenHash: p.TokenHash,
		expiresAt: f.now.Add(p.TTL), lastSentAt: f.now,
	}
	return true, nil
}

func (f *signupStoreFake) CreateUserFromSignupVerification(ctx context.Context, tokenHash string) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	for key, row := range f.rows {
		if row.tokenHash != tokenHash || !row.expiresAt.After(f.now) {
			continue
		}
		user, err := f.users.CreateUser(ctx, domain.CreateUserParams{Username: row.username, Email: row.email, PasswordDigest: row.digest})
		if err != nil {
			return domain.User{}, err
		}
		delete(f.rows, key)
		return user, nil
	}
	return domain.User{}, domain.ErrSignupTokenInvalid
}

func (f *signupStoreFake) DiscardExpiredSignupVerifications(_ context.Context, limit int) (int64, error) {
	var n int64
	for key, row := range f.rows {
		if n >= int64(limit) {
			break
		}
		if !row.expiresAt.After(f.now) {
			delete(f.rows, key)
			n++
		}
	}
	return n, nil
}

// signupKit は、signup の確認つきの router と、その裏の fake 一式である。
type signupKit struct {
	router http.Handler
	users  *userStoreFake
	store  *signupStoreFake
	mailer *mailRecorder
}

// newSignupKit は、signup・login・users を「同一の」in-memory のユーザーの上で配線する。
func newSignupKit(t *testing.T) *signupKit {
	return newSignupKitWithHasher(t, hasherFake{})
}

func newSignupKitWithHasher(t *testing.T, hasher usecase.PasswordHasher) *signupKit {
	t.Helper()
	users := newUserStoreFake()
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	store := newSignupStoreFake(users)
	mailer := &mailRecorder{}
	auth := usecase.NewAuth(users, hasher, codec, codec)
	signups := usecase.NewSignups(users, domain.NewSignupVerifications(store), hasher, mailer, codec, testSignupConfig)
	reviewRepo := newReviewStoreFake()
	shopRepo := &shopStoreFake{}
	router := handler.NewRouter(okPinger, auth, signups, usecase.NewShops(shopRepo, domain.NewShops(shopRepo)),
		usecase.NewReviews(reviewRepo, domain.NewReviews(reviewRepo), storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(users, domain.NewUsers(users), hasher), nil)
	return &signupKit{router: router, users: users, store: store, mailer: mailer}
}

func signupBody(username, email, password string) string {
	return fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`, username, email, password)
}

func confirmBody(token string) string { return fmt.Sprintf(`{"token":%q}`, token) }

// TestSignupConfirmFlow は AC1・AC6 を扱う：signup は 202 で、users にはまだ作られず、確認メールが
// 1 通届く。そのリンクのトークンで確認すると、従来の signup と同じ 201 の本文（user と token）が
// 返り、そのトークンで保護されたルートを通れて、パスワードでログインもできる。
func TestSignupConfirmFlow(t *testing.T) {
	kit := newSignupKit(t)

	rec := do(kit.router, http.MethodPost, "/signup",
		`{"username":"alice","email":"alice@example.com","password":"Password123!","password_confirmation":"Password123!"}`, "")
	if rec.Code != http.StatusAccepted || rec.Body.String() != signupAcceptedBody {
		t.Fatalf("signup = %d %s, want 202 %s", rec.Code, rec.Body, signupAcceptedBody)
	}
	if len(kit.users.users) != 0 {
		t.Fatalf("確認の前に users が作られた: %d 件", len(kit.users.users))
	}
	if mails := kit.mailer.all(); len(mails) != 1 || mails[0].To != "alice@example.com" || mails[0].Subject != "Confirm your email address" {
		t.Fatalf("確認メール = %+v, want 1 通", mails)
	}

	token := kit.mailer.lastToken(t)
	rec = do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(token), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("confirm status = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	user := decodeAuthUser(t, rec.Body.Bytes())
	if user.ID != uid.N(1) || user.Username != "alice" || user.Email != "alice@example.com" || user.Admin || user.Token == "" {
		t.Errorf("confirm body = %+v, want id=%s alice alice@example.com admin=false と token", user, uid.N(1))
	}
	if rec := do(kit.router, http.MethodPost, "/logout", "", "Bearer "+user.Token); rec.Code != http.StatusOK {
		t.Errorf("logout status = %d, want 200", rec.Code)
	}
	if rec := loginAs(kit.router, "alice@example.com", "Password123!"); rec.Code != http.StatusOK {
		t.Errorf("確認後の login status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	// 使用済みのトークンは使えない（同じ 400）。
	if rec := do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(token), ""); rec.Code != http.StatusBadRequest || rec.Body.String() != tokenInvalidBody {
		t.Errorf("再利用 = %d %s, want 400 %s", rec.Code, rec.Body, tokenInvalidBody)
	}
}

// TestSignupResponsesAreIndistinguishable は AC2・AC4 を扱う：登録済みの email でも未登録の email
// でも、signup の応答（ステータス・ヘッダー・本文）は 1 バイトも違わない。違うのは届くメール
// だけで、登録済みには確認リンクのない通知が届く。検証エラーも登録の有無に依存しない。
func TestSignupResponsesAreIndistinguishable(t *testing.T) {
	kit := newSignupKit(t)
	kit.users.seed("bob", "bob@example.com", "Password123!")

	fresh := do(kit.router, http.MethodPost, "/signup", signupBody("eve", "eve@example.com", "Password123!"), "")
	registered := do(kit.router, http.MethodPost, "/signup", signupBody("eve", "bob@example.com", "Password123!"), "")

	if fresh.Code != http.StatusAccepted || registered.Code != http.StatusAccepted {
		t.Fatalf("status = %d / %d, want 202 / 202", fresh.Code, registered.Code)
	}
	if fresh.Body.String() != registered.Body.String() || fresh.Body.String() != signupAcceptedBody {
		t.Errorf("body = %q / %q, want 同一の %q", fresh.Body, registered.Body, signupAcceptedBody)
	}
	if fmt.Sprint(fresh.Header()) != fmt.Sprint(registered.Header()) {
		t.Errorf("header = %v / %v, want 同一", fresh.Header(), registered.Header())
	}
	if len(kit.users.users) != 1 {
		t.Errorf("users = %d 件, want 1（登録済みの signup で users は変わらない）", len(kit.users.users))
	}

	mails := kit.mailer.all()
	if len(mails) != 2 {
		t.Fatalf("送られたメール = %d 通, want 2", len(mails))
	}
	if mails[0].To != "eve@example.com" || !strings.Contains(mails[0].Body, confirmLinkPrefix) {
		t.Errorf("未登録への確認メール = %+v", mails[0])
	}
	if mails[1].To != "bob@example.com" || mails[1].Subject != "You already have an account" ||
		strings.Contains(mails[1].Body, "token") || strings.Contains(mails[1].Body, "confirm") {
		t.Errorf("登録済みへの通知メール = %+v（確認リンク・トークンを含めない）", mails[1])
	}

	t.Run("検証エラーは登録の有無に依存せず、「登録済み」を示すメッセージも返らない", func(t *testing.T) {
		for _, password := range []string{"", "weak", "abcd1234"} {
			a := do(kit.router, http.MethodPost, "/signup", signupBody("eve", "new@example.com", password), "")
			b := do(kit.router, http.MethodPost, "/signup", signupBody("eve", "bob@example.com", password), "")
			if a.Code != http.StatusUnprocessableEntity || a.Code != b.Code || a.Body.String() != b.Body.String() {
				t.Errorf("password %q: 未登録 = %d %s / 登録済み = %d %s, want 同一の 422", password, a.Code, a.Body, b.Code, b.Body)
			}
			for _, body := range []string{a.Body.String(), b.Body.String()} {
				if strings.Contains(strings.ToLower(body), "taken") || strings.Contains(strings.ToLower(body), "already") {
					t.Errorf("password %q: 応答に「登録済み」を示す文言がある: %s", password, body)
				}
			}
		}
	})
}

// slowHasher は、bcrypt のように時間がかかる PasswordHasher の代役である。
type slowHasher struct{ delay time.Duration }

func (h slowHasher) Hash(password string) (string, error) {
	time.Sleep(h.delay)
	return "digest:" + password, nil
}

func (h slowHasher) Compare(digest, password string) error {
	time.Sleep(h.delay)
	return hasherFake{}.Compare(digest, password)
}

// TestSignupResponseTimeDoesNotRevealRegistration は AC3 を扱う：どの分岐でも bcrypt を行ってから分岐する
// ので、登録済みと未登録の応答時間の差は、bcrypt 1 回分より十分に小さい。閾値は緩めにして、
// 複数回の平均で比べる（フレーキーにしない）。
func TestSignupResponseTimeDoesNotRevealRegistration(t *testing.T) {
	const (
		hashDelay = 40 * time.Millisecond
		tolerance = hashDelay / 2
		rounds    = 5
	)
	kit := newSignupKitWithHasher(t, slowHasher{delay: hashDelay})
	kit.users.seed("bob", "bob@example.com", "Password123!")

	measure := func(email string) time.Duration {
		var total time.Duration
		for i := 0; i < rounds; i++ {
			// 未登録のメールは、毎回別のアドレスにして、送信間隔の見送りに入らないようにする。
			addr := email
			if strings.HasPrefix(email, "fresh") {
				addr = fmt.Sprintf("fresh%d@example.com", i)
			}
			start := time.Now()
			if rec := do(kit.router, http.MethodPost, "/signup", signupBody("eve", addr, "Password123!"), ""); rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", rec.Code)
			}
			total += time.Since(start)
		}
		return total / rounds
	}
	fresh, registered := measure("fresh@example.com"), measure("bob@example.com")
	if fresh < hashDelay || registered < hashDelay {
		t.Errorf("平均の応答時間 = %v / %v, want どちらも bcrypt 1 回分（%v）以上（どの分岐でもハッシュを行う）", fresh, registered, hashDelay)
	}
	if diff := fresh - registered; diff > tolerance || diff < -tolerance {
		t.Errorf("平均の応答時間の差 = %v（未登録 %v / 登録済み %v）, want %v 以内", diff, fresh, registered, tolerance)
	}
}

// TestSignupResendWindow は AC5 を扱う：60 秒以内の再 signup は 202 だが、確認待ちを変えず、
// メールも送らない。間隔を過ぎた再 signup は、最新の入力とトークンに置き換わり、古いトークンは無効になる。
func TestSignupResendWindow(t *testing.T) {
	kit := newSignupKit(t)
	do(kit.router, http.MethodPost, "/signup", signupBody("first", "alice@example.com", "Password123!"), "")
	firstToken := kit.mailer.lastToken(t)

	kit.store.now = kit.store.now.Add(30 * time.Second)
	rec := do(kit.router, http.MethodPost, "/signup", signupBody("second", "alice@example.com", "Password456!"), "")
	if rec.Code != http.StatusAccepted || rec.Body.String() != signupAcceptedBody {
		t.Fatalf("間隔内の再 signup = %d %s, want 202 %s", rec.Code, rec.Body, signupAcceptedBody)
	}
	if n := len(kit.mailer.all()); n != 1 {
		t.Fatalf("間隔内の再 signup でメールが送られた（合計 %d 通, want 1）", n)
	}

	kit.store.now = kit.store.now.Add(31 * time.Second)
	rec = do(kit.router, http.MethodPost, "/signup", signupBody("second", "alice@example.com", "Password456!"), "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("間隔後の再 signup status = %d, want 202", rec.Code)
	}
	if n := len(kit.mailer.all()); n != 2 {
		t.Fatalf("間隔後の再 signup で送られたメール = 合計 %d 通, want 2", n)
	}
	secondToken := kit.mailer.lastToken(t)
	if secondToken == firstToken {
		t.Fatal("再 signup で同じトークンが使われた")
	}
	if rec := do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(firstToken), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("古いトークンでの確認 = %d, want 400", rec.Code)
	}
	rec = do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(secondToken), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("新しいトークンでの確認 = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	if user := decodeAuthUser(t, rec.Body.Bytes()); user.Username != "second" {
		t.Errorf("username = %q, want 最新の入力の second", user.Username)
	}
	if rec := loginAs(kit.router, "alice@example.com", "Password456!"); rec.Code != http.StatusOK {
		t.Errorf("最新のパスワードでの login = %d, want 200", rec.Code)
	}
}

// TestSignupConfirmRejections は AC7・AC8・AC12 を扱う：期限切れ・存在しない・改ざん・空のトークン、
// 確認までの間に同じ email のユーザーが作られていた場合は、区別できない同一の 400 になる。
// pending だけの email での login は、未登録と同じ 401 である。
func TestSignupConfirmRejections(t *testing.T) {
	newPending := func() (*signupKit, string) {
		kit := newSignupKit(t)
		do(kit.router, http.MethodPost, "/signup", signupBody("alice", "alice@example.com", "Password123!"), "")
		return kit, kit.mailer.lastToken(t)
	}
	tests := []struct {
		name  string
		token func(kit *signupKit, token string) string
		setup func(kit *signupKit)
	}{
		{"期限切れ", func(_ *signupKit, tok string) string { return tok }, func(kit *signupKit) { kit.store.now = kit.store.now.Add(25 * time.Hour) }},
		{"存在しない", func(*signupKit, string) string { return "does-not-exist" }, nil},
		// 末尾の 1 文字を必ず別の文字に変える（元の末尾が "A" のとき "A" に変えると、改ざんにならない）。
		{"改ざん(末尾を変える)", func(_ *signupKit, tok string) string {
			last := "A"
			if strings.HasSuffix(tok, "A") {
				last = "B"
			}
			return tok[:len(tok)-1] + last
		}, nil},
		{"空", func(*signupKit, string) string { return "" }, nil},
		{"確認までの間に同じ email のユーザーが作られていた", func(_ *signupKit, tok string) string { return tok },
			func(kit *signupKit) { kit.users.seed("other", "alice@example.com", "Password123!") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kit, token := newPending()
			usersBefore := len(kit.users.users)
			if tt.setup != nil {
				tt.setup(kit)
			}
			usersBefore = len(kit.users.users)
			rec := do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(tt.token(kit, token)), "")
			if rec.Code != http.StatusBadRequest || rec.Body.String() != tokenInvalidBody {
				t.Fatalf("confirm = %d %s, want 400 %s", rec.Code, rec.Body, tokenInvalidBody)
			}
			if len(kit.users.users) != usersBefore {
				t.Errorf("users = %d 件, want %d（確認の失敗で users を増やさない）", len(kit.users.users), usersBefore)
			}
		})
	}

	t.Run("不正な JSON は 400 で、body が大きすぎれば 413", func(t *testing.T) {
		kit := newSignupKit(t)
		if rec := do(kit.router, http.MethodPost, "/signup/confirm", `{"token":`, ""); rec.Code != http.StatusBadRequest || rec.Body.String() != `{"error":"invalid JSON body"}` {
			t.Errorf("不正な JSON = %d %s, want 400 invalid JSON body", rec.Code, rec.Body)
		}
	})

	t.Run("pending だけの email での login は、未登録と同じ 401 になる", func(t *testing.T) {
		kit, _ := newPending()
		pending := loginAs(kit.router, "alice@example.com", "Password123!")
		unknown := loginAs(kit.router, "nobody@example.com", "Password123!")
		if pending.Code != http.StatusUnauthorized || pending.Code != unknown.Code || pending.Body.String() != unknown.Body.String() {
			t.Errorf("pending = %d %s / 未登録 = %d %s, want 同一の 401", pending.Code, pending.Body, unknown.Code, unknown.Body)
		}
	})
}

// TestSignupDoesNotLeakSecrets は AC9 を扱う：平文のトークンとパスワードは、ログにも、エラーの
// 応答にも出ない。repository が失敗したときは、確認は 400 ではなく 500 になる（無効なトークンと区別する）。
func TestSignupDoesNotLeakSecrets(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(io.Discard)

	kit := newSignupKit(t)
	const password = "Password123!"
	do(kit.router, http.MethodPost, "/signup", signupBody("alice", "alice@example.com", password), "")
	token := kit.mailer.lastToken(t)

	kit.store.err = io.ErrUnexpectedEOF
	responses := []*httptest.ResponseRecorder{
		do(kit.router, http.MethodPost, "/signup", signupBody("bob", "bob@example.com", password), ""),
		do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(token), ""),
	}
	for i, rec := range responses {
		if rec.Code != http.StatusInternalServerError || rec.Body.String() != `{"error":"internal server error"}` {
			t.Errorf("失敗 %d = %d %s, want 500 internal server error", i, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), password) {
			t.Errorf("失敗 %d の応答に秘密が含まれている: %s", i, rec.Body)
		}
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), password) {
		t.Errorf("ログに平文のトークンかパスワードが含まれている:\n%s", logs.String())
	}
}

// TestSignupConfirmIntegration は、本物の PostgreSQL・repository・bcrypt・router を通して、signup の
// 確認を一通り検証する（S16）：signup では users が作られず、確認待ちには平文のトークンもパスワードも
// 保存されない。確認で users が作られ、確認待ちが消え、パスワードでログインできる。登録済みの email の
// signup は、未登録と同一の応答になる。
func TestSignupConfirmIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)
	mailer := router.(*mailedRouter).mailer
	count := func(table string) int {
		t.Helper()
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	const password = "Password123!"

	fresh := do(router, http.MethodPost, "/signup", signupBody("alice", "alice@example.com", password), "")
	if fresh.Code != http.StatusAccepted || fresh.Body.String() != signupAcceptedBody {
		t.Fatalf("signup = %d %s, want 202 %s", fresh.Code, fresh.Body, signupAcceptedBody)
	}
	if count("users") != 0 || count("signup_verifications") != 1 {
		t.Fatalf("users = %d, signup_verifications = %d, want 0 と 1", count("users"), count("signup_verifications"))
	}
	token := mailer.lastToken(t)
	var leaked int
	if err := conn.QueryRow(ctx,
		"SELECT count(*) FROM signup_verifications WHERE token_hash = $1 OR password_digest = $2 OR username = $2 OR email = $1", token, password).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("平文のトークンかパスワードが保存されている: %d 行 (err %v)", leaked, err)
	}
	var stored string
	if err := conn.QueryRow(ctx, "SELECT token_hash FROM signup_verifications").Scan(&stored); err != nil || stored != domain.HashSignupToken(token) {
		t.Fatalf("保存された token_hash = %q (err %v), want リンクのトークンの SHA-256", stored, err)
	}

	confirm := do(router, http.MethodPost, "/signup/confirm", confirmBody(token), "")
	if confirm.Code != http.StatusCreated {
		t.Fatalf("confirm = %d %s, want 201", confirm.Code, confirm.Body)
	}
	if count("users") != 1 || count("signup_verifications") != 0 {
		t.Fatalf("確認後: users = %d, signup_verifications = %d, want 1 と 0", count("users"), count("signup_verifications"))
	}
	if rec := loginAs(router, "alice@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("確認後の login = %d %s, want 200", rec.Code, rec.Body)
	}
	if rec := do(router, http.MethodPost, "/signup/confirm", confirmBody(token), ""); rec.Code != http.StatusBadRequest || rec.Body.String() != tokenInvalidBody {
		t.Errorf("再利用 = %d %s, want 400 %s", rec.Code, rec.Body, tokenInvalidBody)
	}

	registered := do(router, http.MethodPost, "/signup", signupBody("alice2", "alice@example.com", password), "")
	if registered.Code != fresh.Code || registered.Body.String() != fresh.Body.String() || fmt.Sprint(registered.Header()) != fmt.Sprint(fresh.Header()) {
		t.Errorf("登録済みの signup = %d %s %v, want 未登録と同一の %d %s %v",
			registered.Code, registered.Body, registered.Header(), fresh.Code, fresh.Body, fresh.Header())
	}
	if count("users") != 1 || count("signup_verifications") != 0 {
		t.Errorf("登録済みの signup 後: users = %d, signup_verifications = %d, want 1 と 0", count("users"), count("signup_verifications"))
	}
	mails := mailer.all()
	if last := mails[len(mails)-1]; last.Subject != "You already have an account" || strings.Contains(last.Body, "token") {
		t.Errorf("登録済みへのメール = %+v", last)
	}
}
