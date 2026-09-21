package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeUserQuery は、手書きの usecase.UserQuery の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeUserQuery struct {
	getByEmail func(ctx context.Context, email string) (usecase.UserCredentials, error)
	getByID    func(ctx context.Context, id string) (domain.User, error)
}

func (f *fakeUserQuery) GetActiveUserByEmail(ctx context.Context, email string) (usecase.UserCredentials, error) {
	if f.getByEmail == nil {
		panic("unexpected GetActiveUserByEmail call")
	}
	return f.getByEmail(ctx, email)
}

func (f *fakeUserQuery) GetActiveUserByID(ctx context.Context, id string) (domain.User, error) {
	if f.getByID == nil {
		panic("unexpected GetActiveUserByID call")
	}
	return f.getByID(ctx, id)
}

// fakeUserRepo は、手書きの domain.UserRepository（書き込み）の test double
// である。未設定の振る舞いは panic するので、想定外の呼び出し、特にテスト対象の
// フローの外での書き込みに対して、テストは fail-loud する。
type fakeUserRepo struct {
	createUser    func(ctx context.Context, params domain.CreateUserParams) (domain.User, error)
	updateProfile func(ctx context.Context, id string, changes domain.ProfileChanges) (domain.User, error)
	discard       func(ctx context.Context, id string) error
}

func (f *fakeUserRepo) CreateUser(ctx context.Context, params domain.CreateUserParams) (domain.User, error) {
	if f.createUser == nil {
		panic("unexpected CreateUser call")
	}
	return f.createUser(ctx, params)
}

func (f *fakeUserRepo) UpdateUserProfile(ctx context.Context, id string, changes domain.ProfileChanges) (domain.User, error) {
	if f.updateProfile == nil {
		panic("unexpected UpdateUserProfile call")
	}
	return f.updateProfile(ctx, id, changes)
}

func (f *fakeUserRepo) DiscardUser(ctx context.Context, id string) error {
	if f.discard == nil {
		panic("unexpected DiscardUser call")
	}
	return f.discard(ctx, id)
}

// fakeHasher は digest に決定的な印を付けるので、テストは本物の bcrypt の
// 処理なしに、何が保存・比較されたかをアサートできる。
type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) { return "digest(" + password + ")", nil }

func (fakeHasher) Compare(digest, password string) error {
	if digest != "digest("+password+")" {
		return errors.New("password mismatch")
	}
	return nil
}

// recordingHasher は fakeHasher と同様に振る舞うが、Hash と Compare の呼び出しを
// 数えるので、テストは Login の未知の email の経路でのダミー比較や、
// 検証エラー時にハッシュ化へ進まないことをアサートできる。
type recordingHasher struct {
	hashCalls    int
	compareCalls int
}

func (h *recordingHasher) Hash(password string) (string, error) {
	h.hashCalls++
	return "digest(" + password + ")", nil
}

func (h *recordingHasher) Compare(digest, password string) error {
	h.compareCalls++
	if digest != "digest("+password+")" {
		return errors.New("password mismatch")
	}
	return nil
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(userID string) (string, error) {
	return "token-for-" + userID, nil
}

type fakeVerifier struct {
	verify func(token string) (string, error)
}

func (f fakeVerifier) Verify(token string) (string, error) { return f.verify(token) }

func strPtr(s string) *string { return &s }

// assertValidationError は、err がちょうど wantMsgs を保持する
// *domain.ValidationError でない限り、テストを失敗させる。
func assertValidationError(t *testing.T, err error, wantMsgs []string) {
	t.Helper()
	var vErr *domain.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("error = %v (%T), want *domain.ValidationError", err, err)
	}
	if !reflect.DeepEqual(vErr.Messages, wantMsgs) {
		t.Fatalf("validation messages = %q, want %q", vErr.Messages, wantMsgs)
	}
}

func TestAuthSignupValidation(t *testing.T) {
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
			// validation が失敗したとき、repository にもハッシュ化にも到達してはならない。
			hasher := &recordingHasher{}
			auth := newAuth(&fakeUserQuery{}, &fakeUserRepo{}, hasher, fakeIssuer{}, fakeVerifier{})
			_, _, err := auth.Signup(context.Background(), tt.input)
			assertValidationError(t, err, tt.wantMsgs)
			if hasher.hashCalls != 0 {
				t.Errorf("Hash calls = %d, want 0 (validation failure must not hash)", hasher.hashCalls)
			}
		})
	}
}

func TestAuthSignup(t *testing.T) {
	t.Run("有効な入力なら、ハッシュ化した password で非 admin ユーザーを作成し token を返す", func(t *testing.T) {
		var gotParams domain.CreateUserParams
		repo := &fakeUserRepo{
			createUser: func(_ context.Context, params domain.CreateUserParams) (domain.User, error) {
				gotParams = params
				return domain.User{ID: uid.N(1), Username: params.Username, Email: params.Email, Admin: params.Admin}, nil
			},
		}
		auth := newAuth(&fakeUserQuery{}, repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})

		user, token, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username:             "alice",
			Email:                "a@example.com",
			Password:             "Password123!",
			PasswordConfirmation: strPtr("Password123!"),
		})
		if err != nil {
			t.Fatalf("Signup returned error: %v", err)
		}
		wantParams := domain.CreateUserParams{
			Username:       "alice",
			Email:          "a@example.com",
			PasswordDigest: "digest(Password123!)",
			Admin:          false,
		}
		if gotParams != wantParams {
			t.Fatalf("CreateUser params = %+v, want %+v", gotParams, wantParams)
		}
		wantUser := domain.User{ID: uid.N(1), Username: "alice", Email: "a@example.com", Admin: false}
		if user != wantUser {
			t.Fatalf("Signup user = %+v, want %+v", user, wantUser)
		}
		if want := "token-for-" + uid.N(1); token != want {
			t.Fatalf("Signup token = %q, want %q", token, want)
		}
	})

	t.Run("password confirmation が nil でも受け付ける", func(t *testing.T) {
		repo := &fakeUserRepo{
			createUser: func(_ context.Context, params domain.CreateUserParams) (domain.User, error) {
				return domain.User{ID: uid.N(2), Username: params.Username, Email: params.Email}, nil
			},
		}
		auth := newAuth(&fakeUserQuery{}, repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "Password123!",
		})
		if err != nil {
			t.Fatalf("Signup returned error: %v", err)
		}
	})

	t.Run("email が重複していると検証エラーとして返る", func(t *testing.T) {
		repo := &fakeUserRepo{
			createUser: func(context.Context, domain.CreateUserParams) (domain.User, error) {
				return domain.User{}, fmt.Errorf("create user: %w", domain.ErrEmailTaken)
			},
		}
		auth := newAuth(&fakeUserQuery{}, repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "Password123!",
		})
		assertValidationError(t, err, []string{"Email has already been taken"})
	})

	t.Run("それ以外の repository エラーはそのまま伝播する", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		repo := &fakeUserRepo{
			createUser: func(context.Context, domain.CreateUserParams) (domain.User, error) {
				return domain.User{}, repoErr
			},
		}
		auth := newAuth(&fakeUserQuery{}, repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "Password123!",
		})
		if !errors.Is(err, repoErr) {
			t.Fatalf("error = %v, want wrapped %v", err, repoErr)
		}
	})
}

func TestAuthLogin(t *testing.T) {
	activeUser := domain.User{ID: uid.N(7), Username: "alice", Email: "a@example.com"}
	query := &fakeUserQuery{
		getByEmail: func(_ context.Context, email string) (usecase.UserCredentials, error) {
			if email != "a@example.com" {
				return usecase.UserCredentials{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			}
			return usecase.UserCredentials{User: activeUser, PasswordDigest: "digest(Password123!)"}, nil
		},
	}
	auth := newAuth(query, &fakeUserRepo{}, fakeHasher{}, fakeIssuer{}, fakeVerifier{})

	tests := []struct {
		name      string
		email     string
		password  string
		wantErr   error
		wantToken string
	}{
		{name: "正しい認証情報なら token を返す", email: "a@example.com", password: "Password123!", wantToken: "token-for-" + uid.N(7)},
		{name: "password が誤っていると invalid credentials になる", email: "a@example.com", password: "Wrongpass1!", wantErr: domain.ErrInvalidCredentials},
		{name: "未知の email だと invalid credentials になる", email: "b@example.com", password: "Password123!", wantErr: domain.ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, token, err := auth.Login(context.Background(), tt.email, tt.password)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Login error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Login returned error: %v", err)
			}
			if user != activeUser {
				t.Fatalf("Login user = %+v, want %+v", user, activeUser)
			}
			if token != tt.wantToken {
				t.Fatalf("Login token = %q, want %q", token, tt.wantToken)
			}
		})
	}

	// 認証情報が signup と同じ規則（domain.ValidateCredentials）を満たさないときは、
	// DB の検索も hash の比較も行わず、検証エラーを返す（Story #61）。
	// getByEmail を設定しない fakeUserQuery は、呼ばれると panic するので、検索されないことも固定される。
	rejected := []struct {
		name     string
		email    string
		password string
		want     []string
	}{
		{"email と password が空なら blank を返す", "", "", []string{"Email can't be blank", "Password can't be blank"}},
		{"形式の合わない email は invalid を返す", "abc", "Password123!", []string{"Email is invalid"}},
		{"強度を満たさない password は password の違反を返す", "a@example.com", "weakpassword", []string{"Password must include letters, numbers and symbols"}},
		{"短い password は短さと文字種の違反を返す", "a@example.com", "a", []string{
			"Password is too short (minimum is 8 characters)",
			"Password must include letters, numbers and symbols",
		}},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			hasher := &recordingHasher{}
			auth := newAuth(&fakeUserQuery{}, &fakeUserRepo{}, hasher, fakeIssuer{}, fakeVerifier{})
			_, token, err := auth.Login(context.Background(), tt.email, tt.password)
			var vErr *domain.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("Login error = %v, want *domain.ValidationError", err)
			}
			if !reflect.DeepEqual(vErr.Messages, tt.want) {
				t.Errorf("Login messages = %v, want %v", vErr.Messages, tt.want)
			}
			if token != "" {
				t.Errorf("Login token = %q, want empty", token)
			}
			if hasher.compareCalls != 0 {
				t.Errorf("Compare calls = %d, want 0 (規則を満たさない入力では比較しない)", hasher.compareCalls)
			}
		})
	}

	t.Run("未知の email でもダミーのハッシュ比較を行う", func(t *testing.T) {
		// タイミングのサイドチャネル対策：Compare の呼び出しがなければ、
		// 未知の email の経路は誤ったパスワードの経路より測定できるほど
		// 速く返り、email の列挙を許してしまう。
		notFound := &fakeUserQuery{
			getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
				return usecase.UserCredentials{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			},
		}
		hasher := &recordingHasher{}
		auth := newAuth(notFound, &fakeUserRepo{}, hasher, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Login(context.Background(), "b@example.com", "Password123!")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
		if hasher.compareCalls != 1 {
			t.Fatalf("Compare calls = %d, want 1 (dummy comparison on not-found path)", hasher.compareCalls)
		}
	})

	t.Run("repository の失敗は invalid credentials にならずそのまま伝播する", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		failing := &fakeUserQuery{
			getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
				return usecase.UserCredentials{}, repoErr
			},
		}
		auth := newAuth(failing, &fakeUserRepo{}, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Login(context.Background(), "a@example.com", "Password123!")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want wrapped %v", err, repoErr)
		}
	})
}

func TestAuthAuthenticateToken(t *testing.T) {
	activeUser := domain.User{ID: uid.N(7), Username: "alice", Email: "a@example.com"}
	query := &fakeUserQuery{
		getByID: func(_ context.Context, id string) (domain.User, error) {
			if id != activeUser.ID {
				// 未知のユーザーと discard 済みのユーザーは、query の
				// 境界ではどちらも "not found" になる。
				return domain.User{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			}
			return activeUser, nil
		},
	}
	verifier := fakeVerifier{verify: func(token string) (string, error) {
		switch token {
		case "valid-active":
			return activeUser.ID, nil
		case "valid-discarded":
			return uid.N(8), nil
		default:
			// 改ざんされた、期限切れの、アルゴリズムが誤っているトークンの
			// 代役であり、これらはすべて本物の verifier に拒否される
			// （infra の JWT テストを参照）。
			return "", errors.New("invalid token")
		}
	}}
	auth := newAuth(query, &fakeUserRepo{}, fakeHasher{}, fakeIssuer{}, verifier)

	tests := []struct {
		name    string
		token   string
		wantErr error
	}{
		{name: "active ユーザーの有効な token ならそのユーザーを返す", token: "valid-active"},
		{name: "不正な token は unauthenticated になる", token: "tampered-or-expired", wantErr: domain.ErrUnauthenticated},
		{name: "未知または discard 済みのユーザーの token は unauthenticated になる", token: "valid-discarded", wantErr: domain.ErrUnauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := auth.AuthenticateToken(context.Background(), tt.token)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("AuthenticateToken error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AuthenticateToken returned error: %v", err)
			}
			if user != activeUser {
				t.Fatalf("AuthenticateToken user = %+v, want %+v", user, activeUser)
			}
		})
	}

	t.Run("repository の失敗は unauthenticated にならずそのまま伝播する", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		failing := &fakeUserQuery{
			getByID: func(context.Context, string) (domain.User, error) {
				return domain.User{}, repoErr
			},
		}
		auth := newAuth(failing, &fakeUserRepo{}, fakeHasher{}, fakeIssuer{}, verifier)
		_, err := auth.AuthenticateToken(context.Background(), "valid-active")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("AuthenticateToken error = %v, want wrapped %v", err, repoErr)
		}
	})
}
