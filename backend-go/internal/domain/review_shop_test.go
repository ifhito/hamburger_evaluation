package domain_test

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestReviewShopFor は、レビューの詳細で、閲覧者にとってのショップと、そこに書けるか(can_review)が、
// 「書けるショップの先頭 → 見えるショップの先頭 → なし」で決まり、見えないショップが応答に影響しないことを確かめる。
func TestReviewShopFor(t *testing.T) {
	alice := &domain.User{ID: "alice", Username: "alice"}
	bob := &domain.User{ID: "bob", Username: "bob"}
	admin := &domain.User{ID: "root", Username: "root", Admin: true}
	ref := func(id string) *domain.ShopRef { return &domain.ShopRef{ID: id, Name: "name-" + id} }
	shop := func(id string, status domain.ShopStatus, creator string) domain.Shop {
		s := domain.Shop{ID: id, Name: "name-" + id, Status: status}
		if creator != "" {
			s.CreatorID = &creator
		}
		return s
	}
	active := shop("active", domain.ShopStatusActive, "")
	active2 := shop("active2", domain.ShopStatusActive, "")
	pendingByAlice := shop("pending", domain.ShopStatusPending, "alice")
	rejectedByAlice := shop("rejected", domain.ShopStatusRejected, "alice")

	for _, tt := range []struct {
		name          string
		shops         []domain.Shop
		viewer        *domain.User
		want          *domain.ShopRef
		wantCanReview bool
	}{
		{"匿名は、承認済みのショップが見えるが、書けない", []domain.Shop{active}, nil, ref("active"), false},
		{"ログイン済みの利用者は、承認済みのショップに書ける", []domain.Shop{active}, bob, ref("active"), true},
		{"作成者は、自分の承認待ちのショップに書ける", []domain.Shop{pendingByAlice}, alice, ref("pending"), true},
		{"作成者以外には、承認待ちのショップは見えず、何も返さない", []domain.Shop{pendingByAlice}, bob, nil, false},
		{"匿名には、承認待ちのショップは見えず、何も返さない", []domain.Shop{pendingByAlice}, nil, nil, false},
		{"管理者は、承認待ちのショップに書ける", []domain.Shop{pendingByAlice}, admin, ref("pending"), true},
		{"作成者は、自分の却下されたショップが見えるが、書けない", []domain.Shop{rejectedByAlice}, alice, ref("rejected"), false},
		{"管理者は、却下されたショップが見えるが、書けない", []domain.Shop{rejectedByAlice}, admin, ref("rejected"), false},
		{"作成者以外には、却下されたショップは見えず、何も返さない", []domain.Shop{rejectedByAlice}, bob, nil, false},
		{"ショップがなければ(ショップに紐づかないバーガー)、何も返さず、エラーにもしない", nil, bob, nil, false},
		{
			"古い方が却下されていても、新しい承認済みのショップが書けるなら、そちらを返す(ショップ詳細と食い違わない)",
			[]domain.Shop{shop("rejected-old", domain.ShopStatusRejected, "bob"), active}, bob, ref("active"), true,
		},
		{
			"見えない古いショップがあっても、その存在は応答に出ず、見えるショップを根拠に決まる",
			[]domain.Shop{pendingByAlice, active}, bob, ref("active"), true,
		},
		{
			"見えるショップが複数で、どこにも書けないときは、見える先頭を返す",
			[]domain.Shop{active, active2}, nil, ref("active"), false,
		},
		{
			"書けるショップが複数のときは、書ける先頭(作成の古い方)を返す",
			[]domain.Shop{rejectedByAlice, active2, active}, alice, ref("active2"), true,
		},
		{
			"匿名は、見えないショップを飛ばして、見える先頭を返す",
			[]domain.Shop{rejectedByAlice, pendingByAlice, active2}, nil, ref("active2"), false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, canReview := domain.ReviewShopFor(tt.shops, tt.viewer)
			switch {
			case (got == nil) != (tt.want == nil), got != nil && *got != *tt.want:
				t.Errorf("shop = %+v, want %+v", got, tt.want)
			}
			if canReview != tt.wantCanReview {
				t.Errorf("canReview = %v, want %v", canReview, tt.wantCanReview)
			}
		})
	}
}
