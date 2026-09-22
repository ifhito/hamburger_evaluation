package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

var shopStatsNow = time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)

func fact(id string, rating float64, age time.Duration, photo string, history ...float64) domain.ShopReviewFact {
	f := domain.ShopReviewFact{ID: id, Rating: rating, CreatedAt: shopStatsNow.Add(-age), ReviewerHistory: domain.ReviewerHistory{Ratings: history}}
	if photo != "" {
		f.PhotoKey = &photo
	}
	return f
}

const day = 24 * time.Hour

// veteran は、評価が 10 件以上あり、ばらつきのある投稿者の履歴である(信頼度 0.9)。
var veteran = []float64{1, 2, 3, 4, 5, 1, 2, 3, 4, 5}

func average(t *testing.T, facts ...domain.ShopReviewFact) float64 {
	t.Helper()
	stat := domain.CalculateShopStat("shop", facts, shopStatsNow)
	if stat.AverageRating == nil {
		t.Fatalf("AverageRating = nil, want a value")
	}
	return *stat.AverageRating
}

// TestCalculateShopStatWeightedAverage は、ショップの評価が、バーガーのスコアと同じ重み付け(投稿者の信頼度 ×
// 半減期 180 日の新しさ)の加重平均で、手で計算した値になることを確かめる。単純平均とは違う値になる場面を含める。
func TestCalculateShopStatWeightedAverage(t *testing.T) {
	t.Run("新しいレビューほど重い: 5 点(新しい・重み 0.5)と 1 点(180 日前・重み 0.25)の加重平均は (5×0.5+1×0.25)/0.75 = 3.67 → 3.7 で、単純平均 3.0 と違う", func(t *testing.T) {
		got := average(t, fact("a", 5, 0, ""), fact("b", 1, 180*day, ""))
		if got != 3.7 {
			t.Errorf("AverageRating = %v, want 3.7(単純平均なら 3.0)", got)
		}
	})

	t.Run("信頼度の高い投稿者ほど重い: 10 件以上のばらつきのある投稿者(0.9)の 5 点と、新規の投稿者(0.5)の 1 点は (5×0.9+1×0.5)/1.4 = 3.57 → 3.6 で、単純平均 3.0 と違う", func(t *testing.T) {
		got := average(t, fact("a", 5, 0, "", veteran...), fact("b", 1, 0, ""))
		if got != 3.6 {
			t.Errorf("AverageRating = %v, want 3.6(単純平均なら 3.0)", got)
		}
	})

	t.Run("評価が偏った(ばらつきの小さい)投稿者は、重みが 0.7 倍になる: 履歴 [3,4,5](信頼度 0.7)の 5 点は 3.3、履歴 [4,4,4](0.7×0.7=0.49)の 5 点は、新規の投稿者の 1 点との加重平均が 3.0 になる", func(t *testing.T) {
		if got := average(t, fact("a", 5, 0, "", 3, 4, 5), fact("b", 1, 0, "")); got != 3.3 {
			t.Errorf("ばらつきのある投稿者: AverageRating = %v, want 3.3((5×0.7+1×0.5)/1.2)", got)
		}
		if got := average(t, fact("a", 5, 0, "", 4, 4, 4), fact("b", 1, 0, "")); got != 3.0 {
			t.Errorf("偏った投稿者: AverageRating = %v, want 3.0((5×0.49+1×0.5)/0.99 = 2.98)", got)
		}
	})

	t.Run("全員が同じ重みなら、単純平均と同じになり、小数 1 桁に丸める(4.25 → 4.3)", func(t *testing.T) {
		got := average(t, fact("a", 4, 0, ""), fact("b", 4, 0, ""), fact("c", 4, 0, ""), fact("d", 5, 0, ""))
		if got != 4.3 {
			t.Errorf("AverageRating = %v, want 4.3", got)
		}
	})

	t.Run("レビューが 1 件なら、その評価がそのまま平均になる", func(t *testing.T) {
		if got := average(t, fact("a", 2, 400*day, "")); got != 2 {
			t.Errorf("AverageRating = %v, want 2", got)
		}
	})

	t.Run("複数のバーガーのレビューも、1 つの集合として重みをかける(facts の並びや、レビューの属するバーガーに依らない)", func(t *testing.T) {
		facts := []domain.ShopReviewFact{fact("a", 5, 0, "", veteran...), fact("b", 1, 180*day, ""), fact("c", 3, 30*day, "", 3, 4, 5)}
		reversed := []domain.ShopReviewFact{facts[2], facts[1], facts[0]}
		if a, b := average(t, facts...), average(t, reversed...); a != b {
			t.Errorf("並びを変えると平均が変わった: %v vs %v", a, b)
		}
	})

	t.Run("結果は、バーガーのスコアの計算器(加重平均を小数 2 桁に丸めたもの)と、丸めの差(0.05)の範囲で一致する", func(t *testing.T) {
		facts := []domain.ShopReviewFact{fact("a", 5, 0, "", veteran...), fact("b", 1, 180*day, ""), fact("c", 3, 30*day, "", 3, 4, 5), fact("d", 4, 400*day, "", 4, 4, 4)}
		reviewFacts := make([]domain.ReviewFact, 0, len(facts))
		for _, f := range facts {
			reviewFacts = append(reviewFacts, domain.ReviewFact{Rating: f.Rating, CreatedAt: f.CreatedAt, ReviewerHistory: f.ReviewerHistory})
		}
		want := domain.CalculateBurgerScore(reviewFacts, shopStatsNow).WeightedAverage
		if got := average(t, facts...); math.Abs(got-want) > 0.05+1e-9 {
			t.Errorf("AverageRating = %v, バーガーのスコアの計算器 = %v(差が 0.05 を超えている)", got, want)
		}
	})
}

func TestCalculateShopStat(t *testing.T) {
	t.Run("レビューがなければ、件数 0・平均と写真は nil", func(t *testing.T) {
		got := domain.CalculateShopStat("shop", nil, shopStatsNow)
		if got.ShopID != "shop" || got.ReviewCount != 0 || got.AverageRating != nil || got.PhotoKey != nil || !got.CalculatedAt.Equal(shopStatsNow) {
			t.Errorf("stat = %+v, want 空の集計(計算時刻 %v)", got, shopStatsNow)
		}
	})

	t.Run("件数は、重みに依らない単純な件数である", func(t *testing.T) {
		got := domain.CalculateShopStat("shop", []domain.ShopReviewFact{fact("a", 5, 0, ""), fact("b", 1, 900*day, ""), fact("c", 3, 30*day, "")}, shopStatsNow)
		if got.ReviewCount != 3 {
			t.Errorf("ReviewCount = %d, want 3", got.ReviewCount)
		}
	})

	t.Run("写真は、写真つきのレビューのうち最も新しいものを使い、写真のない新しいレビューは飛ばす", func(t *testing.T) {
		got := domain.CalculateShopStat("shop", []domain.ShopReviewFact{
			fact("a", 5, 10*day, "old.jpg"),
			fact("b", 4, 5*day, "newest.jpg"),
			fact("c", 3, 1*day, ""), // 最も新しいが、写真なし
		}, shopStatsNow)
		if got.PhotoKey == nil || *got.PhotoKey != "newest.jpg" {
			t.Errorf("PhotoKey = %v, want newest.jpg", got.PhotoKey)
		}
	})

	t.Run("写真つきのレビューが同じ時刻に 2 件あるときは、id が大きい方の写真を使う(並びに依らない)", func(t *testing.T) {
		low, high := fact("00000000-0000-4000-8000-000000000001", 4, day, "low.jpg"), fact("00000000-0000-4000-8000-000000000002", 4, day, "high.jpg")
		for _, facts := range [][]domain.ShopReviewFact{{low, high}, {high, low}} {
			if got := domain.CalculateShopStat("shop", facts, shopStatsNow); got.PhotoKey == nil || *got.PhotoKey != "high.jpg" {
				t.Errorf("PhotoKey = %v, want high.jpg", got.PhotoKey)
			}
		}
	})

	t.Run("写真つきのレビューがなければ、平均だけがあり、写真は nil になる", func(t *testing.T) {
		got := domain.CalculateShopStat("shop", []domain.ShopReviewFact{fact("a", 4, 0, ""), fact("b", 3, 0, "")}, shopStatsNow)
		if got.AverageRating == nil || got.PhotoKey != nil {
			t.Errorf("stat = %+v, want average and no photo", got)
		}
	})

	t.Run("写真のキーは、入力の値のコピーである(あとから入力を書き換えても、集計は変わらない)", func(t *testing.T) {
		facts := []domain.ShopReviewFact{fact("a", 4, 0, "p.jpg")}
		got := domain.CalculateShopStat("shop", facts, shopStatsNow)
		*facts[0].PhotoKey = "changed.jpg"
		if got.PhotoKey == nil || *got.PhotoKey != "p.jpg" {
			t.Errorf("PhotoKey = %v, want p.jpg", got.PhotoKey)
		}
	})
}
