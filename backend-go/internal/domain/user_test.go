package domain_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestUserProfileFor は、ユーザーのビューの規則をその唯一の置き場で固定する。
// ID・ユーザー名・自己紹介文は常に入り、Email と Admin は viewer が本人のときだけ入る。
// admin の viewer であっても、他人の Email と Admin は決して入らない。CanEdit（編集・削除できるか）は
// 本人の viewer だけが true で、匿名・他人・admin の他人は false である。
func TestUserProfileFor(t *testing.T) {
	target := domain.User{ID: uid.N(1), Username: "alice", Bio: "alice の自己紹介", Email: "alice@example.com", Admin: false}
	targetAdmin := domain.User{ID: uid.N(2), Username: "root", Bio: "root の自己紹介", Email: "root@example.com", Admin: true}
	otherUser := domain.User{ID: uid.N(3), Username: "bob", Email: "bob@example.com"}

	publicView := func(u domain.User) domain.UserProfile {
		return domain.UserProfile{ID: u.ID, Username: u.Username, Bio: u.Bio}
	}
	selfView := func(u domain.User) domain.UserProfile {
		return domain.UserProfile{ID: u.ID, Username: u.Username, Bio: u.Bio, Email: ptr(u.Email), Admin: ptr(u.Admin), CanEdit: true}
	}

	tests := []struct {
		name   string
		target domain.User
		viewer *domain.User
		want   domain.UserProfile
	}{
		{name: "匿名の viewer には公開ビュー（email と admin は nil）を返す", target: target, viewer: nil, want: publicView(target)},
		{name: "他人の viewer には公開ビューを返す", target: target, viewer: &otherUser, want: publicView(target)},
		{name: "本人の viewer には email と admin を含む本人ビューを返す", target: target, viewer: &target, want: selfView(target)},
		{name: "admin でない本人の admin は nil ではなく false のポインタで返す", target: target, viewer: &target, want: domain.UserProfile{ID: uid.N(1), Username: "alice", Bio: "alice の自己紹介", Email: ptr("alice@example.com"), Admin: ptr(false), CanEdit: true}},
		{name: "admin の本人の viewer には admin が true の本人ビューを返す", target: targetAdmin, viewer: &targetAdmin, want: selfView(targetAdmin)},
		{name: "admin の他人の viewer にも、他人の email と admin は返さない", target: target, viewer: &targetAdmin, want: publicView(target)},
		{name: "admin の他人の viewer にも、admin である他人の email と admin は返さない", target: targetAdmin, viewer: &domain.User{ID: uid.N(9), Username: "root2", Email: "root2@example.com", Admin: true}, want: publicView(targetAdmin)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.target.ProfileFor(tt.viewer)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProfileFor(%+v) = %+v, want %+v", tt.viewer, got, tt.want)
			}
		})
	}
}

// TestUserCanModerate は、moderation の権限が admin だけにあることを固定する
// (usecase の認可と、API が返す can_moderate が同じ値を使う)。
func TestUserCanModerate(t *testing.T) {
	if !(domain.User{ID: uid.N(1), Admin: true}).CanModerate() {
		t.Error("admin の CanModerate() = false, want true")
	}
	if (domain.User{ID: uid.N(2), Admin: false}).CanModerate() {
		t.Error("一般のユーザーの CanModerate() = true, want false")
	}
}

// createRecordingRepo は、Create が渡した内容を記録する domain.UserRepository の代役である。
type createRecordingRepo struct {
	created []domain.CreateUserParams
}

func (r *createRecordingRepo) CreateUser(_ context.Context, p domain.CreateUserParams) (domain.User, error) {
	r.created = append(r.created, p)
	return domain.User{ID: "u1", Username: p.Username, Email: p.Email}, nil
}

func (*createRecordingRepo) UpdateUserProfile(context.Context, string, domain.ProfileChanges) (domain.User, error) {
	panic("unexpected UpdateUserProfile call")
}

func (*createRecordingRepo) DiscardUser(context.Context, string) error {
	panic("unexpected DiscardUser call")
}

// TestUsersCreatePasswordChoice は、ユーザーの作成で、パスワードの扱いが明示(Passwordless)と合わないものを、
// repository に渡さずに拒否することを固定する。空の digest が、黙って「パスワードなし」のアカウントになると、
// 呼び出し側の不具合に気づけない。
func TestUsersCreatePasswordChoice(t *testing.T) {
	cases := []struct {
		name    string
		params  domain.CreateUserParams
		wantErr bool
	}{
		{"digest があり、パスワードなしを明示していなければ、作れる", domain.CreateUserParams{Email: "a@example.com", Username: "a", PasswordDigest: "digest"}, false},
		{"digest が空で、パスワードなしを明示していれば、作れる(パスワードなしのアカウント)", domain.CreateUserParams{Email: "a@example.com", Username: "a", Passwordless: true}, false},
		{"digest が空で、パスワードなしを明示していなければ、誤りとして拒否する", domain.CreateUserParams{Email: "a@example.com", Username: "a"}, true},
		{"パスワードなしを明示したのに digest があれば、誤りとして拒否する", domain.CreateUserParams{Email: "a@example.com", Username: "a", PasswordDigest: "digest", Passwordless: true}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &createRecordingRepo{}
			_, err := domain.NewUsers(repo).Create(context.Background(), tt.params)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidPasswordDigest) {
					t.Fatalf("err = %v, want ErrInvalidPasswordDigest", err)
				}
				if len(repo.created) != 0 {
					t.Fatalf("拒否したのに、repository に渡された: %+v", repo.created)
				}
				return
			}
			if err != nil || len(repo.created) != 1 {
				t.Fatalf("err = %v, repository への呼び出し = %d 回, want エラーなしで 1 回", err, len(repo.created))
			}
		})
	}
}
