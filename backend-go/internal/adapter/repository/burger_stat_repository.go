package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerStatRepository は、sqlc 生成のクエリ上で domain.BurgerStatRepository を実装する。
// burger の統計の書き込み（ロックと upsert）だけを担い、SQL の詳細をこの境界の内側に留める。
// 統計の計算は行わない（domain の CalculateBurgerStat）。統計の元になる facts の読み取りは
// adapter/query の BurgerStatsQuery が担う。
type BurgerStatRepository struct {
	q *sqlcgen.Queries
}

// NewBurgerStatRepository は db（UnitOfWork のトランザクション。テストでは接続）をラップする。
func NewBurgerStatRepository(db sqlcgen.DBTX) *BurgerStatRepository {
	return &BurgerStatRepository{q: sqlcgen.New(db)}
}

var _ domain.BurgerStatRepository = (*BurgerStatRepository)(nil)

// LockBurgerStat は burger の行を FOR UPDATE でロックする（LockBurgerForStats。lost-update の
// 根拠はそのクエリのコメントを参照）。トランザクションがすでに持つロックの再取得は no-op である。
func (r *BurgerStatRepository) LockBurgerStat(ctx context.Context, burgerID int64) error {
	if _, err := r.q.LockBurgerForStats(ctx, burgerID); err != nil {
		return fmt.Errorf("lock burger stat: %w", err)
	}
	return nil
}

// UpdateBurgerStat は burger_stats の行を stat の値で upsert する。
func (r *BurgerStatRepository) UpdateBurgerStat(ctx context.Context, stat domain.BurgerStat) error {
	if _, err := r.q.UpsertBurgerStats(ctx, sqlcgen.UpsertBurgerStatsParams{
		BurgerID:      stat.BurgerID,
		ReviewCount:   stat.ReviewCount,
		AverageRating: stat.AverageRating,
		WeightedScore: stat.WeightedScore,
		Confidence:    stat.Confidence,
		CalculatedAt:  pgtype.Timestamptz{Time: stat.CalculatedAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("update burger stat: %w", err)
	}
	return nil
}
