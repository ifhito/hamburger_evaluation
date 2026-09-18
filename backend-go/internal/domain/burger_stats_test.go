package domain_test

import (
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// The cases below are ported from the Rails specs
// backend/spec/domain/reviews/{burger_score_calculator_spec,
// reviewer_trust_evaluator_spec, burger_score_spec}.rb, with the expected
// values derived by hand (derivations in the comments). Assertions use
// exact float equality; where the unrounded expectation involves float
// products (e.g. 0.7*0.7), the expected value is computed at runtime from
// float64 variables so it goes through exactly the same float64
// operations as the implementation.

func repeatRatings(rating float64, count int) []float64 {
	ratings := make([]float64, count)
	for i := range ratings {
		ratings[i] = rating
	}
	return ratings
}

// TestBurgerStatsReviewerTrustScore pins ReviewerTrustScore against the
// Rails ReviewerTrustEvaluator behavior.
func TestBurgerStatsReviewerTrustScore(t *testing.T) {
	// 0.7*0.7 computed with runtime float64 semantics (== the value the
	// implementation produces; the constant literal 0.49 is 1 ulp away).
	base, penalty := 0.7, 0.7
	regularPenalized := base * penalty

	tests := []struct {
		name    string
		ratings []float64
		want    float64
	}{
		// No reviews: newcomer base 0.5, fewer than 3 ratings so variance
		// factor 1.0 -> 0.5.
		{name: "empty history is newcomer 0.5", ratings: nil, want: 0.5},
		// 3 ratings: regular base 0.7. mean 13/3, population variance
		// (2*(1/3)^2 + (2/3)^2)/3 = 2/9 ~= 0.2222 < 0.3 -> penalty 0.7,
		// score 0.7*0.7 (~0.49).
		{name: "regular with low variance 4,4,5", ratings: []float64{4, 4, 5}, want: regularPenalized},
		// 20 identical ratings: expert base 1.0, variance 0 < 0.3 ->
		// penalty 0.7, score 1.0*0.7 = 0.7 exactly.
		{name: "expert with zero variance 20x4", ratings: repeatRatings(4.0, 20), want: 0.7},
		// 5 identical ratings: regular base 0.7, variance 0 -> 0.7*0.7.
		{name: "regular with zero variance 5x5", ratings: repeatRatings(5.0, 5), want: regularPenalized},
		// mean 3, variance (4+1+1+4+0)/5 = 2.0 >= 0.3 -> no penalty,
		// regular 0.7 exactly.
		{name: "regular with varied ratings 1,2,4,5,3", ratings: []float64{1, 2, 4, 5, 3}, want: 0.7},
		// 10 ratings alternating 1 and 5: veteran base 0.9, mean 3,
		// variance 4.0 >= 0.3 -> 0.9 exactly.
		{name: "veteran with varied ratings", ratings: []float64{1, 5, 1, 5, 1, 5, 1, 5, 1, 5}, want: 0.9},
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

// TestBurgerStatsCalculateBurgerScore pins CalculateBurgerScore against
// the Rails BurgerScoreCalculator + BurgerScore behavior.
func TestBurgerStatsCalculateBurgerScore(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

	t.Run("empty facts return the zero score", func(t *testing.T) {
		got := domain.CalculateBurgerScore(nil, now)
		want := domain.BurgerScore{WeightedAverage: 0.0, Confidence: 0.0, SampleSize: 0}
		if got != want {
			t.Errorf("CalculateBurgerScore(nil) = %+v, want %+v", got, want)
		}
	})

	t.Run("single fresh fact", func(t *testing.T) {
		// Trust for history [5]: newcomer 0.5, <3 ratings so no variance
		// factor. Recency at now: exp(0) = 1, weight 0.5.
		// WeightedAverage = 5*0.5/0.5 = 5.0.
		// Confidence = min(1/10,1)*0.6 + min(0.5/1,1)*0.4
		//            = 0.06 + 0.2 = 0.26 (rounded to 4 decimals).
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

	t.Run("expert outweighs newcomer", func(t *testing.T) {
		// Newcomer fact: rating 1, history [1] -> trust 0.5.
		// Expert fact: rating 5, history 20x3.0 + [5.0] (21 ratings ->
		// expert base 1.0). mean 65/21, variance
		// (20*(2/21)^2 + (40/21)^2)/21 = 1680/9261 ~= 0.1814 < 0.3 ->
		// penalty 0.7, trust 0.7. Both created at now so weights are
		// 0.5 and 0.7.
		// WeightedAverage = (1*0.5 + 5*0.7)/(0.5+0.7) = 4.0/1.2
		//                 = 3.3333... -> 3.33.
		// Confidence = min(2/10,1)*0.6 + min(1.2/2,1)*0.4
		//            = 0.12 + 0.24 = 0.36.
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

	t.Run("recency halves the weight after 180 days", func(t *testing.T) {
		// History 10x1.0 + 10x5.0: 20 ratings -> expert base 1.0, mean 3,
		// variance 4.0 >= 0.3 -> trust 1.0 exactly.
		trustedHistory := append(repeatRatings(1.0, 10), repeatRatings(5.0, 10)...)
		fresh := domain.ReviewFact{
			Rating:          4,
			CreatedAt:       now,
			ReviewerHistory: domain.ReviewerHistory{Ratings: trustedHistory},
		}
		aged := fresh
		aged.CreatedAt = now.Add(-180 * 24 * time.Hour)

		// Fresh: weight 1.0 -> Confidence = 0.1*0.6 + 1.0*0.4 = 0.46.
		gotFresh := domain.CalculateBurgerScore([]domain.ReviewFact{fresh}, now)
		wantFresh := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 0.46, SampleSize: 1}
		if gotFresh != wantFresh {
			t.Errorf("fresh score = %+v, want %+v", gotFresh, wantFresh)
		}

		// 180 days = one half-life: weight = exp(-180*ln2/180) = 0.5 ->
		// Confidence = 0.1*0.6 + 0.5*0.4 = 0.26. The weighted average of
		// a single fact is unchanged by recency (weight cancels): 4.0.
		gotAged := domain.CalculateBurgerScore([]domain.ReviewFact{aged}, now)
		wantAged := domain.BurgerScore{WeightedAverage: 4.0, Confidence: 0.26, SampleSize: 1}
		if gotAged != wantAged {
			t.Errorf("aged score = %+v, want %+v", gotAged, wantAged)
		}
	})

	t.Run("confidence rounds to 4 decimals", func(t *testing.T) {
		// Same trust-1.0 reviewer, 90 days old: weight = exp(-90*ln2/180)
		// = 2^(-1/2) = 0.70710678118654752...
		// Confidence = 0.1*0.6 + 0.4*0.7071067811865475...
		//            = 0.3428427124746190... -> rounds to 0.3428
		// (half away from zero on the 4th decimal).
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

	t.Run("confidence caps at 1.0 with many trusted fresh reviews", func(t *testing.T) {
		// 10 fresh facts of trust 1.0: totalWeight 10 ->
		// Confidence = min(10/10,1)*0.6 + min(10/10,1)*0.4 = 1.0, which
		// the [0,1] clamp keeps at exactly 1.0.
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

// TestBurgerStatsAverageRating pins AverageRating against
// Burgers::BurgerEntity#average_rating, including the Ruby Float#round
// half-away-from-zero semantics.
func TestBurgerStatsAverageRating(t *testing.T) {
	tests := []struct {
		name    string
		ratings []float64
		want    float64
	}{
		{name: "empty is 0.0", ratings: nil, want: 0.0},
		// (4+5)/2 = 4.5, exact. Ordinary case: the MRI boundary correction
		// must NOT fire ((450+0.5)/100 = 4.505 > 4.5), so 4.5 stays 4.5.
		{name: "mean of 4 and 5", ratings: []float64{4, 5}, want: 4.5},
		// 13/3 = 4.3333... -> 4.33. Ordinary case: no over-correction
		// ((433+0.5)/100 = 4.335 > 4.3333...), so 4.33 stays 4.33.
		{name: "rounds 13/3 to 4.33", ratings: []float64{4, 4, 5}, want: 4.33},
		// MRI numeric.c round_half_up boundary: 39x1 + 1x2 sums to 41, mean
		// 41/40 whose nearest double is 1.0249999999999999 (41.0/40*100 ==
		// 102.49999999999999), so math.Round alone gives 1.02. MRI's
		// (f+0.5)/scale <= x correction fires (102.5/100 is the very same
		// double, <= x holds) and bumps to 1.03 — matching Ruby 3.3:
		// (41.0/40).round(2) == 1.03.
		{name: "MRI boundary 41/40 rounds to 1.03", ratings: append(repeatRatings(1.0, 39), 2.0), want: 1.03},
		// Same boundary shape: 31x4 + 9x5 sums to 169, mean 169/40 ==
		// 4.2249999999999996 as a double (169.0/40*100 ==
		// 422.49999999999994), plain rounding gives 4.22; the correction
		// fires and yields 4.23 — matching Ruby's (169.0/40).round(2).
		{name: "MRI boundary 169/40 rounds to 4.23", ratings: append(repeatRatings(4.0, 31), repeatRatings(5.0, 9)...), want: 4.23},
		// (4.0+4.25)/2 = 4.125 (exactly representable) -> 4.13: Ruby
		// rounds halves away from zero, not to even (4.12).
		{name: "rounds halves away from zero", ratings: []float64{4.0, 4.25}, want: 4.13},
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
