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

// BurgerStatsQuery は、sqlc が生成したクエリを使って usecase.BurgerStatsQuery を実装する。
// 統計を計算し直すのに必要な元データの読み取りだけを担い、統計の値を計算する処理は持たない
// (domain の CalculateBurgerStat が行う)。db が UnitOfWork(まとめて 1 つのトランザクションにする範囲)の
// トランザクションなら、同じトランザクションの、まだ確定していない書き込みも読み取れる。
type BurgerStatsQuery struct {
	q *sqlcgen.Queries
}

// NewBurgerStatsQuery は db をラップする。本番では UnitOfWork のトランザクションが渡される
// (直前のレビューの書き込みを、統計の元データに反映させるため)。UnitOfWork は、まとめて 1 つの
// トランザクションにする範囲を、usecase が指定する仕組みである。
func NewBurgerStatsQuery(db sqlcgen.DBTX) *BurgerStatsQuery {
	return &BurgerStatsQuery{q: sqlcgen.New(db)}
}

var _ usecase.BurgerStatsQuery = (*BurgerStatsQuery)(nil)

// ListBurgerReviewFacts は、バーガーの統計の元になるレビュー(削除されていないレビューのうち、
// 削除済みのユーザーが書いたものを除く)を、計算用の値(domain.ReviewFact)にして返す。それぞれの
// 値には、そのレビューの投稿者が、すべてのバーガーに付けた有効な評価を、投稿者の信頼度を計算する
// ための履歴として添える。
func (r *BurgerStatsQuery) ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error) {
	rows, err := r.q.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return nil, fmt.Errorf("list burger review facts: %w", err)
	}
	// 同じ投稿者のレビューが複数あっても、履歴の読み取りは投稿者ごとに 1 回で済ませる。まず投稿者の
	// 一覧(重複なし)を作り、まとめて読んだ評価を投稿者ごとに振り分ける。
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

// ListReviewedBurgerIDsByUser は、ユーザーの有効なレビューが付いているバーガーの ID を、重複なしで
// 昇順に返す。昇順にそろえるのは、複数のバーガーを続けてロックする再計算が、別の処理と逆の順序に
// なって互いを待ち合わない(デッドロックしない)ようにするため。並び順は SQL の ORDER BY が保証する。
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
	rows, err := r.q.ListDueBurgerStatsRecalcRequests(ctx, sqlcgen.ListDueBurgerStatsRecalcRequestsParams{
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
