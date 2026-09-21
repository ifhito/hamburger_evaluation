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

// BurgerStatsQuery は、sqlc 生成のクエリ上で usecase.BurgerStatsQuery を実装する。
// 統計の再計算に必要な facts の読み取りだけを担い、統計の計算は行わない（domain の
// CalculateBurgerStat）。db が UnitOfWork のトランザクションなら、同じトランザクションの
// 未コミットの書き込みが見える。
type BurgerStatsQuery struct {
	q *sqlcgen.Queries
}

// NewBurgerStatsQuery は db（通常は UnitOfWork のトランザクション）をラップする。
func NewBurgerStatsQuery(db sqlcgen.DBTX) *BurgerStatsQuery {
	return &BurgerStatsQuery{q: sqlcgen.New(db)}
}

var _ usecase.BurgerStatsQuery = (*BurgerStatsQuery)(nil)

// ListBurgerReviewFacts は、burger の kept な review（author が discard 済みの user である
// ものを除く）を facts として返す。各 fact には、その author が、すべての burger にわたって
// つけた kept な rating（reviewer の履歴）を付ける。
func (r *BurgerStatsQuery) ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error) {
	rows, err := r.q.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return nil, fmt.Errorf("list burger review facts: %w", err)
	}
	// 重複を除いた fact の author の reviewer-trust の履歴：各 author の、
	// すべての burger にわたる kept な rating を user ごとにまとめたもの。
	historyByUser := make(map[string][]float64, len(rows))
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, seen := historyByUser[row.UserID]; !seen {
			historyByUser[row.UserID] = nil
			userIDs = append(userIDs, row.UserID)
		}
	}
	if len(userIDs) > 0 {
		ratings, err := r.q.ListReviewerRatings(ctx, userIDs)
		if err != nil {
			return nil, fmt.Errorf("list burger review facts: list reviewer ratings: %w", err)
		}
		for _, rating := range ratings {
			historyByUser[rating.UserID] = append(historyByUser[rating.UserID], float64(rating.Rating))
		}
	}
	facts := make([]domain.ReviewFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(row.Rating),
			CreatedAt:       row.CreatedAt.Time,
			ReviewerHistory: domain.ReviewerHistory{Ratings: historyByUser[row.UserID]},
		})
	}
	return facts, nil
}

// ListReviewedBurgerIDsByUser は、user の kept な review が付く burger の id を、重複なしで
// burger_id の昇順に返す。昇順は、複数の burger をロックする再計算がデッドロックしないための
// 規約で、SQL の ORDER BY が保証する。
func (r *BurgerStatsQuery) ListReviewedBurgerIDsByUser(ctx context.Context, userID string) ([]string, error) {
	ids, err := r.q.ListUserKeptReviewBurgerIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list reviewed burger ids by user: %w", err)
	}
	return ids, nil
}

// ListDueRecalcRequests は、再計算の時期が来ている依頼を、上限件数だけ返す。next_attempt_at が
// なしか now 以前で、失敗の回数が maxAttempts に達していないものが対象である(達したものは
// 打ち切りで、行は残るが、ここには現れない)。再計算の時期が古い順(すぐのものが先)に並べる。
func (r *BurgerStatsQuery) ListDueRecalcRequests(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error) {
	rows, err := r.q.ListDueBurgerStatsDirty(ctx, sqlcgen.ListDueBurgerStatsDirtyParams{
		MaxAttempts: int32(maxAttempts),
		Now:         pgtype.Timestamptz{Time: now, Valid: true},
		Batch:       int32(batch),
	})
	if err != nil {
		return nil, fmt.Errorf("list due recalc requests: %w", err)
	}
	requests := make([]domain.RecalcRequest, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, domain.RecalcRequest{BurgerID: row.BurgerID, Version: row.Version, Attempts: int(row.Attempts)})
	}
	return requests, nil
}
