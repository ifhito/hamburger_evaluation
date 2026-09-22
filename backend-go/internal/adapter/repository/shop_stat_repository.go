package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopStatRepository は、sqlc が生成したクエリを使って domain.ShopStatRepository を実装する。
// ショップの集計に対する書き込み(行のロック、集計の登録・更新、再計算の依頼の登録と削除)だけを担い、SQL の詳細を
// この層の内側に留める。集計の値を計算する処理は持たない(domain の CalculateShopStat が行う)。集計の
// 元データの読み取りは、adapter/query の ShopStatsQuery が担う。
type ShopStatRepository struct {
	q *sqlcgen.Queries
}

// NewShopStatRepository は db をラップする。本番では UnitOfWork のトランザクションが渡される(集計の再計算を、
// バーガーの統計の再計算と同じトランザクションで依頼するため)。
func NewShopStatRepository(db sqlcgen.DBTX) *ShopStatRepository {
	return &ShopStatRepository{q: sqlcgen.New(db)}
}

var _ domain.ShopStatRepository = (*ShopStatRepository)(nil)

// LockShopStat は、ショップの行をロックする(FOR NO KEY UPDATE。sqlc のクエリ LockShopForStats)。ショップが
// ないときは、wrap された domain.ErrShopNotFound を返す。
func (r *ShopStatRepository) LockShopStat(ctx context.Context, shopID string) error {
	if _, err := r.q.LockShopForStats(ctx, shopID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock shop stat: %w", domain.ErrShopNotFound)
		}
		return fmt.Errorf("lock shop stat: %w", err)
	}
	return nil
}

// UpdateShopStat は、ショップの集計を保存する。行がまだなければ追加し、あれば stat の値で上書きする。
func (r *ShopStatRepository) UpdateShopStat(ctx context.Context, stat domain.ShopStat) error {
	params := sqlcgen.UpsertShopStatsParams{
		ShopID:       stat.ShopID,
		ReviewCount:  stat.ReviewCount,
		CalculatedAt: pgtype.Timestamptz{Time: stat.CalculatedAt, Valid: true},
	}
	if stat.AverageRating != nil {
		params.AverageRating = pgtype.Float8{Float64: *stat.AverageRating, Valid: true}
	}
	if stat.PhotoKey != nil {
		params.PhotoKey = pgtype.Text{String: *stat.PhotoKey, Valid: true}
	}
	if err := r.q.UpsertShopStats(ctx, params); err != nil {
		return fmt.Errorf("update shop stat: %w", err)
	}
	return nil
}

// CreateShopStatRecalcRequest は、shop の集計の再計算を依頼する(UpsertShopStatsRecalcRequest。すでに依頼が
// あれば version を進めて、失敗の記録を消す)。
func (r *ShopStatRepository) CreateShopStatRecalcRequest(ctx context.Context, shopID string) error {
	if err := r.q.UpsertShopStatsRecalcRequest(ctx, shopID); err != nil {
		return fmt.Errorf("create shop stat recalc request: %w", err)
	}
	return nil
}

// DiscardShopStatRecalcRequest は、取り出したときの version と一致する依頼だけを消し、消せたかどうかを返す。
func (r *ShopStatRepository) DiscardShopStatRecalcRequest(ctx context.Context, shopID string, version int64) (bool, error) {
	n, err := r.q.DeleteShopStatsRecalcRequestIfVersion(ctx, sqlcgen.DeleteShopStatsRecalcRequestIfVersionParams{
		ShopID:  shopID,
		Version: version,
	})
	if err != nil {
		return false, fmt.Errorf("discard shop stat recalc request: %w", err)
	}
	return n > 0, nil
}

// UpdateShopStatRecalcFailure は、取り出したときの version と一致する依頼だけに、再計算の失敗を記録し、
// 記録できたかどうかを返す。
func (r *ShopStatRepository) UpdateShopStatRecalcFailure(ctx context.Context, shopID string, version int64, failure domain.RecalcFailure) (bool, error) {
	n, err := r.q.RecordShopStatsRecalcRequestFailure(ctx, sqlcgen.RecordShopStatsRecalcRequestFailureParams{
		ShopID:        shopID,
		Version:       version,
		NextAttemptAt: pgtype.Timestamptz{Time: failure.NextAttemptAt, Valid: true},
		LastError:     pgtype.Text{String: failure.Reason, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("update shop stat recalc failure: %w", err)
	}
	return n > 0, nil
}
