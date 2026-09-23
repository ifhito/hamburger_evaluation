package usecase

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerQuery は burger ランキング向けの consumer 側の読み取りの契約である。
type BurgerQuery interface {
	// ListBurgerRankings は、review が 1 件以上ある(burger_stats を持つ)burger を、
	// weighted_score の降順(同値は id の昇順)で返す。review が無い burger は対象外。
	// 2 つ目の戻り値は、offset+limit 件より後ろにも一致する burger があるか(has_more)で、
	// 実装は limit+1 件を取得して判定する。
	ListBurgerRankings(ctx context.Context, limit, offset int32) ([]domain.BurgerRanking, bool, error)
}

// Burgers は GET /burgers の use case を実装する。書き込みはなく、読み取りは query だけを
// 通す(repository には依存しない)。
type Burgers struct {
	query BurgerQuery
}

// NewBurgers は query を使う Burgers を返す。
func NewBurgers(query BurgerQuery) *Burgers {
	return &Burgers{query: query}
}

// List は、weighted_score の高い順に burger を返す。範囲外の page/perPage は clampPage の
// 規則で補正される。2 つ目の戻り値は、次のページがあるか(has_more)である。
func (b *Burgers) List(ctx context.Context, page, perPage int) ([]domain.BurgerRanking, bool, error) {
	limit, offset := clampPage(page, perPage)
	rankings, hasMore, err := b.query.ListBurgerRankings(ctx, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list burger rankings: %w", err)
	}
	return rankings, hasMore, nil
}
