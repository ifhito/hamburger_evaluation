package usecase

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerQuery は burger 詳細向けの consumer 側の読み取りの契約である。読み取り専用。
type BurgerQuery interface {
	// GetBurgerWithStats は burger 1 件を、保存された統計(まだレビューが1件もなければ nil)とともに返す。
	// 一致する行がなければ(wrap された)domain.ErrBurgerNotFound を返す。
	GetBurgerWithStats(ctx context.Context, id string) (domain.BurgerDetail, error)
	// ListBurgerShops は、burger に紐づく shop(shops_burgers 経由)を、作成の古い順ですべて返す
	// (viewer ごとの可視性フィルタは usecase が行う)。
	ListBurgerShops(ctx context.Context, burgerID string) ([]domain.Shop, error)
}

// Burgers は burger 詳細の use case を実装する。読み取りは query だけを通す。
type Burgers struct {
	query BurgerQuery
}

// NewBurgers は query を使う Burgers を返す。
func NewBurgers(query BurgerQuery) *Burgers {
	return &Burgers{query: query}
}

// Get は burger の詳細を返す。存在しない id は domain.ErrBurgerNotFound。Shops は、viewer(nil = 匿名)に
// 見えるショップだけに絞り込む(domain.ShopVisibilityFor.CanView。pending/rejected な shop を、それを見る権限の
// ない viewer に漏らさないため。review 詳細の ReviewShopFor と同じ考え方)。
func (b *Burgers) Get(ctx context.Context, viewer *domain.User, id string) (domain.BurgerDetail, error) {
	detail, err := b.query.GetBurgerWithStats(ctx, id)
	if err != nil {
		return domain.BurgerDetail{}, fmt.Errorf("get burger: %w", err)
	}
	shops, err := b.query.ListBurgerShops(ctx, id)
	if err != nil {
		return domain.BurgerDetail{}, fmt.Errorf("get burger: %w", err)
	}
	vis := domain.ShopVisibilityFor(viewer)
	visible := make([]domain.ShopRef, 0, len(shops))
	for _, shop := range shops {
		if vis.CanView(shop) {
			visible = append(visible, domain.ShopRef{ID: shop.ID, Name: shop.Name})
		}
	}
	detail.Shops = visible
	return detail, nil
}
