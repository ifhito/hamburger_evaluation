package domain_test

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func ptr[T any](v T) *T { return &v }

// TestShopVisibility pins the shop visibility rule in its single home:
// anonymous viewers see active shops only, regular users additionally see
// shops they created (any status), admins see everything.
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
		{name: "anonymous sees active", viewer: nil, shop: activeShop, want: true},
		{name: "anonymous cannot see pending", viewer: nil, shop: pendingOwn, want: false},
		{name: "anonymous cannot see rejected", viewer: nil, shop: rejectedNoCreator, want: false},
		{name: "user sees active", viewer: &alice, shop: activeShop, want: true},
		{name: "user sees own pending", viewer: &alice, shop: pendingOwn, want: true},
		{name: "user cannot see someone else's pending", viewer: &alice, shop: pendingOther, want: false},
		{name: "user cannot see creatorless rejected", viewer: &alice, shop: rejectedNoCreator, want: false},
		{name: "admin sees active", viewer: &admin, shop: activeShop, want: true},
		{name: "admin sees any pending", viewer: &admin, shop: pendingOther, want: true},
		{name: "admin sees rejected", viewer: &admin, shop: rejectedNoCreator, want: true},
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

// TestShopVisibilityFor pins the descriptor itself, since repositories
// translate it into SQL parameters.
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
