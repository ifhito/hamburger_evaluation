package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// testSignupConfig は、signup のテストで使う設定である。
var testSignupConfig = usecase.SignupConfig{BaseURL: "https://app.example.com", Now: func() time.Time { return testNow }}

// testNow は、テストの時計の現在時刻である（冪等キーの時間の窓が決まる）。
var testNow = time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC)

// acceptedReceipt は、確認待ちを保存できた（送信の間隔の外だった）ときの結果である。
var acceptedReceipt = domain.SignupVerificationReceipt{Accepted: true, ID: uid.N(100), Generation: 3}

// fakeSignupRepo は、手書きの domain.SignupVerificationRepository（書き込み）の test double である。
// create と confirm が未設定のまま呼ばれると panic するので、想定外の書き込みに対して
// テストは fail-loud する。discard は、signup のたびに日和見的に呼ばれるので、未設定なら
// 何もしない。
type fakeSignupRepo struct {
	create       func(ctx context.Context, p domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error)
	confirm      func(ctx context.Context, tokenHash string) (domain.User, error)
	discard      func(ctx context.Context, limit int) (int64, error)
	discardCalls int
}

func (f *fakeSignupRepo) CreateSignupVerification(ctx context.Context, p domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
	if f.create == nil {
		panic("unexpected CreateSignupVerification call")
	}
	return f.create(ctx, p)
}

func (f *fakeSignupRepo) CreateUserFromSignupVerification(ctx context.Context, tokenHash string) (domain.User, error) {
	if f.confirm == nil {
		panic("unexpected CreateUserFromSignupVerification call")
	}
	return f.confirm(ctx, tokenHash)
}

func (f *fakeSignupRepo) DiscardExpiredSignupVerifications(ctx context.Context, limit int) (int64, error) {
	f.discardCalls++
	if f.discard == nil {
		return 0, nil
	}
	return f.discard(ctx, limit)
}

// recordingMailer は、出すよう頼まれたメールの意図を記録する usecase.Mailer の fake である。
type recordingMailer struct {
	confirmations []usecase.SignupConfirmation
	notices       []usecase.AlreadyRegisteredNotice
}

func (m *recordingMailer) SendSignupConfirmation(n usecase.SignupConfirmation) {
	m.confirmations = append(m.confirmations, n)
}

func (m *recordingMailer) SendAlreadyRegistered(n usecase.AlreadyRegisteredNotice) {
	m.notices = append(m.notices, n)
}

// total は、出すよう頼まれたメールの合計である。
func (m *recordingMailer) total() int { return len(m.confirmations) + len(m.notices) }

var notRegistered = &fakeUserQuery{
	getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
		return usecase.UserCredentials{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
	},
}

var alreadyRegistered = &fakeUserQuery{
	getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
		return usecase.UserCredentials{User: domain.User{ID: uid.N(7), Username: "alice", Email: "a@example.com"}, PasswordDigest: "digest(existing)"}, nil
	},
}

var validSignup = usecase.SignupInput{
	Username:             "alice",
	Email:                "a@example.com",
	Password:             "Password123!",
	PasswordConfirmation: strPtr("Password123!"),
}

// TestSignupsRequestValidation は、登録の有無に依存しない検証のメッセージと順序を固定する。
func TestSignupsRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    usecase.SignupInput
		wantMsgs []string
	}{
		{
			name:     "username が空だと検証エラーになる",
			input:    usecase.SignupInput{Email: "a@example.com", Password: "Password123!"},
			wantMsgs: []string{"Username can't be blank"},
		},
		{
			name:     "email が空だと検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Password: "Password123!"},
			wantMsgs: []string{"Email can't be blank"},
		},
		{
			name:     "email の形式が不正だと検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "abc", Password: "Password123!"},
			wantMsgs: []string{"Email is invalid"},
		},
		{
			name:     "表示名つきの email は形式が不正として検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "Alice <a@example.com>", Password: "Password123!"},
			wantMsgs: []string{"Email is invalid"},
		},
		{
			name:     "password が空だと検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "a@example.com"},
			wantMsgs: []string{"Password can't be blank"},
		},
		{
			// 文字種は満たす 73 バイトにして、too long だけが出ることを見る。
			name: "72 バイトを超える password は too long だけの検証エラーになる",
			input: usecase.SignupInput{
				Username: "alice",
				Email:    "a@example.com",
				Password: "Aa1!" + strings.Repeat("x", 69),
			},
			wantMsgs: []string{"Password is too long (maximum is 72 characters)"},
		},
		{
			// 強度ルール（domain.ValidatePassword）が signup に適用されていることを示す代表例。
			// 全パターンと境界の網羅は domain のテストが担う。
			name:     "弱い password は短さと文字種の 2 件の検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "a@example.com", Password: "abc123"},
			wantMsgs: []string{"Password is too short (minimum is 8 characters)", "Password must include letters, numbers and symbols"},
		},
		{
			name:     "記号のない 8 バイトの password は文字種だけの検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "a@example.com", Password: "abcd1234"},
			wantMsgs: []string{"Password must include letters, numbers and symbols"},
		},
		{
			name:     "日本語と数字と記号だけの password は文字種の検証エラーになる",
			input:    usecase.SignupInput{Username: "alice", Email: "a@example.com", Password: "あいう123!!"},
			wantMsgs: []string{"Password must include letters, numbers and symbols"},
		},
		{
			name: "password confirmation が password と一致しないと検証エラーになる",
			input: usecase.SignupInput{
				Username:             "alice",
				Email:                "a@example.com",
				Password:             "Password123!",
				PasswordConfirmation: strPtr("Password124!"),
			},
			wantMsgs: []string{"Password confirmation doesn't match Password"},
		},
		{
			name:  "全フィールドが空だと 3 件の検証エラーをまとめて返す",
			input: usecase.SignupInput{},
			wantMsgs: []string{
				"Username can't be blank",
				"Email can't be blank",
				"Password can't be blank",
			},
		},
		{
			// API の外部契約であるメッセージの順序（username → email → password → confirmation）を固定する。
			name: "複数の違反があるとき username、email、password、confirmation の順に返す",
			input: usecase.SignupInput{
				Password:             "abc123",
				PasswordConfirmation: strPtr("other"),
			},
			wantMsgs: []string{
				"Username can't be blank",
				"Email can't be blank",
				"Password is too short (minimum is 8 characters)",
				"Password must include letters, numbers and symbols",
				"Password confirmation doesn't match Password",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// validation が失敗したとき、DB の検索にもハッシュ化にもメール送信にも到達してはならない
			// （fakeUserQuery と fakeSignupRepo の検索・書き込みは、呼ばれると panic する）。
			hasher := &recordingHasher{}
			mailer := &recordingMailer{}
			signups := newSignups(&fakeUserQuery{}, &fakeSignupRepo{}, hasher, mailer, fakeIssuer{}, testSignupConfig)
			err := signups.Request(context.Background(), tt.input)
			assertValidationError(t, err, tt.wantMsgs)
			if hasher.hashCalls != 0 {
				t.Errorf("Hash calls = %d, want 0 (validation failure must not hash)", hasher.hashCalls)
			}
			if mailer.total() != 0 {
				t.Errorf("出すよう頼まれたメール = %d 通, want 0", mailer.total())
			}
		})
	}
}

func TestSignupsRequest(t *testing.T) {
	okCreate := func(context.Context, domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
		return acceptedReceipt, nil
	}
	skipped := func(context.Context, domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
		return domain.SignupVerificationReceipt{}, nil
	}

	t.Run("未登録の email なら、確認待ちを保存し、確認の意図(リンク・有効期間・冪等キー)を 1 件だけ渡す", func(t *testing.T) {
		var got domain.CreateSignupVerificationParams
		repo := &fakeSignupRepo{create: func(_ context.Context, p domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
			got = p
			return acceptedReceipt, nil
		}}
		mailer := &recordingMailer{}
		signups := newSignups(notRegistered, repo, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig)

		if err := signups.Request(context.Background(), validSignup); err != nil {
			t.Fatalf("Request returned error: %v", err)
		}
		if got.Email != "a@example.com" || got.Username != "alice" || got.PasswordDigest != "digest(Password123!)" {
			t.Errorf("Create params = %+v", got)
		}
		if len(mailer.confirmations) != 1 || len(mailer.notices) != 0 {
			t.Fatalf("確認 %d 件・通知 %d 件, want 確認 1 件だけ", len(mailer.confirmations), len(mailer.notices))
		}
		n := mailer.confirmations[0]
		if n.To != "a@example.com" || n.ValidFor != domain.SignupTokenTTL {
			t.Errorf("意図 = %+v, want 宛先 a@example.com・有効期間 domain.SignupTokenTTL", n)
		}
		if want := domain.SignupConfirmationMailKey(acceptedReceipt.ID, acceptedReceipt.Generation); n.IdempotencyKey != want {
			t.Errorf("IdempotencyKey = %q, want %q（確認待ちの id と世代で決まる）", n.IdempotencyKey, want)
		}
		// リンクの平文トークンをハッシュ化したものが、保存した TokenHash と一致する。
		const prefix = "https://app.example.com/signup/confirm?token="
		raw, ok := strings.CutPrefix(n.ConfirmURL, prefix)
		if !ok {
			t.Fatalf("ConfirmURL = %q, want %s で始まる", n.ConfirmURL, prefix)
		}
		if domain.HashSignupToken(raw) != got.TokenHash {
			t.Errorf("リンクのトークンのハッシュ = %q, 保存した TokenHash = %q", domain.HashSignupToken(raw), got.TokenHash)
		}
		if got.TokenHash == raw {
			t.Error("TokenHash に平文のトークンが入っている")
		}
		for _, secret := range []string{"alice", "Password123!", got.PasswordDigest} {
			if strings.Contains(n.ConfirmURL, secret) {
				t.Errorf("ConfirmURL に %q が含まれている(利用者の入力・秘密をリンクに入れない)", secret)
			}
		}
	})

	t.Run("登録済みの email なら、確認待ちを作らず、通知の意図(ログイン画面の URL・冪等キー)を 1 件だけ渡す", func(t *testing.T) {
		repo := &fakeSignupRepo{} // create は呼ばれると panic する
		mailer := &recordingMailer{}
		signups := newSignups(alreadyRegistered, repo, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig)

		if err := signups.Request(context.Background(), validSignup); err != nil {
			t.Fatalf("Request returned error: %v", err)
		}
		if len(mailer.notices) != 1 || len(mailer.confirmations) != 0 {
			t.Fatalf("通知 %d 件・確認 %d 件, want 通知 1 件だけ", len(mailer.notices), len(mailer.confirmations))
		}
		n := mailer.notices[0]
		if n.To != "a@example.com" || n.SignInURL != "https://app.example.com/signin" {
			t.Errorf("意図 = %+v", n)
		}
		if want := domain.AlreadyRegisteredMailKey("a@example.com", testNow); n.IdempotencyKey != want {
			t.Errorf("IdempotencyKey = %q, want %q（email と時間の窓で決まる）", n.IdempotencyKey, want)
		}
	})

	t.Run("同じ窓の中の通知は同じ冪等キーで、窓をまたぐと別のキーになる(大文字小文字は区別しない)", func(t *testing.T) {
		now := testNow
		cfg := usecase.SignupConfig{BaseURL: "https://app.example.com", Now: func() time.Time { return now }}
		mailer := &recordingMailer{}
		signups := newSignups(alreadyRegistered, &fakeSignupRepo{}, &recordingHasher{}, mailer, fakeIssuer{}, cfg)
		request := func(email string) {
			in := validSignup
			in.Email = email
			if err := signups.Request(context.Background(), in); err != nil {
				t.Fatalf("Request returned error: %v", err)
			}
		}
		request("a@example.com")
		now = now.Add(10 * time.Second) // 12:00:40。同じ窓（12:00:00〜12:00:59）
		request("A@Example.com")
		now = now.Add(30 * time.Second) // 12:01:10。次の窓
		request("a@example.com")
		keys := []string{mailer.notices[0].IdempotencyKey, mailer.notices[1].IdempotencyKey, mailer.notices[2].IdempotencyKey}
		if keys[0] != keys[1] {
			t.Errorf("同じ窓の同じ宛先(大文字小文字違い)のキーが違う: %q / %q", keys[0], keys[1])
		}
		if keys[1] == keys[2] {
			t.Errorf("窓をまたいだのにキーが同じ: %q", keys[1])
		}
	})

	t.Run("登録の有無に関係なく、成功は同じ結果で、bcrypt は 1 回ずつ行う", func(t *testing.T) {
		branches := []struct {
			name  string
			query usecase.UserQuery
			repo  *fakeSignupRepo
		}{
			{"未登録", notRegistered, &fakeSignupRepo{create: okCreate}},
			{"登録済み", alreadyRegistered, &fakeSignupRepo{}},
			{"間隔内の再 signup", notRegistered, &fakeSignupRepo{create: skipped}},
		}
		for _, b := range branches {
			hasher := &recordingHasher{}
			err := newSignups(b.query, b.repo, hasher, &recordingMailer{}, fakeIssuer{}, testSignupConfig).Request(context.Background(), validSignup)
			if err != nil {
				t.Errorf("%s: Request returned error: %v (want nil。分岐で結果が変わると列挙できる)", b.name, err)
			}
			if hasher.hashCalls != 1 {
				t.Errorf("%s: Hash calls = %d, want 1 (どの分岐でも bcrypt を 1 回行う)", b.name, hasher.hashCalls)
			}
		}
	})

	t.Run("前回の送信から間隔内なら、成功するがメールの意図は渡さない", func(t *testing.T) {
		mailer := &recordingMailer{}
		err := newSignups(notRegistered, &fakeSignupRepo{create: skipped}, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig).Request(context.Background(), validSignup)
		if err != nil {
			t.Fatalf("Request returned error: %v", err)
		}
		if mailer.total() != 0 {
			t.Errorf("出すよう頼まれたメール = %d 通, want 0", mailer.total())
		}
	})

	t.Run("期限切れの掃除に失敗しても、signup の結果は変わらない", func(t *testing.T) {
		repo := &fakeSignupRepo{
			create:  okCreate,
			discard: func(context.Context, int) (int64, error) { return 0, errors.New("cleanup failed") },
		}
		mailer := &recordingMailer{}
		if err := newSignups(notRegistered, repo, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig).Request(context.Background(), validSignup); err != nil {
			t.Fatalf("Request returned error: %v", err)
		}
		if repo.discardCalls != 1 || mailer.total() != 1 {
			t.Errorf("discard calls = %d, メール = %d 通, want 1 と 1", repo.discardCalls, mailer.total())
		}
	})

	t.Run("email の検索に失敗したら、確認待ちも作らず、メールも頼まず、エラーを返す", func(t *testing.T) {
		queryErr := errors.New("connection lost")
		query := &fakeUserQuery{getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
			return usecase.UserCredentials{}, queryErr
		}}
		mailer := &recordingMailer{}
		err := newSignups(query, &fakeSignupRepo{}, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig).Request(context.Background(), validSignup)
		if !errors.Is(err, queryErr) {
			t.Fatalf("error = %v, want wrapped %v", err, queryErr)
		}
		if mailer.total() != 0 {
			t.Errorf("出すよう頼まれたメール = %d 通, want 0", mailer.total())
		}
	})

	t.Run("確認待ちの保存に失敗したら、メールを頼まずエラーを返す", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		repo := &fakeSignupRepo{create: func(context.Context, domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
			return domain.SignupVerificationReceipt{}, repoErr
		}}
		mailer := &recordingMailer{}
		err := newSignups(notRegistered, repo, &recordingHasher{}, mailer, fakeIssuer{}, testSignupConfig).Request(context.Background(), validSignup)
		if !errors.Is(err, repoErr) {
			t.Fatalf("error = %v, want wrapped %v", err, repoErr)
		}
		if mailer.total() != 0 {
			t.Errorf("出すよう頼まれたメール = %d 通, want 0", mailer.total())
		}
	})

	t.Run("時計を注入しなくても動く(既定は time.Now)", func(t *testing.T) {
		cfg := usecase.SignupConfig{BaseURL: "https://app.example.com"}
		mailer := &recordingMailer{}
		if err := newSignups(alreadyRegistered, &fakeSignupRepo{}, &recordingHasher{}, mailer, fakeIssuer{}, cfg).Request(context.Background(), validSignup); err != nil {
			t.Fatalf("Request returned error: %v", err)
		}
		if len(mailer.notices) != 1 || mailer.notices[0].IdempotencyKey == "" {
			t.Errorf("通知 = %+v", mailer.notices)
		}
	})
}

func TestSignupsConfirm(t *testing.T) {
	t.Run("有効なトークンなら、平文ではなくハッシュで確認し、作られたユーザーと認証トークンを返す", func(t *testing.T) {
		var gotHash string
		repo := &fakeSignupRepo{confirm: func(_ context.Context, tokenHash string) (domain.User, error) {
			gotHash = tokenHash
			return domain.User{ID: uid.N(5), Username: "alice", Email: "a@example.com"}, nil
		}}
		user, token, err := newSignups(notRegistered, repo, fakeHasher{}, &recordingMailer{}, fakeIssuer{}, testSignupConfig).
			Confirm(context.Background(), "raw-token")
		if err != nil {
			t.Fatalf("Confirm returned error: %v", err)
		}
		if gotHash != domain.HashSignupToken("raw-token") {
			t.Errorf("repository へ渡した値 = %q, want HashSignupToken(raw-token)", gotHash)
		}
		if want := (domain.User{ID: uid.N(5), Username: "alice", Email: "a@example.com"}); user != want {
			t.Errorf("user = %+v, want %+v", user, want)
		}
		if want := "token-for-" + uid.N(5); token != want {
			t.Errorf("token = %q, want %q", token, want)
		}
	})

	confirmErr := func(err error) *fakeSignupRepo {
		return &fakeSignupRepo{confirm: func(context.Context, string) (domain.User, error) { return domain.User{}, err }}
	}
	t.Run("期限切れ・存在しない・使用済みは ErrSignupTokenInvalid になる", func(t *testing.T) {
		repo := confirmErr(fmt.Errorf("confirm signup: %w", domain.ErrSignupTokenInvalid))
		_, _, err := newSignups(notRegistered, repo, fakeHasher{}, &recordingMailer{}, fakeIssuer{}, testSignupConfig).Confirm(context.Background(), "x")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
	})
	t.Run("確認までの間に同じ email のユーザーが作られていたときも、区別できない ErrSignupTokenInvalid になる", func(t *testing.T) {
		repo := confirmErr(fmt.Errorf("confirm signup: %w", domain.ErrEmailTaken))
		_, _, err := newSignups(notRegistered, repo, fakeHasher{}, &recordingMailer{}, fakeIssuer{}, testSignupConfig).Confirm(context.Background(), "x")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) || errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("error = %v, want only %v", err, domain.ErrSignupTokenInvalid)
		}
	})
	t.Run("それ以外の repository エラーは ErrSignupTokenInvalid にならずそのまま伝播する", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		_, _, err := newSignups(notRegistered, confirmErr(repoErr), fakeHasher{}, &recordingMailer{}, fakeIssuer{}, testSignupConfig).Confirm(context.Background(), "x")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("error = %v, want wrapped %v", err, repoErr)
		}
	})
}
