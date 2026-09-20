package domain_test

import (
	"errors"
	"reflect"
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

// TestNewShopSubmission pins the submission constructor: a valid name
// yields a pending shop with the creator recorded and no moderation note;
// blank and whitespace-only names fail with the exact Rails message.
func TestNewShopSubmission(t *testing.T) {
	t.Run("valid name starts pending with creator", func(t *testing.T) {
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
		t.Run("blank name "+name+" fails validation", func(t *testing.T) {
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

// TestShopModerationTransitions pins the Rails-parity state machine:
// approve and reject are unconditional value transitions from any current
// status; approve clears the moderation note, reject replaces it.
func TestShopModerationTransitions(t *testing.T) {
	statuses := []domain.ShopStatus{
		domain.ShopStatusPending,
		domain.ShopStatusActive,
		domain.ShopStatusRejected, // rejected→active re-approval is allowed
	}

	for _, from := range statuses {
		t.Run("approve from "+string(from)+" activates and clears note", func(t *testing.T) {
			shop := domain.Shop{ID: 1, Name: "Shack", Status: from, ModerationNote: ptr("old note")}
			got := shop.Approve()
			if got.Status != domain.ShopStatusActive {
				t.Errorf("status = %q, want %q", got.Status, domain.ShopStatusActive)
			}
			if got.ModerationNote != nil {
				t.Errorf("ModerationNote = %v, want nil", *got.ModerationNote)
			}
		})

		t.Run("reject from "+string(from)+" sets status and note", func(t *testing.T) {
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

	t.Run("reject without note clears any previous note", func(t *testing.T) {
		shop := domain.Shop{ID: 1, Status: domain.ShopStatusRejected, ModerationNote: ptr("old note")}
		if got := shop.Reject(nil); got.ModerationNote != nil {
			t.Errorf("ModerationNote = %v, want nil", *got.ModerationNote)
		}
	})

	t.Run("transitions keep visibility coherent", func(t *testing.T) {
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
