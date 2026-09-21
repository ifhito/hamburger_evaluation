package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// 以下のケースは、Rails の spec
// backend/spec/domain/reviews/{burger_score_calculator_spec,
// reviewer_trust_evaluator_spec, burger_score_spec}.rb から移植したもので、
// 期待値は手計算で導出している（導出はコメントに記載）。アサーションには
// float の厳密な等値比較を使う。丸め前の期待値に float の積（例：0.7*0.7）が
// 関わる場合は、実装とまったく同じ float64 演算を通るように、期待値を
// float64 変数から実行時に計算する。

func repeatRatings(rating float64, count int) []float64 {
	ratings := make([]float64, count)
	for i := range ratings {
		ratings[i] = rating
	}
	return ratings
}

// TestBurgerStatsReviewerTrustScore は、ReviewerTrustScore を Rails の
// ReviewerTrustEvaluator の挙動に対して固定する。
func TestBurgerStatsReviewerTrustScore(t *testing.T) {
	// 0.7*0.7 を実行時の float64 の意味論で計算する（実装が生成する値と等しい。
	// 定数リテラル 0.49 は 1 ulp 離れている）。
	base, penalty := 0.7, 0.7
	regularPenalized := base * penalty

	tests := []struct {
		name    string
		ratings []float64
		want    float64
	}{
		// review なし：newcomer の base は 0.5、rating が 3 件未満なので
		// variance factor は 1.0 -> 0.5。
		{name: "履歴が空なら newcomer の 0.5 になる", ratings: nil, want: 0.5},
		// rating 3 件：regular の base は 0.7。平均 13/3、母分散は
		// (2*(1/3)^2 + (2/3)^2)/3 = 2/9 ~= 0.2222 < 0.3 -> penalty 0.7、
		// score は 0.7*0.7（~0.49）。
		{name: "分散が低い regular（4,4,5）は penalty がかかる", ratings: []float64{4, 4, 5}, want: regularPenalized},
		// 同一の rating 20 件：expert の base は 1.0、variance 0 < 0.3 ->
		// penalty 0.7、score は 1.0*0.7 = 厳密に 0.7。
		{name: "分散 0 の expert（20x4）は penalty がかかる", ratings: repeatRatings(4.0, 20), want: 0.7},
		// 同一の rating 5 件：regular の base は 0.7、variance 0 -> 0.7*0.7。
		{name: "分散 0 の regular（5x5）は penalty がかかる", ratings: repeatRatings(5.0, 5), want: regularPenalized},
		// 平均 3、variance (4+1+1+4+0)/5 = 2.0 >= 0.3 -> penalty なし、
		// regular は厳密に 0.7。
		{name: "rating がばらつく regular（1,2,4,5,3）は penalty がかからない", ratings: []float64{1, 2, 4, 5, 3}, want: 0.7},
		// 1 と 5 が交互に並ぶ rating 10 件：veteran の base は 0.9、平均 3、
		// variance 4.0 >= 0.3 -> 厳密に 0.9。
		{name: "rating がばらつく veteran は penalty がかからない", ratings: []float64{1, 5, 1, 5, 1, 5, 1, 5, 1, 5}, want: 0.9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.ReviewerTrustScore(domain.ReviewerHistory{Ratings: tt.ratings})
			if got != tt.want {
				t.Errorf("ReviewerTrustScore(%v) = %v, want %v", tt.ratings, got, tt.want)
			}
		})
	}
}

// TestBurgerStatsCalculateBurgerScore は、CalculateBurgerScore を Rails の
// BurgerScoreCalculator と BurgerScore の挙動に対して固定する。
func TestBurgerStatsCalculateBurgerScore(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

	t.Run("fact が空ならゼロスコアを返す", func(t *testing.T) {
		got := domain.CalculateBurgerScore(nil, now)
		want := domain.BurgerScore{WeightedAverage: 0.0, Confidence: 0.0, SampleSize: 0}
		if got != want {
			t.Errorf("CalculateBurgerScore(nil) = %+v, want %+v", got, want)
		}
	})

	t.Run("fresh な fact 1 件のスコアを計算する", func(t *testing.T) {
		// history [5] の trust：newcomer で 0.5、rating が 3 件未満なので
		// variance factor はない。now における recency は exp(0) = 1 で、
		// weight は 0.5。
		// WeightedAverage = 5*0.5/0.5 = 5.0。
		// Confidence = min(1/10,1)*0.6 + min(0.5/1,1)*0.4
		//            = 0.06 + 0.2 = 0.26（小数 4 桁に丸める）。
		facts := []domain.ReviewFact{{
			Rating:          5,
			CreatedAt:       now,
			ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{5}},
		}}
		got := domain.CalculateBurgerScore(facts, now)
		want := domain.BurgerScore{WeightedAverage: 5.0, Confidence: 0.26, SampleSize: 1}
		if got != want {
			t.Errorf("CalculateBurgerScore = %+v, want %+v", got, want)
		}
	})

	t.Run("expert は newcomer より weight が大きい", func(t *testing.T) {
		// newcomer の fact：rating 1、history [1] -> trust 0.5。
		// expert の fact：rating 5、history 20x3.0 + [5.0]（rating 21 件 ->
		// expert の base 1.0）。平均 65/21、variance は
		// (20*(2/21)^2 + (40/21)^2)/21 = 1680/9261 ~= 0.1814 < 0.3 ->
		// penalty 0.7、trust 0.7。どちらも now に作成されているので、weight は
		// 0.5 と 0.7。
		// WeightedAverage = (1*0.5 + 5*0.7)/(0.5+0.7) = 4.0/1.2
		//                 = 3.3333... -> 3.33。
		// Confidence = min(2/10,1)*0.6 + min(1.2/2,1)*0.4
		//            = 0.12 + 0.24 = 0.36。
		expertHistory := append(repeatRatings(3.0, 20), 5.0)
		facts := []domain.ReviewFact{
			{Rating: 1, CreatedAt: now, ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{1}}},
			{Rating: 5, CreatedAt: now, ReviewerHistory: domain.ReviewerHistory{Ratings: expertHistory}},
		}
		got := domain.CalculateBurgerScore(facts, now)
		want := domain.BurgerScore{WeightedAverage: 3.33, Confidence: 0.36, SampleSize: 2}
		if got != want {
			t.Errorf("CalculateBurgerScore = %+v, want %+v", got, want)
		}
	})

	t.Run("180 日経つと recency で weight が半分になる", func(t *testing.T) {
		// history 10x1.0 + 10x5.0：rating 20 件 -> expert の base 1.0、平均 3、
		// variance 4.0 >= 0.3 -> trust は厳密に 1.0。
		trustedHistory := append(repeatRatings(1.0, 10), repeatRatings(5.0, 10)...)
		fresh := domain.ReviewFact{
			Rating:          4,
			CreatedAt:       now,
			ReviewerHistory: domain.ReviewerHistory{Ratings: trustedHistory},
		}
		aged := fresh
		aged.CreatedAt = now.Add(-180 * 24 * time.Hour)

		// fresh：weight 1.0 -> Confidence = 0.1*0.6 + 1.0*0.4 = 0.46。
		gotFresh := domain.CalculateBurgerScore([]domain.ReviewFact{fresh}, now)
		wantFresh := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 0.46, SampleSize: 1}
		if gotFresh != wantFresh {
			t.Errorf("fresh score = %+v, want %+v", gotFresh, wantFresh)
		}

		// 180 日 = 半減期 1 回分：weight = exp(-180*ln2/180) = 0.5 ->
		// Confidence = 0.1*0.6 + 0.5*0.4 = 0.26。単一の fact の weighted
		// average は recency では変わらない（weight が相殺される）：4.0。
		gotAged := domain.CalculateBurgerScore([]domain.ReviewFact{aged}, now)
		wantAged := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 0.26, SampleSize: 1}
		if gotAged != wantAged {
			t.Errorf("aged score = %+v, want %+v", gotAged, wantAged)
		}
	})

	t.Run("confidence は小数 4 桁に丸められる", func(t *testing.T) {
		// 同じ trust 1.0 の reviewer、90 日前：weight = exp(-90*ln2/180)
		// = 2^(-1/2) = 0.70710678118654752...
		// Confidence = 0.1*0.6 + 0.4*0.7071067811865475...
		//            = 0.3428427124746190... -> 0.3428 に丸められる
		// （小数第 4 位で half away from zero）。
		trustedHistory := append(repeatRatings(1.0, 10), repeatRatings(5.0, 10)...)
		facts := []domain.ReviewFact{{
			Rating:          4,
			CreatedAt:       now.Add(-90 * 24 * time.Hour),
			ReviewerHistory: domain.ReviewerHistory{Ratings: trustedHistory},
		}}
		got := domain.CalculateBurgerScore(facts, now)
		want := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 0.3428, SampleSize: 1}
		if got != want {
			t.Errorf("CalculateBurgerScore = %+v, want %+v", got, want)
		}
	})

	t.Run("信頼できる fresh な review が多いと confidence は 1.0 で頭打ちになる", func(t *testing.T) {
		// trust 1.0 の fresh な fact 10 件：totalWeight 10 ->
		// Confidence = min(10/10,1)*0.6 + min(10/10,1)*0.4 = 1.0 であり、
		// [0,1] の clamp によって厳密に 1.0 のまま保たれる。
		trustedHistory := append(repeatRatings(1.0, 10), repeatRatings(5.0, 10)...)
		facts := make([]domain.ReviewFact, 10)
		for i := range facts {
			facts[i] = domain.ReviewFact{
				Rating:          4,
				CreatedAt:       now,
				ReviewerHistory: domain.ReviewerHistory{Ratings: trustedHistory},
			}
		}
		got := domain.CalculateBurgerScore(facts, now)
		want := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 1.0, SampleSize: 10}
		if got != want {
			t.Errorf("CalculateBurgerScore = %+v, want %+v", got, want)
		}
	})
}

// TestBurgerStatsAverageRating は、AverageRating を
// Burgers::BurgerEntity#average_rating に対して固定する。Ruby の Float#round
// の half away from zero の意味論も含む。
func TestBurgerStatsAverageRating(t *testing.T) {
	tests := []struct {
		name    string
		ratings []float64
		want    float64
	}{
		{name: "空なら 0.0 になる", ratings: nil, want: 0.0},
		// (4+5)/2 = 4.5、厳密。通常のケース：MRI の境界補正は発動しては
		// ならず（(450+0.5)/100 = 4.505 > 4.5）、4.5 は 4.5 のまま。
		{name: "4 と 5 の平均は 4.5 になる", ratings: []float64{4, 5}, want: 4.5},
		// 13/3 = 4.3333... -> 4.33。通常のケース：過補正なし
		// （(433+0.5)/100 = 4.335 > 4.3333...）なので、4.33 は 4.33 のまま。
		{name: "13 ÷ 3 は 4.33 に丸められる", ratings: []float64{4, 4, 5}, want: 4.33},
		// MRI の numeric.c の round_half_up の境界：39x1 + 1x2 の合計は 41 で、
		// 平均 41/40 の最も近い double は 1.0249999999999999（41.0/40*100 ==
		// 102.49999999999999）であるため、math.Round だけでは 1.02 になる。
		// MRI の (f+0.5)/scale <= x という補正が発動し（102.5/100 はまったく
		// 同じ double であり、<= x が成り立つ）、1.03 に繰り上げられる。これは
		// Ruby 3.3 と一致する：(41.0/40).round(2) == 1.03。
		{name: "MRI 境界の 41 ÷ 40 は 1.03 に丸められる", ratings: append(repeatRatings(1.0, 39), 2.0), want: 1.03},
		// 同じ境界の形：31x4 + 9x5 の合計は 169 で、平均 169/40 は
		// double として 4.2249999999999996（169.0/40*100 ==
		// 422.49999999999994）であり、単純な丸めでは 4.22 になる。補正が発動して
		// 4.23 になり、Ruby の (169.0/40).round(2) と一致する。
		{name: "MRI 境界の 169 ÷ 40 は 4.23 に丸められる", ratings: append(repeatRatings(4.0, 31), repeatRatings(5.0, 9)...), want: 4.23},
		// (4.0+4.25)/2 = 4.125（厳密に表現可能）-> 4.13：Ruby は half を
		// 偶数丸め（4.12）ではなく、0 から遠ざかる方向へ丸める。
		{name: "ちょうど中間の値 (half) は 0 から遠ざかる方向に丸められる", ratings: []float64{4.0, 4.25}, want: 4.13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := make([]domain.ReviewFact, len(tt.ratings))
			for i, rating := range tt.ratings {
				facts[i] = domain.ReviewFact{Rating: rating}
			}
			if got := domain.AverageRating(facts); got != tt.want {
				t.Errorf("AverageRating(%v) = %v, want %v", tt.ratings, got, tt.want)
			}
		})
	}
}

// TestNewRecalcFailure は、再計算に失敗したときの、再試行の待ち時間と記録する理由を固定する。
func TestNewRecalcFailure(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cause := errors.New("db down")

	t.Run("失敗が続くごとに、待ち時間が 2 秒から倍になり、5 分で頭打ちになる", func(t *testing.T) {
		tests := []struct {
			failures int
			want     time.Duration
		}{
			{1, 2 * time.Second},
			{2, 4 * time.Second},
			{3, 8 * time.Second},
			{4, 16 * time.Second},
			{7, 128 * time.Second},
			{8, 256 * time.Second},
			{9, 5 * time.Minute},
			{20, 5 * time.Minute},
			{21, 5 * time.Minute},
			{1 << 30, 5 * time.Minute},
		}
		for _, tt := range tests {
			got := domain.NewRecalcFailure(tt.failures, cause, now).NextAttemptAt.Sub(now)
			if got != tt.want {
				t.Errorf("失敗 %d 回目の待ち時間 = %v, want %v", tt.failures, got, tt.want)
			}
		}
	})

	t.Run("回数が 0 以下でも、待ち時間は上限を超えず、負にもならない", func(t *testing.T) {
		for _, failures := range []int{0, -1} {
			got := domain.NewRecalcFailure(failures, cause, now).NextAttemptAt.Sub(now)
			if got <= 0 || got > 5*time.Minute {
				t.Errorf("回数 %d の待ち時間 = %v, want 0 より大きく 5 分以下", failures, got)
			}
		}
	})

	t.Run("理由は原因のエラーの文言になり、原因がなければ空になる", func(t *testing.T) {
		if got := domain.NewRecalcFailure(1, cause, now).Reason; got != "db down" {
			t.Errorf("Reason = %q, want %q", got, "db down")
		}
		if got := domain.NewRecalcFailure(1, nil, now).Reason; got != "" {
			t.Errorf("原因がないときの Reason = %q, want 空", got)
		}
	})

	t.Run("長い理由は、バイト数ではなく文字数で上限に切り詰められる", func(t *testing.T) {
		limit := domain.MaxRecalcFailureReasonChars
		exact := strings.Repeat("あ", limit)
		if got := domain.NewRecalcFailure(1, errors.New(exact), now).Reason; got != exact {
			t.Errorf("上限ちょうどの理由が変わった(%d 文字 → %d 文字)", limit, utf8.RuneCountInString(got))
		}
		got := domain.NewRecalcFailure(1, errors.New(exact+"い"), now).Reason
		if got != exact {
			t.Errorf("上限を 1 文字超えた理由 = %d 文字, want 先頭の %d 文字", utf8.RuneCountInString(got), limit)
		}
	})

	t.Run("NUL と不正なバイト列は、データベースに入れられる形に直される", func(t *testing.T) {
		got := domain.NewRecalcFailure(1, errors.New("a\x00b\xffc"), now).Reason
		if strings.Contains(got, "\x00") || !utf8.ValidString(got) {
			t.Errorf("Reason = %q, want NUL を含まない有効な UTF-8", got)
		}
	})
}
