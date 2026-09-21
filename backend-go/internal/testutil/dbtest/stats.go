package dbtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// このファイルは、バーガーの統計(burger_stats テーブル)を実データベースで確かめるテストが共通で
// 使う道具である。保存された統計の行を読み戻す関数と、その行が「保存されているレビューから、
// domain の計算で求め直した値」とちょうど一致するかを確かめる関数を提供する。

// StoredBurgerStats は、テストで読み戻した burger_stats の 1 行である。
type StoredBurgerStats struct {
	ReviewCount   int64
	AverageRating float64
	WeightedScore float64
	Confidence    float64
	CalculatedAt  time.Time
}

// FetchBurgerStats は、burger_stats の行を SQL で直接読み取る。行がなければ、2 つ目の戻り値が
// false になる。
func FetchBurgerStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) (StoredBurgerStats, bool) {
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

// keptReviewFacts は、バーガーの統計の元になるレビュー(削除されていないレビューのうち、削除
// されていないユーザーが書いたもの)を、本番の読み取りと同じ条件で SQL から読み、計算用の値
// (domain.ReviewFact)にして返す。各値には、そのレビューの投稿者が、すべてのバーガーに付けた
// 有効な評価を、投稿者の信頼度の計算に使う履歴として添える。本番の読み取りの実装を使わずに
// 期待値を作ることで、実装の取り違えを検算で見つけられるようにしている。
func keptReviewFacts(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) []domain.ReviewFact {
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

// KeptRatingsOf は、ユーザーがすべてのバーガーに付けた、削除されていない評価を、レビューの ID の
// 昇順で返す(投稿者の信頼度の計算に使う履歴)。
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

// RequireConsistentStats は、バーガーの統計の行が保存されていて、その値が「いま保存されている
// レビューと、行に保存された計算時刻(calculated_at)から、domain の関数で求め直した値」と
// ちょうど一致することを確かめ、その行を返す。行がなかったり、値が違っていたりすると、テストを
// 失敗させる。小数も厳密に比較する。同じ入力を同じ計算に通せば、まったく同じ値になるはずで、
// 再計算の時刻を保存の精度(マイクロ秒)に切り詰めているのも、この検算が一致するようにするため。
func RequireConsistentStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) StoredBurgerStats {
	t.Helper()
	got, ok := FetchBurgerStats(ctx, t, conn, burgerID)
	if !ok {
		t.Fatalf("バーガー %d の統計の行(burger_stats)がない", burgerID)
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
		t.Fatalf("保存された統計 = %+v, want 保存されているレビューから求め直した値 %+v", got, want)
	}
	return got
}
