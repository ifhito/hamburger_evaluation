package query

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// BurgerQuery は、sqlc 生成のクエリ上で usecase.BurgerQuery を実装する。
type BurgerQuery struct {
	q *sqlcgen.Queries
}

// NewBurgerQuery は db(通常は共有の pgx pool)をラップする。
func NewBurgerQuery(db sqlcgen.DBTX) *BurgerQuery {
	return &BurgerQuery{q: sqlcgen.New(db)}
}

var _ usecase.BurgerQuery = (*BurgerQuery)(nil)

// ListBurgerRankings は、weighted_score の降順(同値は id の昇順)で burger を返す。
// review が無い(burger_stats がない)burger と、active な shop に 1 つも紐づかない burger は
// 対象外(SQL 側の JOIN で除外する)。次のページの有無を知るために limit+1 件を取得し、
// limit 件に切り詰めて返す。
func (r *BurgerQuery) ListBurgerRankings(ctx context.Context, limit, offset int32) ([]domain.BurgerRanking, bool, error) {
	rows, err := r.q.ListBurgerRankings(ctx, sqlcgen.ListBurgerRankingsParams{
		PageLimit:  limit + 1,
		PageOffset: offset,
	})
	if err != nil {
		return nil, false, fmt.Errorf("list burger rankings: %w", err)
	}
	rows, hasMore := trimPage(rows, limit)
	rankings := make([]domain.BurgerRanking, 0, len(rows))
	for _, row := range rows {
		rankings = append(rankings, domain.BurgerRanking{
			ID:            row.ID,
			Name:          row.Name,
			Shop:          domain.ShopRef{ID: row.ShopID, Name: row.ShopName},
			AverageRating: row.AverageRating,
			WeightedScore: row.WeightedScore,
			ReviewCount:   row.ReviewCount,
		})
	}
	return rankings, hasMore, nil
}
