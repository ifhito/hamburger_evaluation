package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeUsersRepo は、手書きの usecase.UsersRepository の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeUsersRepo struct {
	list          func(ctx context.Context) ([]domain.User, error)
	getByID       func(ctx context.Context, id int64) (domain.User, error)
	updateProfile func(ctx context.Context, id int64, changes usecase.ProfileChanges) (domain.User, error)
	discard       func(ctx context.Context, id int64) error
}

func (f *fakeUsersRepo) ListActiveUsers(ctx context.Context) ([]domain.User, error) {
	if f.list == nil {
		panic("unexpected ListActiveUsers call")
	}
	return f.list(ctx)
}

func (f *fakeUsersRepo) GetActiveUserByID(ctx context.Context, id int64) (domain.User, error) {
	if f.getByID == nil {
		panic("unexpected GetActiveUserByID call")
	}
	return f.getByID(ctx, id)
}

func (f *fakeUsersRepo) UpdateUserProfile(ctx context.Context, id int64, changes usecase.ProfileChanges) (domain.User, error) {
	if f.updateProfile == nil {
		panic("unexpected UpdateUserProfile call")
	}
	return f.updateProfile(ctx, id, changes)
}

func (f *fakeUsersRepo) DiscardUser(ctx context.Context, id int64) error {
	if f.discard == nil {
		panic("unexpected DiscardUser call")
	}
	return f.discard(ctx, id)
}

var (
	usersViewer = domain.User{ID: 1, Username: "alice", Email: "alice@example.com"}
	usersOther  = domain.User{ID: 2, Username: "bob", Email: "bob@example.com"}
)

// activeUsersByID は、指定されたユーザーを返し、それ以外のすべての id には
// domain.ErrUserNotFound を返す getByID の振る舞いを返す。
func activeUsersByID(users ...domain.User) func(context.Context, int64) (domain.User, error) {
	return func(_ context.Context, id int64) (domain.User, error) {
		for _, u := range users {
			if u.ID == id {
				return u, nil
			}
		}
		return domain.User{}, fmt.Errorf("get active user by id: %w", domain.ErrUserNotFound)
	}
}

func TestUsersList(t *testing.T) {
	want := []domain.User{usersViewer, usersOther}
	repo := &fakeUsersRepo{list: func(context.Context) ([]domain.User, error) { return want, nil }}
	got, err := usecase.NewUsers(repo, fakeHasher{}).List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}

	repoErr := errors.New("db down")
	failing := &fakeUsersRepo{list: func(context.Context) ([]domain.User, error) { return nil, repoErr }}
	if _, err := usecase.NewUsers(failing, fakeHasher{}).List(context.Background()); !errors.Is(err, repoErr) {
		t.Fatalf("List error = %v, want wrapped %v", err, repoErr)
	}
}

// TestUsersUpdateCheckOrder は、issue #16 AC2 の find-then-authorize の順序を
// 固定する。未知の target は、所有者でない場合でも ErrUserNotFound を返し、
// 存在する他人の target は、validation や書き込みの前に ErrForbidden を返す
// （未設定の updateProfile は、到達すれば panic する）。
func TestUsersUpdateCheckOrder(t *testing.T) {
	repo := &fakeUsersRepo{getByID: activeUsersByID(usersViewer, usersOther)}
	users := usecase.NewUsers(repo, fakeHasher{})

	if _, err := users.Update(context.Background(), usersViewer, 999, usecase.UpdateUserInput{}); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("unknown target error = %v, want %v", err, domain.ErrUserNotFound)
	}
	// 他人の target に対しては、不正な入力であっても 403 で応答され、
	// validation の結果で応答されることは決してない。
	input := usecase.UpdateUserInput{Username: strPtr("")}
	if _, err := users.Update(context.Background(), usersViewer, usersOther.ID, input); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("foreign target error = %v, want %v", err, domain.ErrForbidden)
	}
}

func TestUsersUpdateValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    usecase.UpdateUserInput
		wantMsgs []string
	}{
		{
			name:     "blank username",
			input:    usecase.UpdateUserInput{Username: strPtr("")},
			wantMsgs: []string{"Username can't be blank"},
		},
		{
			name:     "blank email",
			input:    usecase.UpdateUserInput{Email: strPtr("")},
			wantMsgs: []string{"Email can't be blank"},
		},
		{
			name:     "blank username and email collect both messages",
			input:    usecase.UpdateUserInput{Username: strPtr(""), Email: strPtr("")},
			wantMsgs: []string{"Username can't be blank", "Email can't be blank"},
		},
		{
			name:     "password over 72 bytes",
			input:    usecase.UpdateUserInput{Password: strPtr(strings.Repeat("a", 73))},
			wantMsgs: []string{"Password is too long (maximum is 72 characters)"},
		},
		{
			name: "mismatched confirmation",
			input: usecase.UpdateUserInput{
				Password:             strPtr("newpassword1"),
				PasswordConfirmation: strPtr("other"),
			},
			wantMsgs: []string{"Password confirmation doesn't match Password"},
		},
		{
			name:     "confirmation without password mismatches the absent password",
			input:    usecase.UpdateUserInput{PasswordConfirmation: strPtr("stray")},
			wantMsgs: []string{"Password confirmation doesn't match Password"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeUsersRepo{getByID: activeUsersByID(usersViewer)}
			_, err := usecase.NewUsers(repo, fakeHasher{}).Update(context.Background(), usersViewer, usersViewer.ID, tt.input)
			assertValidationError(t, err, tt.wantMsgs)
		})
	}
}

// TestUsersUpdateChanges は、repository に届くものを固定する。存在しない
// フィールドは nil のまま（部分更新）、空文字列のパスワードは存在しない
// ものとして扱われ（Rails の has_secure_password）、存在するパスワードは
// ハッシュ化されて届く。平文で届くことは決してない。
func TestUsersUpdateChanges(t *testing.T) {
	tests := []struct {
		name        string
		input       usecase.UpdateUserInput
		wantChanges usecase.ProfileChanges
	}{
		{
			name:        "empty input is a no-op update",
			input:       usecase.UpdateUserInput{},
			wantChanges: usecase.ProfileChanges{},
		},
		{
			name:        "username only leaves the rest nil",
			input:       usecase.UpdateUserInput{Username: strPtr("alice2")},
			wantChanges: usecase.ProfileChanges{Username: strPtr("alice2")},
		},
		{
			name:        "empty-string password is absent: no digest change, no error",
			input:       usecase.UpdateUserInput{Password: strPtr("")},
			wantChanges: usecase.ProfileChanges{},
		},
		{
			name:        "empty password with empty confirmation stays a no-op",
			input:       usecase.UpdateUserInput{Password: strPtr(""), PasswordConfirmation: strPtr("")},
			wantChanges: usecase.ProfileChanges{},
		},
		{
			name: "present password is hashed",
			input: usecase.UpdateUserInput{
				Password:             strPtr("newpassword1"),
				PasswordConfirmation: strPtr("newpassword1"),
			},
			wantChanges: usecase.ProfileChanges{PasswordDigest: strPtr("digest(newpassword1)")},
		},
		{
			name:  "all fields together",
			input: usecase.UpdateUserInput{Username: strPtr("alice2"), Email: strPtr("alice2@example.com"), Password: strPtr("newpassword1")},
			wantChanges: usecase.ProfileChanges{
				Username:       strPtr("alice2"),
				Email:          strPtr("alice2@example.com"),
				PasswordDigest: strPtr("digest(newpassword1)"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID int64
			var gotChanges usecase.ProfileChanges
			stored := domain.User{ID: usersViewer.ID, Username: "stored", Email: "stored@example.com"}
			repo := &fakeUsersRepo{
				getByID: activeUsersByID(usersViewer),
				updateProfile: func(_ context.Context, id int64, changes usecase.ProfileChanges) (domain.User, error) {
					gotID, gotChanges = id, changes
					return stored, nil
				},
			}
			got, err := usecase.NewUsers(repo, fakeHasher{}).Update(context.Background(), usersViewer, usersViewer.ID, tt.input)
			if err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			if got != stored {
				t.Errorf("Update = %+v, want the stored user %+v", got, stored)
			}
			if gotID != usersViewer.ID {
				t.Errorf("repository id = %d, want %d", gotID, usersViewer.ID)
			}
			if !reflect.DeepEqual(gotChanges, tt.wantChanges) {
				t.Errorf("repository changes = %s, want %s", profileChangesString(gotChanges), profileChangesString(tt.wantChanges))
			}
		})
	}
}

// profileChangesString は、失敗時に読みやすいよう、ポインタのフィールドを
// 文字列に整形する。
func profileChangesString(c usecase.ProfileChanges) string {
	deref := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%q", *p)
	}
	return fmt.Sprintf("{Username:%s Email:%s PasswordDigest:%s}", deref(c.Username), deref(c.Email), deref(c.PasswordDigest))
}

// TestUsersUpdateEmailTaken は usecase レベルで AC3 を扱う。repository の
// unique violation の sentinel が、Rails parity の validation message として
// 現れる。
func TestUsersUpdateEmailTaken(t *testing.T) {
	repo := &fakeUsersRepo{
		getByID: activeUsersByID(usersViewer),
		updateProfile: func(context.Context, int64, usecase.ProfileChanges) (domain.User, error) {
			return domain.User{}, fmt.Errorf("update user profile: email: %w", domain.ErrEmailTaken)
		},
	}
	_, err := usecase.NewUsers(repo, fakeHasher{}).Update(context.Background(), usersViewer, usersViewer.ID,
		usecase.UpdateUserInput{Email: strPtr("bob@example.com")})
	assertValidationError(t, err, []string{"Email has already been taken"})
}

// TestUsersDelete は削除のフローを固定する。AC2 のチェック順序（404 が
// 403 より先）、本人のみのルール、そして所有者に対する discard の呼び出しで
// ある。
func TestUsersDelete(t *testing.T) {
	t.Run("unknown target yields ErrUserNotFound even for a non-owner", func(t *testing.T) {
		repo := &fakeUsersRepo{getByID: activeUsersByID(usersViewer, usersOther)}
		if err := usecase.NewUsers(repo, fakeHasher{}).Delete(context.Background(), usersViewer, 999); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("foreign target yields ErrForbidden without a discard", func(t *testing.T) {
		repo := &fakeUsersRepo{getByID: activeUsersByID(usersViewer, usersOther)}
		if err := usecase.NewUsers(repo, fakeHasher{}).Delete(context.Background(), usersViewer, usersOther.ID); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("owner discards self", func(t *testing.T) {
		var discarded int64
		repo := &fakeUsersRepo{
			getByID: activeUsersByID(usersViewer),
			discard: func(_ context.Context, id int64) error { discarded = id; return nil },
		}
		if err := usecase.NewUsers(repo, fakeHasher{}).Delete(context.Background(), usersViewer, usersViewer.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		if discarded != usersViewer.ID {
			t.Errorf("discarded id = %d, want %d", discarded, usersViewer.ID)
		}
	})
}
