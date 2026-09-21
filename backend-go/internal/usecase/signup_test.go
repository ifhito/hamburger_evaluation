package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// testSignupConfig は、signup のテストで使う設定である。
var testSignupConfig = usecase.SignupConfig{BaseURL: "https://app.example.com", Now: func() time.Time { return testNow }}

// testNow は、テストの時計の現在時刻である（冪等キーの時間の窓が決まる）。
var testNow = time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC)

// acceptedReceipt は、確認待ちを保存できた（送信の間隔の外だった）ときの結果である。
var acceptedReceipt = domain.SignupVerificationReceipt{Accepted: true, ID: uid.N(100), Generation: 3}

// fakeSignupRepo は、手書きの domain.SignupVerificationRepository（書き込み）の test double である。
// create・lock・discardOne が未設定のまま呼ばれると panic するので、想定外の書き込みに対して
// テストは fail-loud する。discard(期限切れの掃除)は、signup のたびに日和見的に呼ばれるので、
// 未設定なら何もしない。
type fakeSignupRepo struct {
	create       func(ctx context.Context, p domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error)
	lock         func(ctx context.Context, tokenHash string) error
	discardOne   func(ctx context.Context, id string) error
	discard      func(ctx context.Context, limit int) (int64, error)
	discardCalls int
}

func (f *fakeSignupRepo) CreateSignupVerification(ctx context.Context, p domain.CreateSignupVerificationParams) (domain.SignupVerificationReceipt, error) {
	if f.create == nil {
		panic("unexpected CreateSignupVerification call")
	}
	return f.create(ctx, p)
}

func (f *fakeSignupRepo) LockSignupVerification(ctx context.Context, tokenHash string) error {
	if f.lock == nil {
		panic("unexpected LockSignupVerification call")
	}
	return f.lock(ctx, tokenHash)
}

func (f *fakeSignupRepo) DiscardSignupVerification(ctx context.Context, id string) error {
	if f.discardOne == nil {
		panic("unexpected DiscardSignupVerification call")
	}
	return f.discardOne(ctx, id)
}

func (f *fakeSignupRepo) DiscardExpiredSignupVerifications(ctx context.Context, limit int) (int64, error) {
	f.discardCalls++
	if f.discard == nil {
		return 0, nil
	}
	return f.discard(ctx, limit)
}

// fakePendingQuery は、手書きの usecase.SignupVerificationQuery（確認待ちの読み取り）の test double である。
// get が未設定のまま呼ばれると panic する。
type fakePendingQuery struct {
	get func(ctx context.Context, tokenHash string) (domain.PendingSignup, error)
}

func (f *fakePendingQuery) GetSignupVerificationByTokenHash(ctx context.Context, tokenHash string) (domain.PendingSignup, error) {
	if f.get == nil {
		panic("unexpected GetSignupVerificationByTokenHash call")
	}
	return f.get(ctx, tokenHash)
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
	getByEmailIgnoreCase: func(context.Context, string) (domain.User, error) {
		return domain.User{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
	},
}

var alreadyRegistered = &fakeUserQuery{
	getByEmailIgnoreCase: func(context.Context, string) (domain.User, error) {
		return domain.User{ID: uid.N(7), Username: "alice", Email: "a@example.com"}, nil
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
			// 強度ルール（domain.PasswordIssues）が signup に適用されていることを示す代表例。
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
		query := &fakeUserQuery{getByEmailIgnoreCase: func(context.Context, string) (domain.User, error) {
			return domain.User{}, queryErr
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
	// 確認の手順を、UnitOfWork(まとめて 1 つのトランザクションにする範囲)の代役の中で動かし、
	// 「ロック → 読み取り → ユーザーの作成 → 確認待ちの削除」の順序と、確定・取り消しの回数を確かめる。
	pending := domain.PendingSignup{ID: uid.N(100), Email: "a@example.com", Username: "alice", PasswordDigest: "digest(pw)"}
	type confirmKit struct {
		signups *usecase.Signups
		unit    *uowtest.UoW
		ops     *[]string
	}
	newConfirmKit := func(lockErr, getErr, createErr, discardErr error) confirmKit {
		var ops []string
		unit := &uowtest.UoW{
			SignupVerifications: &fakeSignupRepo{
				lock: func(_ context.Context, tokenHash string) error {
					ops = append(ops, "lock:"+tokenHash)
					return lockErr
				},
				discardOne: func(_ context.Context, id string) error {
					ops = append(ops, "discard:"+id)
					return discardErr
				},
			},
			PendingSignups: &fakePendingQuery{get: func(_ context.Context, tokenHash string) (domain.PendingSignup, error) {
				ops = append(ops, "get:"+tokenHash)
				return pending, getErr
			}},
			Users: &fakeUserRepo{createUser: func(_ context.Context, p domain.CreateUserParams) (domain.User, error) {
				ops = append(ops, fmt.Sprintf("create:%s/%s/%s/admin=%t", p.Email, p.Username, p.PasswordDigest, p.Admin))
				if createErr != nil {
					return domain.User{}, createErr
				}
				return domain.User{ID: uid.N(5), Username: p.Username, Email: p.Email}, nil
			}},
		}
		signups := usecase.NewSignups(notRegistered, domain.NewSignupVerifications(&fakeSignupRepo{}), unit, fakeHasher{}, &recordingMailer{}, fakeIssuer{}, testSignupConfig)
		return confirmKit{signups: signups, unit: unit, ops: &ops}
	}
	hash := domain.HashSignupToken("raw-token")

	t.Run("有効なトークンなら、確認待ちをロックして読み、その内容でユーザーを作り、確認待ちを削除して確定する", func(t *testing.T) {
		kit := newConfirmKit(nil, nil, nil, nil)
		user, token, err := kit.signups.Confirm(context.Background(), "raw-token")
		if err != nil {
			t.Fatalf("Confirm returned error: %v", err)
		}
		wantOps := []string{
			"lock:" + hash, // 平文ではなく、保存の形(ハッシュ)で照合する
			"get:" + hash,
			"create:a@example.com/alice/digest(pw)/admin=false", // 確認待ちの内容で作る。管理者にはしない
			"discard:" + uid.N(100),
		}
		if !reflect.DeepEqual(*kit.ops, wantOps) {
			t.Errorf("操作の順序 = %v, want %v", *kit.ops, wantOps)
		}
		if kit.unit.Commits != 1 || kit.unit.Rollbacks != 0 {
			t.Errorf("確定 %d 回・取り消し %d 回, want 確定 1・取り消し 0", kit.unit.Commits, kit.unit.Rollbacks)
		}
		if want := (domain.User{ID: uid.N(5), Username: "alice", Email: "a@example.com"}); user != want {
			t.Errorf("user = %+v, want %+v", user, want)
		}
		if want := "token-for-" + uid.N(5); token != want {
			t.Errorf("token = %q, want %q", token, want)
		}
	})

	t.Run("期限切れ・存在しない・使用済みのトークンは ErrSignupTokenInvalid になり、ユーザーを作らず、取り消される", func(t *testing.T) {
		kit := newConfirmKit(fmt.Errorf("lock: %w", domain.ErrSignupTokenInvalid), nil, nil, nil)
		_, _, err := kit.signups.Confirm(context.Background(), "raw-token")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
		if want := []string{"lock:" + hash}; !reflect.DeepEqual(*kit.ops, want) {
			t.Errorf("操作 = %v, want %v（ロックに失敗したら、読み取りも作成も削除もしない）", *kit.ops, want)
		}
		if kit.unit.Commits != 0 || kit.unit.Rollbacks != 1 {
			t.Errorf("確定 %d 回・取り消し %d 回, want 確定 0・取り消し 1", kit.unit.Commits, kit.unit.Rollbacks)
		}
	})

	t.Run("ロックのあとに確認待ちが読めなくなっていても(使用済みと同じ)、ErrSignupTokenInvalid になり、取り消される", func(t *testing.T) {
		kit := newConfirmKit(nil, fmt.Errorf("get: %w", domain.ErrSignupTokenInvalid), nil, nil)
		_, _, err := kit.signups.Confirm(context.Background(), "raw-token")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
		if kit.unit.Commits != 0 || kit.unit.Rollbacks != 1 {
			t.Errorf("確定 %d 回・取り消し %d 回, want 確定 0・取り消し 1", kit.unit.Commits, kit.unit.Rollbacks)
		}
	})

	t.Run("確認までの間に同じ email のユーザーが作られていたときは、区別できない ErrSignupTokenInvalid になり、確認待ちは削除されず、取り消される", func(t *testing.T) {
		kit := newConfirmKit(nil, nil, fmt.Errorf("create: %w", domain.ErrEmailTaken), nil)
		_, _, err := kit.signups.Confirm(context.Background(), "raw-token")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) || errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("error = %v, want only %v", err, domain.ErrSignupTokenInvalid)
		}
		for _, op := range *kit.ops {
			if strings.HasPrefix(op, "discard:") {
				t.Errorf("ユーザーの作成に失敗したのに、確認待ちを削除した: %v", *kit.ops)
			}
		}
		if kit.unit.Commits != 0 || kit.unit.Rollbacks != 1 {
			t.Errorf("確定 %d 回・取り消し %d 回, want 確定 0・取り消し 1（確認待ちが消えない）", kit.unit.Commits, kit.unit.Rollbacks)
		}
	})

	t.Run("確認待ちの削除に失敗したら、作ったユーザーごと取り消され、エラーはそのまま伝播する", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		kit := newConfirmKit(nil, nil, nil, repoErr)
		_, _, err := kit.signups.Confirm(context.Background(), "raw-token")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("error = %v, want wrapped %v", err, repoErr)
		}
		if kit.unit.Commits != 0 || kit.unit.Rollbacks != 1 {
			t.Errorf("確定 %d 回・取り消し %d 回, want 確定 0・取り消し 1", kit.unit.Commits, kit.unit.Rollbacks)
		}
	})

	t.Run("トランザクションを開始できなければ、ユーザーを作らずにエラーを返す", func(t *testing.T) {
		kit := newConfirmKit(nil, nil, nil, nil)
		beginErr := errors.New("begin failed")
		kit.unit.BeginErr = beginErr
		if _, _, err := kit.signups.Confirm(context.Background(), "raw-token"); !errors.Is(err, beginErr) {
			t.Fatalf("error = %v, want wrapped %v", err, beginErr)
		}
		if len(*kit.ops) != 0 {
			t.Errorf("操作 = %v, want なし", *kit.ops)
		}
	})
}
