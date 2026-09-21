package dbtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// このファイルは、burger_stats の DB を使うテスト（adapter/uow など）に共通の、統計の読み戻しと、
// 保存された行と domain の計算の一致の確認を提供する。

// StoredBurgerStats は、テストで読み戻した burger_stats の行である。
type StoredBurgerStats struct {
	ReviewCount   int64
	AverageRating float64
	WeightedScore float64
	Confidence    float64
	CalculatedAt  time.Time
}

// FetchBurgerStats は burger_stats の行を直接読み取る。行が存在しない場合、
// ok は false になる。
func FetchBurgerStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID string) (StoredBurgerStats, bool) {
	t.Helper()
	var s StoredBurgerStats
	err := conn.QueryRow(ctx,
		`SELECT review_count, average_rating, weighted_score, confidence, calculated_at
		 FROM burger_stats WHERE burger_id = $1`, burgerID,
	).Scan(&s.ReviewCount, &s.AverageRating, &s.WeightedScore, &s.Confidence, &s.CalculatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredBurgerStats{}, false
	}
	if err != nil {
		t.Fatalf("fetch burger stats: %v", err)
	}
	return s, true
}

// keptReviewFacts は、burger の kept な review のうち kept な user のものを
// （repository が使うのと同じルールで）domain の fact として読み込み、
// 各 fact の author の、すべての burger にわたる kept な rating を
// reviewer の履歴として付ける。
func keptReviewFacts(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID string) []domain.ReviewFact {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT r.rating, r.created_at, r.user_id
		 FROM reviews r JOIN users u ON u.id = r.user_id
		 WHERE r.burger_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
		 ORDER BY r.id`, burgerID)
	if err != nil {
		t.Fatalf("query review facts: %v", err)
	}
	type factRow struct {
		rating    int16
		createdAt time.Time
		userID    string
	}
	var factRows []factRow
	for rows.Next() {
		var fr factRow
		if err := rows.Scan(&fr.rating, &fr.createdAt, &fr.userID); err != nil {
			t.Fatalf("scan review fact: %v", err)
		}
		factRows = append(factRows, fr)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate review facts: %v", err)
	}
	facts := make([]domain.ReviewFact, 0, len(factRows))
	for _, fr := range factRows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(fr.rating),
			CreatedAt:       fr.createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: KeptRatingsOf(ctx, t, conn, fr.userID)},
		})
	}
	return facts
}

// KeptRatingsOf は、user の、すべての burger にわたる kept な rating を id の
// 昇順で返す（reviewer-trust の履歴）。
func KeptRatingsOf(ctx context.Context, t *testing.T, conn *pgx.Conn, userID string) []float64 {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT rating FROM reviews WHERE user_id = $1 AND discarded_at IS NULL ORDER BY id`, userID)
	if err != nil {
		t.Fatalf("query reviewer history: %v", err)
	}
	var ratings []float64
	for rows.Next() {
		var rating int16
		if err := rows.Scan(&rating); err != nil {
			t.Fatalf("scan reviewer rating: %v", err)
		}
		ratings = append(ratings, float64(rating))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate reviewer history: %v", err)
	}
	return ratings
}

// RequireConsistentStats は、保存された burger_stats の行が存在し、保存された
// review の行と保存された calculated_at から domain の関数で再計算した結果と
// 完全に一致することをアサートし（repository が "now" を timestamptz の精度に
// 切り詰めるのは、まさにこれが往復しても一致するようにするためである）、その
// 行を返す。float は厳密に比較する：同じ入力を同じ純粋関数に通せば、同一の値に
// ならなければならない。
func RequireConsistentStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID string) StoredBurgerStats {
	t.Helper()
	got, ok := FetchBurgerStats(ctx, t, conn, burgerID)
	if !ok {
		t.Fatalf("burger %s has no burger_stats row, want one", burgerID)
	}
	facts := keptReviewFacts(ctx, t, conn, burgerID)
	score := domain.CalculateBurgerScore(facts, got.CalculatedAt)
	want := StoredBurgerStats{
		ReviewCount:   int64(len(facts)),
		AverageRating: domain.AverageRating(facts),
		WeightedScore: score.WeightedAverage,
		Confidence:    score.Confidence,
		CalculatedAt:  got.CalculatedAt,
	}
	if got != want {
		t.Fatalf("stored stats = %+v, want recomputed %+v", got, want)
	}
	return got
}
