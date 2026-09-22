package query

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ShopStatsQuery は、sqlc が生成したクエリを使って usecase.ShopStatsQuery を実装する。ショップの集計を計算し
// 直すのに必要な元データ(レビュー)と、再計算の依頼の一覧の読み取りだけを担い、集計の値を計算する処理は持たない
// (domain の CalculateShopStat が行う)。db が UnitOfWork のトランザクションなら、同じトランザクションの、まだ
// 確定していない書き込みも読み取れる。
type ShopStatsQuery struct {
	q *sqlcgen.Queries
}

// NewShopStatsQuery は db をラップする。
func NewShopStatsQuery(db sqlcgen.DBTX) *ShopStatsQuery {
	return &ShopStatsQuery{q: sqlcgen.New(db)}
}

var _ usecase.ShopStatsQuery = (*ShopStatsQuery)(nil)

// ListShopReviewFacts は、ショップの集計の元になるレビュー(削除されていないレビューのうち、削除済みのユーザーが
// 書いたものを除く。ショップ詳細に出るレビューと同じ範囲)を、計算用の値(domain.ShopReviewFact)にして返す。
// バーガーは複数のショップにありうるので、ショップのすべてのバーガーのレビューをまとめて返す。それぞれの値には、
// そのレビューの投稿者が、すべてのバーガーに付けた有効な評価(投稿者の信頼度の計算に使う履歴)を添える。
func (r *ShopStatsQuery) ListShopReviewFacts(ctx context.Context, shopID string) ([]domain.ShopReviewFact, error) {
	rows, err := r.q.ListShopReviewFacts(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("list shop review facts: %w", err)
	}
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	histories, err := loadReviewerHistories(ctx, r.q, userIDs)
	if err != nil {
		return nil, fmt.Errorf("list shop review facts: %w", err)
	}
	facts := make([]domain.ShopReviewFact, 0, len(rows))
	for _, row := range rows {
		fact := domain.ShopReviewFact{
			ID:              row.ID,
			Rating:          float64(row.Rating),
			CreatedAt:       row.CreatedAt.Time,
			ReviewerHistory: histories[row.UserID],
		}
		if row.PhotoKey.Valid {
			key := row.PhotoKey.String
			fact.PhotoKey = &key
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// ListBurgerShopIDs は、バーガーが紐づくショップの id を、重複なしで昇順に返す。昇順にそろえるのは、複数のショップを
// 続けて扱う処理が、別の処理と逆の順序になって互いを待ち合わない(デッドロックしない)ようにするため。並び順は
// SQL の ORDER BY が保証する。
func (r *ShopStatsQuery) ListBurgerShopIDs(ctx context.Context, burgerID string) ([]string, error) {
	ids, err := r.q.ListBurgerShopIDs(ctx, burgerID)
	if err != nil {
		return nil, fmt.Errorf("list burger shop ids: %w", err)
	}
	return ids, nil
}

// ListDueShopRecalcRequests は、再計算の時期が来ている依頼を、上限件数だけ返す。next_attempt_at がなしか now 以前で、
// 失敗の回数が maxAttempts に達していないものが対象である(達したものは打ち切りで、行は残るが、ここには現れない)。
func (r *ShopStatsQuery) ListDueShopRecalcRequests(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.ShopRecalcRequest, error) {
	rows, err := r.q.ListDueShopStatsRecalcRequests(ctx, sqlcgen.ListDueShopStatsRecalcRequestsParams{
		MaxAttempts: int32(maxAttempts),
		Now:         pgtype.Timestamptz{Time: now, Valid: true},
		Batch:       int32(batch),
	})
	if err != nil {
		return nil, fmt.Errorf("list due shop recalc requests: %w", err)
	}
	requests := make([]domain.ShopRecalcRequest, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, domain.ShopRecalcRequest{ShopID: row.ShopID, Version: row.Version, Attempts: int(row.Attempts)})
	}
	return requests, nil
}
