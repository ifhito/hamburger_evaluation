package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerStatRepository は、sqlc が生成したクエリを使って domain.BurgerStatRepository を実装する。
// バーガーの統計に対する書き込み(行のロック、統計の登録・更新、再計算の依頼の登録と削除)だけを担い、SQL の詳細をこの層の
// 内側に留める。統計の値を計算する処理は持たない(domain の CalculateBurgerStat が行う)。統計の
// 元データの読み取りは、adapter/query の BurgerStatsQuery が担う。
type BurgerStatRepository struct {
	q *sqlcgen.Queries
}

// NewBurgerStatRepository は db をラップする。本番では UnitOfWork のトランザクションが渡される
// (統計の書き込みは、レビューの書き込みと同じトランザクションで行うため)。UnitOfWork は、
// まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組みである。
func NewBurgerStatRepository(db sqlcgen.DBTX) *BurgerStatRepository {
	return &BurgerStatRepository{q: sqlcgen.New(db)}
}

var _ domain.BurgerStatRepository = (*BurgerStatRepository)(nil)

// LockBurgerStat は、バーガーの行をロックする(FOR NO KEY UPDATE。sqlc のクエリ LockBurgerForStats)。
// 同じバーガーの統計を同時に計算し直す処理が、互いの追加分を知らないまま上書きして、更新を
// 取りこぼすのを防ぐ(詳しくは、そのクエリのコメントを参照)。トランザクションがすでに持っている
// ロックを取り直しても、待たされない。レビューの書き込みは待たせない(理由は、そのクエリのコメントを参照)。
func (r *BurgerStatRepository) LockBurgerStat(ctx context.Context, burgerID string) error {
	if _, err := r.q.LockBurgerForStats(ctx, burgerID); err != nil {
		return fmt.Errorf("lock burger stat: %w", err)
	}
	return nil
}

// UpdateBurgerStat は、バーガーの統計を保存する。統計の行がまだなければ追加し、あれば
// stat の値で上書きする。
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

// CreateBurgerStatRecalcRequest は、burger の統計の再計算を依頼する（UpsertBurgerStatsRecalcRequest。
// すでに依頼があれば version を進めて、失敗の記録を消す）。
func (r *BurgerStatRepository) CreateBurgerStatRecalcRequest(ctx context.Context, burgerID string) error {
	if err := r.q.UpsertBurgerStatsRecalcRequest(ctx, burgerID); err != nil {
		return fmt.Errorf("create burger stat recalc request: %w", err)
	}
	return nil
}

// DiscardBurgerStatRecalcRequest は、取り出したときの version と一致する依頼だけを消し、
// 消せたかどうかを返す。
func (r *BurgerStatRepository) DiscardBurgerStatRecalcRequest(ctx context.Context, burgerID string, version int64) (bool, error) {
	n, err := r.q.DeleteBurgerStatsRecalcRequestIfVersion(ctx, sqlcgen.DeleteBurgerStatsRecalcRequestIfVersionParams{
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
	n, err := r.q.RecordBurgerStatsRecalcRequestFailure(ctx, sqlcgen.RecordBurgerStatsRecalcRequestFailureParams{
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
