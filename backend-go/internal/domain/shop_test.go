package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func ptr[T any](v T) *T { return &v }

// TestShopVisibility は、shop の可視性ルールをその唯一の置き場で固定する。
// 匿名の viewer は active な shop のみ見え、通常のユーザーは自分が作成した
// shop（どの status でも）も見え、admin はすべて見える。
func TestShopVisibility(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	admin := domain.User{ID: 2, Username: "root", Admin: true}

	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	pendingOwn := domain.Shop{ID: 11, Status: domain.ShopStatusPending, CreatorID: ptr(alice.ID)}
	pendingOther := domain.Shop{ID: 12, Status: domain.ShopStatusPending, CreatorID: ptr(int64(99))}
	rejectedNoCreator := domain.Shop{ID: 13, Status: domain.ShopStatusRejected}

	tests := []struct {
		name   string
		viewer *domain.User
		shop   domain.Shop
		want   bool
	}{
		{name: "匿名ユーザーは active な shop を見られる", viewer: nil, shop: activeShop, want: true},
		{name: "匿名ユーザーは pending な shop を見られない", viewer: nil, shop: pendingOwn, want: false},
		{name: "匿名ユーザーは rejected な shop を見られない", viewer: nil, shop: rejectedNoCreator, want: false},
		{name: "ユーザーは active な shop を見られる", viewer: &alice, shop: activeShop, want: true},
		{name: "ユーザーは自分の pending な shop を見られる", viewer: &alice, shop: pendingOwn, want: true},
		{name: "ユーザーは他人の pending な shop を見られない", viewer: &alice, shop: pendingOther, want: false},
		{name: "ユーザーは creator のない rejected な shop を見られない", viewer: &alice, shop: rejectedNoCreator, want: false},
		{name: "admin は active な shop を見られる", viewer: &admin, shop: activeShop, want: true},
		{name: "admin はどの pending な shop も見られる", viewer: &admin, shop: pendingOther, want: true},
		{name: "admin は rejected な shop を見られる", viewer: &admin, shop: rejectedNoCreator, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis := domain.ShopVisibilityFor(tt.viewer)
			if got := vis.CanView(tt.shop); got != tt.want {
				t.Errorf("CanView(%+v) = %v, want %v (descriptor %+v)", tt.shop, got, tt.want, vis)
			}
		})
	}
}

// TestShopCanBeReviewedBy は reviewable ルールをその唯一の置き場で固定する
// （issue #14 AC2）。rejected の shop は決して reviewable ではなく、active な
// shop は認証済みの誰でも reviewable であり、pending な shop はその creator か
// admin のみが reviewable である。
func TestShopCanBeReviewedBy(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}

	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	pendingOwn := domain.Shop{ID: 11, Status: domain.ShopStatusPending, CreatorID: ptr(alice.ID)}
	pendingNoCreator := domain.Shop{ID: 12, Status: domain.ShopStatusPending}
	rejectedOwn := domain.Shop{ID: 13, Status: domain.ShopStatusRejected, CreatorID: ptr(alice.ID)}

	tests := []struct {
		name   string
		viewer domain.User
		shop   domain.Shop
		want   bool
	}{
		{name: "誰でも active な shop に review できる", viewer: bob, shop: activeShop, want: true},
		{name: "creator は自分の pending な shop に review できる", viewer: alice, shop: pendingOwn, want: true},
		{name: "他のユーザーは pending な shop に review できない", viewer: bob, shop: pendingOwn, want: false},
		{name: "admin はどの pending な shop にも review できる", viewer: admin, shop: pendingOwn, want: true},
		{name: "creator のない pending な shop には一般ユーザーは review できない", viewer: bob, shop: pendingNoCreator, want: false},
		{name: "rejected な shop は creator であっても決して review できない", viewer: alice, shop: rejectedOwn, want: false},
		{name: "rejected な shop は admin であっても決して review できない", viewer: admin, shop: rejectedOwn, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shop.CanBeReviewedBy(tt.viewer); got != tt.want {
				t.Errorf("CanBeReviewedBy(%+v) = %v, want %v", tt.viewer, got, tt.want)
			}
		})
	}
}

// TestNewShopSubmission は投稿用のコンストラクタを固定する。有効な名前は、
// creator が記録され moderation note のない pending な shop を返す。空および
// ホワイトスペースのみの名前は、Rails のメッセージそのままで失敗する。
func TestNewShopSubmission(t *testing.T) {
	t.Run("有効な名前なら creator 付きの pending な shop になる", func(t *testing.T) {
		shop, err := domain.NewShopSubmission("New Shack", 7)
		if err != nil {
			t.Fatalf("NewShopSubmission returned error: %v", err)
		}
		if shop.Name != "New Shack" || shop.Status != domain.ShopStatusPending {
			t.Errorf("shop = %+v, want name New Shack, status pending", shop)
		}
		if shop.ModerationNote != nil {
			t.Errorf("ModerationNote = %v, want nil", *shop.ModerationNote)
		}
		if shop.CreatorID == nil || *shop.CreatorID != 7 {
			t.Errorf("CreatorID = %v, want 7", shop.CreatorID)
		}
	})

	for _, name := range []string{"", "   ", "\t\n"} {
		t.Run("空または空白のみの名前 "+name+" は検証エラーになる", func(t *testing.T) {
			_, err := domain.NewShopSubmission(name, 7)
			var vErr *domain.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("error = %v, want *domain.ValidationError", err)
			}
			want := []string{"Name can't be blank"}
			if !reflect.DeepEqual(vErr.Messages, want) {
				t.Errorf("messages = %v, want %v", vErr.Messages, want)
			}
		})
	}
}

// TestShopModerationTransitions は Rails parity の state machine を固定する。
// approve と reject は現在のどの status からでも行える無条件の値遷移であり、
// approve は moderation note をクリアし、reject は置き換える。
func TestShopModerationTransitions(t *testing.T) {
	statuses := []domain.ShopStatus{
		domain.ShopStatusPending,
		domain.ShopStatusActive,
		domain.ShopStatusRejected, // rejected→active への再承認は許される
	}

	for _, from := range statuses {
		t.Run("approve は "+string(from)+" から active にして note を消す", func(t *testing.T) {
			shop := domain.Shop{ID: 1, Name: "Shack", Status: from, ModerationNote: ptr("old note")}
			got := shop.Approve()
			if got.Status != domain.ShopStatusActive {
				t.Errorf("status = %q, want %q", got.Status, domain.ShopStatusActive)
			}
			if got.ModerationNote != nil {
				t.Errorf("ModerationNote = %v, want nil", *got.ModerationNote)
			}
		})

		t.Run("reject は "+string(from)+" から rejected にして note を設定する", func(t *testing.T) {
			shop := domain.Shop{ID: 1, Name: "Shack", Status: from}
			got := shop.Reject(ptr("needs fixes"))
			if got.Status != domain.ShopStatusRejected {
				t.Errorf("status = %q, want %q", got.Status, domain.ShopStatusRejected)
			}
			if got.ModerationNote == nil || *got.ModerationNote != "needs fixes" {
				t.Errorf("ModerationNote = %v, want needs fixes", got.ModerationNote)
			}
		})
	}

	t.Run("note なしの reject は以前の note を消す", func(t *testing.T) {
		shop := domain.Shop{ID: 1, Status: domain.ShopStatusRejected, ModerationNote: ptr("old note")}
		if got := shop.Reject(nil); got.ModerationNote != nil {
			t.Errorf("ModerationNote = %v, want nil", *got.ModerationNote)
		}
	})

	t.Run("遷移しても可視性の整合性が保たれる", func(t *testing.T) {
		anon := domain.ShopVisibilityFor(nil)
		shop := domain.Shop{ID: 1, Status: domain.ShopStatusPending}
		if approved := shop.Approve(); !anon.CanView(approved) {
			t.Error("approved shop is not anonymously visible")
		}
		if rejected := shop.Approve().Reject(nil); anon.CanView(rejected) {
			t.Error("rejected shop is still anonymously visible")
		}
	})
}

// TestShopVisibilityFor は記述子そのものを固定する。repository がそれを SQL
// パラメータへ変換するためである。
func TestShopVisibilityFor(t *testing.T) {
	if vis := domain.ShopVisibilityFor(nil); vis.ViewAll || vis.ViewerID != nil {
		t.Errorf("anonymous descriptor = %+v, want zero", vis)
	}
	if vis := domain.ShopVisibilityFor(&domain.User{ID: 7}); vis.ViewAll || vis.ViewerID == nil || *vis.ViewerID != 7 {
		t.Errorf("user descriptor = %+v, want ViewerID=7", vis)
	}
	if vis := domain.ShopVisibilityFor(&domain.User{ID: 7, Admin: true}); !vis.ViewAll || vis.ViewerID != nil {
		t.Errorf("admin descriptor = %+v, want ViewAll", vis)
	}
}

// TestShopCanBeReviewedByViewer は、API の can_review の元になる値を固定する。
// 匿名（nil）は常に false で、それ以外は CanBeReviewedBy の規則に従う。
func TestShopCanBeReviewedByViewer(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}

	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	pendingOwn := domain.Shop{ID: 11, Status: domain.ShopStatusPending, CreatorID: ptr(alice.ID)}
	rejectedOwn := domain.Shop{ID: 13, Status: domain.ShopStatusRejected, CreatorID: ptr(alice.ID)}

	tests := []struct {
		name   string
		viewer *domain.User
		shop   domain.Shop
		want   bool
	}{
		{name: "匿名は active な shop にも review できない", viewer: nil, shop: activeShop, want: false},
		{name: "ログイン済みの一般ユーザーは active な shop に review できる", viewer: &bob, shop: activeShop, want: true},
		{name: "creator は自分の pending な shop に review できる", viewer: &alice, shop: pendingOwn, want: true},
		{name: "他のユーザーは pending な shop に review できない", viewer: &bob, shop: pendingOwn, want: false},
		{name: "admin は pending な shop に review できる", viewer: &admin, shop: pendingOwn, want: true},
		{name: "rejected な shop は creator でも review できない", viewer: &alice, shop: rejectedOwn, want: false},
		{name: "rejected な shop は admin でも review できない", viewer: &admin, shop: rejectedOwn, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shop.CanBeReviewedByViewer(tt.viewer); got != tt.want {
				t.Errorf("CanBeReviewedByViewer(%+v) = %v, want %v", tt.viewer, got, tt.want)
			}
		})
	}
}
