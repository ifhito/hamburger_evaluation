package domain_test

import (
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestUserProfileFor は、ユーザーのビューの規則をその唯一の置き場で固定する。
// ID と Username は常に入り、Email と Admin は viewer が本人のときだけ入る。
// admin の viewer であっても、他人の Email と Admin は決して入らない。CanEdit（編集・削除できるか）は
// 本人の viewer だけが true で、匿名・他人・admin の他人は false である。
func TestUserProfileFor(t *testing.T) {
	target := domain.User{ID: 1, Username: "alice", Email: "alice@example.com", Admin: false}
	targetAdmin := domain.User{ID: 2, Username: "root", Email: "root@example.com", Admin: true}
	otherUser := domain.User{ID: 3, Username: "bob", Email: "bob@example.com"}

	publicView := func(u domain.User) domain.UserProfile {
		return domain.UserProfile{ID: u.ID, Username: u.Username}
	}
	selfView := func(u domain.User) domain.UserProfile {
		return domain.UserProfile{ID: u.ID, Username: u.Username, Email: ptr(u.Email), Admin: ptr(u.Admin), CanEdit: true}
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
		{name: "admin でない本人の admin は nil ではなく false のポインタで返す", target: target, viewer: &target, want: domain.UserProfile{ID: 1, Username: "alice", Email: ptr("alice@example.com"), Admin: ptr(false), CanEdit: true}},
		{name: "admin の本人の viewer には admin が true の本人ビューを返す", target: targetAdmin, viewer: &targetAdmin, want: selfView(targetAdmin)},
		{name: "admin の他人の viewer にも、他人の email と admin は返さない", target: target, viewer: &targetAdmin, want: publicView(target)},
		{name: "admin の他人の viewer にも、admin である他人の email と admin は返さない", target: targetAdmin, viewer: &domain.User{ID: 9, Username: "root2", Email: "root2@example.com", Admin: true}, want: publicView(targetAdmin)},
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
