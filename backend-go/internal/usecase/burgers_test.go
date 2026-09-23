package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeBurgerQuery は、手書きの usecase.BurgerQuery の test double である。
type fakeBurgerQuery struct {
	detail    domain.BurgerDetail
	detailErr error
	shops     []domain.Shop
}

func (f *fakeBurgerQuery) GetBurgerWithStats(context.Context, string) (domain.BurgerDetail, error) {
	if f.detailErr != nil {
		return domain.BurgerDetail{}, f.detailErr
	}
	return f.detail, nil
}

func (f *fakeBurgerQuery) ListBurgerShops(context.Context, string) ([]domain.Shop, error) {
	return f.shops, nil
}

var _ usecase.BurgerQuery = (*fakeBurgerQuery)(nil)

// TestBurgersGetNotFound は、存在しない burger の id で domain.ErrBurgerNotFound が
// そのまま(wrap されて)伝播することを確かめる。
func TestBurgersGetNotFound(t *testing.T) {
	query := &fakeBurgerQuery{detailErr: domain.ErrBurgerNotFound}
	_, err := usecase.NewBurgers(query).Get(context.Background(), nil, uid.N(1))
	if !errors.Is(err, domain.ErrBurgerNotFound) {
		t.Fatalf("error = %v, want %v", err, domain.ErrBurgerNotFound)
	}
}

// TestBurgersGetShopVisibility は、Shops が viewer ごとの可視性(domain.ShopVisibility)で
// 絞り込まれることを確かめる：active な shop は誰にでも見え、pending な shop はその creator と
// admin にだけ見え、匿名や無関係な他人には見えない。
func TestBurgersGetShopVisibility(t *testing.T) {
	creatorID := uid.N(1)
	otherID := uid.N(2)
	activeShop := domain.Shop{ID: uid.N(10), Name: "Active Diner", Status: domain.ShopStatusActive}
	pendingShop := domain.Shop{ID: uid.N(11), Name: "Pending Diner", Status: domain.ShopStatusPending, CreatorID: &creatorID}
	burgerID := uid.N(20)

	newQuery := func() *fakeBurgerQuery {
		return &fakeBurgerQuery{
			detail: domain.BurgerDetail{ID: burgerID, Name: "Cheese"},
			shops:  []domain.Shop{activeShop, pendingShop},
		}
	}

	tests := []struct {
		name   string
		viewer *domain.User
		want   []domain.ShopRef
	}{
		{
			name:   "匿名の viewer には active な shop だけが見える",
			viewer: nil,
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}},
		},
		{
			name:   "無関係な他人には active な shop だけが見える",
			viewer: &domain.User{ID: otherID},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}},
		},
		{
			name:   "pending な shop の creator には、pending な shop も追加で見える",
			viewer: &domain.User{ID: creatorID},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}, {ID: pendingShop.ID, Name: pendingShop.Name}},
		},
		{
			name:   "admin には、creator でなくても pending な shop が見える",
			viewer: &domain.User{ID: otherID, Admin: true},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}, {ID: pendingShop.ID, Name: pendingShop.Name}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, err := usecase.NewBurgers(newQuery()).Get(context.Background(), tt.viewer, burgerID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if !reflect.DeepEqual(detail.Shops, tt.want) {
				t.Errorf("Shops = %+v, want %+v", detail.Shops, tt.want)
			}
		})
	}
}
