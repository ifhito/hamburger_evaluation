package domain

import (
	"math"
	"time"
)

// Burger statistics derived from reviews (issue #15, S7). This file is a
// pure port of the Rails domain logic in
// backend/app/domain/reviews/{burger_score_calculator,
// reviewer_trust_evaluator, reviewer_trust, burger_score}.rb and
// backend/app/domain/burgers/burger_entity.rb: identical inputs must
// produce identical outputs. The Rails ReviewerTrust level label
// (newcomer/regular/veteran/expert) is deliberately not ported — only the
// numeric score feeds the outputs.

const (
	// recencyHalfLifeDays mirrors BurgerScoreCalculator::RECENCY_HALF_LIFE_DAYS.
	recencyHalfLifeDays = 180.0
	// lowVarianceThreshold mirrors ReviewerTrustEvaluator::LOW_VARIANCE_THRESHOLD.
	lowVarianceThreshold = 0.3
	// lowVariancePenalty mirrors ReviewerTrustEvaluator::LOW_VARIANCE_PENALTY.
	lowVariancePenalty = 0.7
)

// ReviewerHistory is the kept ratings of one reviewer across all burgers
// (Rails Reviews::ReviewerHistory).
type ReviewerHistory struct{ Ratings []float64 }

// ReviewFact is one kept review as input to the score calculation
// (Rails Reviews::ReviewFact).
type ReviewFact struct {
	Rating          float64
	CreatedAt       time.Time
	ReviewerHistory ReviewerHistory
}

// BurgerScore is the weighted score output (Rails Reviews::BurgerScore):
// the constructor there rounds/clamps, so the fields here always hold the
// already-rounded values.
type BurgerScore struct {
	WeightedAverage float64 // rounded to 2 decimals
	Confidence      float64 // clamped to [0,1], rounded to 4 decimals
	SampleSize      int
}

// ReviewerTrustScore ports Reviews::ReviewerTrustEvaluator#call combined
// with the score clamp in Reviews::ReviewerTrust#initialize. The base
// score comes from the review count (>=20 expert 1.0, >=10 veteran 0.9,
// >=3 regular 0.7, else newcomer 0.5); reviewers with 3+ ratings whose
// population variance is below 0.3 are penalized by 0.7. The result is
// clamped to [0.0, 1.0].
func ReviewerTrustScore(history ReviewerHistory) float64 {
	count := len(history.Ratings)

	var base float64
	switch {
	case count >= 20:
		base = 1.0
	case count >= 10:
		base = 0.9
	case count >= 3:
		base = 0.7
	default:
		base = 0.5
	}

	factor := 1.0
	if count >= 3 {
		var sum float64
		for _, rating := range history.Ratings {
			sum += rating
		}
		mean := sum / float64(count)
		var squaredDeviations float64
		for _, rating := range history.Ratings {
			deviation := rating - mean
			squaredDeviations += deviation * deviation
		}
		variance := squaredDeviations / float64(count)
		if variance < lowVarianceThreshold {
			factor = lowVariancePenalty
		}
	}

	return clampFloat(base*factor, 0.0, 1.0)
}

// CalculateBurgerScore ports Reviews::BurgerScoreCalculator#call with the
// rounding/clamping from Reviews::BurgerScore#initialize. Each fact is
// weighted by reviewer trust times an exponential recency decay with a
// 180-day half-life; daysAgo may be negative for future timestamps —
// mirroring Rails, it is not clamped. Empty input yields the zero score
// (Reviews::BurgerScore.empty).
func CalculateBurgerScore(facts []ReviewFact, now time.Time) BurgerScore {
	if len(facts) == 0 {
		return BurgerScore{WeightedAverage: 0.0, Confidence: 0.0, SampleSize: 0}
	}

	var totalWeight, weightedSum float64
	for _, fact := range facts {
		weight := ReviewerTrustScore(fact.ReviewerHistory) * recencyFactor(fact.CreatedAt, now)
		totalWeight += weight
		weightedSum += fact.Rating * weight
	}

	weightedAverage := weightedSum / totalWeight

	count := float64(len(facts))
	reviewFactor := math.Min(count/10.0, 1.0)
	weightFactor := math.Min(totalWeight/count, 1.0)
	confidence := reviewFactor*0.6 + weightFactor*0.4

	return BurgerScore{
		WeightedAverage: roundHalfAwayFromZero(weightedAverage, 100),
		Confidence:      roundHalfAwayFromZero(clampFloat(confidence, 0.0, 1.0), 10000),
		SampleSize:      len(facts),
	}
}

// AverageRating ports Burgers::BurgerEntity#average_rating: the plain
// arithmetic mean of the ratings rounded to 2 decimals, 0.0 when there
// are no reviews.
func AverageRating(facts []ReviewFact) float64 {
	if len(facts) == 0 {
		return 0.0
	}
	var sum float64
	for _, fact := range facts {
		sum += fact.Rating
	}
	return roundHalfAwayFromZero(sum/float64(len(facts)), 100)
}

// recencyFactor ports BurgerScoreCalculator#recency_factor:
// exp(-daysAgo * ln(2) / 180) where daysAgo is the (possibly negative)
// elapsed time in fractional days.
func recencyFactor(createdAt, now time.Time) float64 {
	daysAgo := now.Sub(createdAt).Seconds() / 86400.0
	return math.Exp(-daysAgo * math.Ln2 / recencyHalfLifeDays)
}

// roundHalfAwayFromZero mirrors Ruby Float#round (half away from zero) by
// porting MRI numeric.c round_half_up: scale is 100 for 2 decimals and
// 10000 for 4 decimals. Beyond math.Round(value*scale), MRI applies a
// correction for decimal boundaries whose nearest double sits just below
// the exact boundary (e.g. 41.0/40 -> 1.0249999999999999): when the next
// rounding step up/down, mapped back through the scale, still does not
// exceed the original value, the result is bumped one step towards away
// from zero — exactly reproducing Ruby's Float#round output.
func roundHalfAwayFromZero(value, scale float64) float64 {
	f := math.Round(value * scale)
	if value > 0 {
		if (f+0.5)/scale <= value {
			f++
		}
	} else if value < 0 {
		if (f-0.5)/scale >= value {
			f--
		}
	}
	return f / scale
}

// clampFloat mirrors Ruby Comparable#clamp for floats.
func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
