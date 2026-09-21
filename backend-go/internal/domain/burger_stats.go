package domain

import (
	"context"
	"math"
	"time"
)

// review から導出される burger の統計（issue #15、S7）。このファイルは
// backend/app/domain/reviews/{burger_score_calculator,
// reviewer_trust_evaluator, reviewer_trust, burger_score}.rb と
// backend/app/domain/burgers/burger_entity.rb にある Rails の domain
// ロジックを純粋に移植したものであり、同一の入力は同一の出力を生成しなければ
// ならない。Rails の ReviewerTrust の level ラベル
// （newcomer/regular/veteran/expert）は意図的に移植していない。出力に
// 反映されるのは数値のスコアだけである。

const (
	// recencyHalfLifeDays は BurgerScoreCalculator::RECENCY_HALF_LIFE_DAYS に
	// 対応する。
	recencyHalfLifeDays = 180.0
	// lowVarianceThreshold は ReviewerTrustEvaluator::LOW_VARIANCE_THRESHOLD に
	// 対応する。
	lowVarianceThreshold = 0.3
	// lowVariancePenalty は ReviewerTrustEvaluator::LOW_VARIANCE_PENALTY に
	// 対応する。
	lowVariancePenalty = 0.7
)

// ReviewerHistory は、1 人の reviewer がすべての burger にわたってつけた kept
// な rating である（Rails Reviews::ReviewerHistory）。
type ReviewerHistory struct{ Ratings []float64 }

// ReviewFact は、スコア計算への入力となる kept な review 1 件である
// （Rails Reviews::ReviewFact）。
type ReviewFact struct {
	Rating          float64
	CreatedAt       time.Time
	ReviewerHistory ReviewerHistory
}

// BurgerScore は重み付きスコアの出力である（Rails Reviews::BurgerScore）。
// Rails 側のコンストラクタが丸めと clamp を行うので、ここのフィールドは常に
// 丸め済みの値を保持する。
type BurgerScore struct {
	WeightedAverage float64 // 小数 2 桁に丸める
	Confidence      float64 // [0,1] に clamp し、小数 4 桁に丸める
	SampleSize      int
}

// ReviewerTrustScore は、Reviews::ReviewerTrustEvaluator#call を、
// Reviews::ReviewerTrust#initialize のスコア clamp と組み合わせて移植する。
// 基礎スコアは review 数から決まる（20 件以上は expert 1.0、10 件以上は
// veteran 0.9、3 件以上は regular 0.7、それ以外は newcomer 0.5）。rating が
// 3 件以上あり母分散が 0.3 未満の reviewer は、基礎スコアに 0.7 が掛けられる
// （0.7 を引くのではなく 0.7 倍）。結果は [0.0, 1.0] に clamp される。
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

// CalculateBurgerScore は、Reviews::BurgerScoreCalculator#call を、
// Reviews::BurgerScore#initialize の丸め/clamp とともに移植する。各 fact は
// reviewer の trust に、半減期 180 日の指数的な recency 減衰を掛けたもので
// 重み付けされる。未来のタイムスタンプに対しては daysAgo が負になりうるが、
// Rails と同様に clamp しない。空の入力はゼロスコア
// （Reviews::BurgerScore.empty）を返す。
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

// AverageRating は Burgers::BurgerEntity#average_rating を移植する。rating の
// 単純な算術平均を小数 2 桁に丸めたもので、review がない場合は 0.0 である。
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

// recencyFactor は BurgerScoreCalculator#recency_factor を移植する。
// exp(-daysAgo * ln(2) / 180) であり、daysAgo は（負になりうる）経過時間を
// 小数の日数で表したものである。
func recencyFactor(createdAt, now time.Time) float64 {
	daysAgo := now.Sub(createdAt).Seconds() / 86400.0
	return math.Exp(-daysAgo * math.Ln2 / recencyHalfLifeDays)
}

// roundHalfAwayFromZero は、MRI の numeric.c の round_half_up を移植することで
// Ruby の Float#round（half away from zero）を再現する。scale は小数 2 桁なら
// 100、小数 4 桁なら 10000 である。math.Round(value*scale) に加えて、MRI は、
// 最も近い double が厳密な境界のすぐ下にある小数の境界（例：41.0/40 ->
// 1.0249999999999999）に対する補正を適用する。次の丸めステップ（上方向/
// 下方向）を scale を通して元に戻した値が、それでも元の値を超えないとき、
// 結果は 0 から遠ざかる方向へ 1 ステップ進められ、Ruby の Float#round の出力を
// 正確に再現する。
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

// clampFloat は float 向けの Ruby Comparable#clamp を再現する。
func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// BurgerStat は、burger 1 件の導出された統計(kept な review の件数・平均・加重スコア・
// 信頼度)と、それを計算した時刻である。burger_stats の 1 行に保存される。
type BurgerStat struct {
	BurgerID      string
	ReviewCount   int64
	AverageRating float64
	WeightedScore float64
	Confidence    float64
	CalculatedAt  time.Time
}

// CalculateBurgerStat は、burger の kept な review の facts から、now 時点の統計を計算する。
// 永続化には触れない純粋な計算で、facts がゼロ件のときはゼロの統計になる
// （Rails BurgerScore.empty）。CalculatedAt は、スコアの計算に使った now そのものである。
func CalculateBurgerStat(burgerID string, facts []ReviewFact, now time.Time) BurgerStat {
	score := CalculateBurgerScore(facts, now)
	return BurgerStat{
		BurgerID:      burgerID,
		ReviewCount:   int64(len(facts)),
		AverageRating: AverageRating(facts),
		WeightedScore: score.WeightedAverage,
		Confidence:    score.Confidence,
		CalculatedAt:  now,
	}
}

// ---- repository の契約(実装は adapter/repository) ----

// BurgerStatRepository は burger の統計の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの BurgerStats）だけで、usecase は呼ばない
// （統計の元になる facts の読み取りは usecase の BurgerStatsQuery）。書き込み専用で、
// 読み取りのメソッドは置かない。
type BurgerStatRepository interface {
	// LockBurgerStat は、burger の行を FOR UPDATE でロックし、burger ごとの統計の再計算を
	// 直列化する。再計算は「全件を読んでから上書きする」処理なので、ロックがないと、並行する
	// 2 つのトランザクションが、相手のコミット前の review が欠けた facts を読み、後の書き込みが
	// 古い件数で統計を上書きする（lost update）。呼び出し側のトランザクションが終わるまで
	// 保持される。トランザクションがすでに持っているロックの再取得は no-op である。
	// 1 つのトランザクションで複数の burger をロックするときは、burger_id の昇順に呼ばなければ
	// ならない（デッドロックの回避）。存在しない burger は、wrap されたエラーを返す。
	// 値を返さず、行を変更もしない、書き込みの前段の排他制御である（読み取りではない）。
	LockBurgerStat(ctx context.Context, burgerID string) error
	// UpdateBurgerStat は、burger_stats の行を stat の値で置き換える（行がなければ作る。upsert）。
	UpdateBurgerStat(ctx context.Context, stat BurgerStat) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// BurgerStats は burger の統計の書き込みオブジェクトである。BurgerStatRepository を持つのは
// この型だけで、usecase は repository に依存せず、統計の書き込みをここに任せる。統計の
// 計算そのもの（CalculateBurgerStat）は純粋な規則で、facts の読み取りと、ロック・保存を
// 組み合わせる再計算の手順は、トランザクションを持つ usecase が組み立てる。
type BurgerStats struct {
	repo BurgerStatRepository
}

// NewBurgerStats は repo を使う BurgerStats を返す。
func NewBurgerStats(repo BurgerStatRepository) *BurgerStats {
	return &BurgerStats{repo: repo}
}

// Lock は burger の統計の再計算を直列化するために、burger の行をロックする。
func (s *BurgerStats) Lock(ctx context.Context, burgerID string) error {
	return s.repo.LockBurgerStat(ctx, burgerID)
}

// Save は、計算済みの統計を保存する（行がなければ作る）。
func (s *BurgerStats) Save(ctx context.Context, stat BurgerStat) error {
	return s.repo.UpdateBurgerStat(ctx, stat)
}
