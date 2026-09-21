package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerStatRepository は、sqlc 生成のクエリ上で domain.BurgerStatRepository を実装する。
// burger の統計の書き込み（ロック・upsert・再計算の依頼の登録と削除）だけを担い、SQL の詳細を
// この境界の内側に留める。
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

// LockBurgerStat は burger の行をロックする（LockBurgerForStats。FOR NO KEY UPDATE。lost-update の
// 根拠と、レビューの書き込みを待たせない理由はそのクエリのコメントを参照）。トランザクションが
// すでに持つロックの再取得は no-op である。
func (r *BurgerStatRepository) LockBurgerStat(ctx context.Context, burgerID string) error {
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

// CreateBurgerStatRecalcRequest は、burger の統計の再計算を依頼する（MarkBurgerStatsDirty。
// すでに依頼があれば version を進めて、失敗の記録を消す）。
func (r *BurgerStatRepository) CreateBurgerStatRecalcRequest(ctx context.Context, burgerID string) error {
	if err := r.q.MarkBurgerStatsDirty(ctx, burgerID); err != nil {
		return fmt.Errorf("create burger stat recalc request: %w", err)
	}
	return nil
}

// DiscardBurgerStatRecalcRequest は、取り出したときの version と一致する依頼だけを消し、
// 消せたかどうかを返す。
func (r *BurgerStatRepository) DiscardBurgerStatRecalcRequest(ctx context.Context, burgerID string, version int64) (bool, error) {
	n, err := r.q.DeleteBurgerStatsDirtyIfVersion(ctx, sqlcgen.DeleteBurgerStatsDirtyIfVersionParams{
		BurgerID: burgerID,
		Version:  version,
	})
	if err != nil {
		return false, fmt.Errorf("discard burger stat recalc request: %w", err)
	}
	return n > 0, nil
}

// UpdateBurgerStatRecalcFailure は、取り出したときの version と一致する依頼だけに、再計算の失敗を
// 記録し、記録できたかどうかを返す。
func (r *BurgerStatRepository) UpdateBurgerStatRecalcFailure(ctx context.Context, burgerID string, version int64, failure domain.RecalcFailure) (bool, error) {
	n, err := r.q.RecordBurgerStatsDirtyFailure(ctx, sqlcgen.RecordBurgerStatsDirtyFailureParams{
		BurgerID:      burgerID,
		Version:       version,
		NextAttemptAt: pgtype.Timestamptz{Time: failure.NextAttemptAt, Valid: true},
		LastError:     pgtype.Text{String: failure.Reason, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("update burger stat recalc failure: %w", err)
	}
	return n > 0, nil
}
