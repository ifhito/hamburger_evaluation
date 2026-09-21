package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	auth := newAuth(query, fakeHasher{}, fakeIssuer{}, fakeVerifier{})

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
			auth := newAuth(&fakeUserQuery{}, hasher, fakeIssuer{}, fakeVerifier{})
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
		auth := newAuth(notFound, hasher, fakeIssuer{}, fakeVerifier{})
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
		auth := newAuth(failing, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
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
	auth := newAuth(query, fakeHasher{}, fakeIssuer{}, verifier)

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
		auth := newAuth(failing, fakeHasher{}, fakeIssuer{}, verifier)
		_, err := auth.AuthenticateToken(context.Background(), "valid-active")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("AuthenticateToken error = %v, want wrapped %v", err, repoErr)
		}
	})
}
