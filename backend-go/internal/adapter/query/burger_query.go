package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// BurgerQuery は、sqlc が生成したクエリを使って usecase.BurgerQuery を実装する。
type BurgerQuery struct {
	q *sqlcgen.Queries
}

// NewBurgerQuery は db（通常は共有の pgx pool）をラップする。
func NewBurgerQuery(db sqlcgen.DBTX) *BurgerQuery {
	return &BurgerQuery{q: sqlcgen.New(db)}
}

var _ usecase.BurgerQuery = (*BurgerQuery)(nil)

// GetBurgerWithStats は burger 1 件を統計つきで返す。レビューが1件もない(統計行が未計算、または
// 削除で 0 件に戻った)ときは、review_count/average_rating/weighted_score をすべて nil にする
// (実際の値 0 と区別するため)。存在しない id は domain.ErrBurgerNotFound を返す。
func (r *BurgerQuery) GetBurgerWithStats(ctx context.Context, id string) (domain.BurgerDetail, error) {
	row, err := r.q.GetBurgerWithStats(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.BurgerDetail{}, fmt.Errorf("get burger: %w", domain.ErrBurgerNotFound)
		}
		return domain.BurgerDetail{}, fmt.Errorf("get burger: %w", err)
	}
	detail := domain.BurgerDetail{ID: row.ID, Name: row.Name}
	if row.ReviewCount.Valid && row.ReviewCount.Int64 > 0 {
		count := row.ReviewCount.Int64
		avg := row.AverageRating.Float64
		weighted := row.WeightedScore.Float64
		detail.ReviewCount = &count
		detail.AverageRating = &avg
		detail.WeightedScore = &weighted
	}
	return detail, nil
}

// ListBurgerShops は burger に紐づく shop(全カラム。可視性フィルタは usecase が行う)を作成の古い順で返す。
func (r *BurgerQuery) ListBurgerShops(ctx context.Context, burgerID string) ([]domain.Shop, error) {
	rows, err := r.q.ListBurgerShops(ctx, burgerID)
	if err != nil {
		return nil, fmt.Errorf("list burger shops: %w", err)
	}
	shops := make([]domain.Shop, 0, len(rows))
	for _, row := range rows {
		shop, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list burger shops: %w", err)
		}
		shops = append(shops, shop)
	}
	return shops, nil
}
