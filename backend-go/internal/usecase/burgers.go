package usecase

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerQuery は burger 向けの consumer 側の読み取りの契約である。読み取り専用。
type BurgerQuery interface {
	// GetBurgerWithStats は burger 1 件を、保存された統計(まだレビューが1件もなければ nil)とともに返す。
	// 一致する行がなければ(wrap された)domain.ErrBurgerNotFound を返す。
	GetBurgerWithStats(ctx context.Context, id string) (domain.BurgerDetail, error)
	// ListBurgerShops は、burger に紐づく shop(shops_burgers 経由)を、作成の古い順ですべて返す
	// (viewer ごとの可視性フィルタは usecase が行う)。
	ListBurgerShops(ctx context.Context, burgerID string) ([]domain.Shop, error)
	// ListBurgerRankings は、review が 1 件以上ある(burger_stats を持つ)burger を、
	// weighted_score の降順(同値は id の昇順)で返す。review が無い burger は対象外。
	// 2 つ目の戻り値は、offset+limit 件より後ろにも一致する burger があるか(has_more)で、
	// 実装は limit+1 件を取得して判定する。
	ListBurgerRankings(ctx context.Context, limit, offset int32) ([]domain.BurgerRanking, bool, error)
}

// Burgers は burger の use case を実装する: 1 件の詳細(GET /burgers/{id})と、weighted_score
// 順のランキング一覧(GET /burgers)。読み取りは query だけを通す(repository には依存しない。
// burger は非ゴールにより独自の書き込みアグリゲートを持たない)。
type Burgers struct {
	query  BurgerQuery
	photos PhotoURLs
}

// NewBurgers は query を使う Burgers を返す。
func NewBurgers(query BurgerQuery, photos PhotoURLs) *Burgers {
	if photos == nil {
		panic("usecase.NewBurgers: nil PhotoURLs")
	}
	return &Burgers{query: query, photos: photos}
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
	detail.PhotoURL = b.photoURL(detail.VisiblePhotoKey())
	return detail, nil
}

// List は、weighted_score の高い順に burger を返す。範囲外の page/perPage は clampPage の
// 規則で補正される。2 つ目の戻り値は、次のページがあるか(has_more)である。
func (b *Burgers) List(ctx context.Context, page, perPage int) ([]domain.BurgerRanking, bool, error) {
	limit, offset := clampPage(page, perPage)
	rankings, hasMore, err := b.query.ListBurgerRankings(ctx, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list burger rankings: %w", err)
	}
	for i := range rankings {
		rankings[i].PhotoURL = b.photoURL(rankings[i].PhotoKey)
	}
	return rankings, hasMore, nil
}

// photoURL は保存キーを公開 URL に変換する。写真がないときは nil。
func (b *Burgers) photoURL(key *string) *string {
	if key == nil {
		return nil
	}
	url := b.photos.URL(*key)
	return &url
}
