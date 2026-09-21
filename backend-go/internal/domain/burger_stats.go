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

// 統計の再計算は、書き込みと同じトランザクションで「再計算の依頼」を登録しておき、
// バックグラウンドのワーカーがあとから実行する。次の型と規則は、その依頼と失敗の扱いを表す。

const (
	// recalcRetryBaseDelay は、再計算に 1 回失敗したときの、次の再試行までの待ち時間である。
	// 失敗が続くたびに倍にする(指数バックオフ)。
	recalcRetryBaseDelay = 2 * time.Second
	// recalcRetryMaxDelay は、再試行までの待ち時間の上限である。
	recalcRetryMaxDelay = 5 * time.Minute
	// MaxRecalcFailureReasonChars は、記録する失敗の理由の文字数の上限(Unicode のコードポイント数)である。
	// DB の CHECK 制約 burger_stats_dirty_last_error_length_check(000010_create_burger_stats_dirty)と
	// 同じ値でなければならない。食い違いは db/migrations_test.go が検出する。
	MaxRecalcFailureReasonChars = 500
)

// RecalcRequest は、統計の再計算を待っているバーガー 1 件の依頼である。
type RecalcRequest struct {
	BurgerID string
	// Version は、依頼が登録されるたびに、時間をまたいで単調に増える番号である。再計算を終えて
	// 依頼を消すときは、取り出したときの Version と一致する場合だけ消す。再計算の最中に
	// 新しい書き込みが入ると Version が進むので、その依頼は消えずに残り、次の再計算で最新になる。
	Version int64
	// Attempts は、これまでに再計算に失敗した回数である。
	Attempts int
}

// RecalcFailure は、再計算の失敗の記録である。
type RecalcFailure struct {
	// NextAttemptAt は、次に再試行する時刻である。
	NextAttemptAt time.Time
	// Reason は、失敗の理由(MaxRecalcFailureReasonChars 文字までに切り詰めたもの)である。
	Reason string
}

// NewRecalcFailure は、再計算の失敗を記録する内容を作る。failures は今回を含む失敗の回数(1 以上)で、
// 次の再試行の時刻は、now に、失敗の回数に応じた待ち時間(2 秒から始めて倍にし、5 分を上限とする)を
// 足したものになる。理由は、原因のエラーの文言を、上限の文字数に切り詰めたものである。
func NewRecalcFailure(failures int, cause error, now time.Time) RecalcFailure {
	delay := recalcRetryMaxDelay
	// 2 の (failures-1) 乗を掛ける。上限を超える大きさになる前に打ち切って、あふれを避ける。
	if failures >= 1 && failures <= 20 {
		if d := recalcRetryBaseDelay << (failures - 1); d < recalcRetryMaxDelay {
			delay = d
		}
	}
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	if runes := []rune(reason); len(runes) > MaxRecalcFailureReasonChars {
		reason = string(runes[:MaxRecalcFailureReasonChars])
	}
	return RecalcFailure{NextAttemptAt: now.Add(delay), Reason: reason}
}

// ---- repository の契約(実装は adapter/repository) ----

// BurgerStatRepository は burger の統計の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの BurgerStats）だけで、usecase は呼ばない
// （統計の元になる facts の読み取りは usecase の BurgerStatsQuery）。書き込み専用で、
// 読み取りのメソッドは置かない。
type BurgerStatRepository interface {
	// LockBurgerStat は、burger の行をロック(FOR NO KEY UPDATE)し、burger ごとの統計の再計算を
	// 直列化する。再計算は「全件を読んでから上書きする」処理なので、ロックがないと、並行する
	// 2 つのトランザクションが、相手のコミット前の review が欠けた facts を読み、後の書き込みが
	// 古い件数で統計を上書きする（lost update）。呼び出し側のトランザクションが終わるまで
	// 保持される。レビューの書き込み(外部キーの検査が取る共有ロック)は待たせない。トランザクションが
	// すでに持っているロックの再取得は no-op である。
	// 1 つのトランザクションで複数の burger をロックするときは、burger_id の昇順に呼ばなければ
	// ならない（デッドロックの回避）。存在しない burger は、wrap されたエラーを返す。
	// 値を返さず、行を変更もしない、書き込みの前段の排他制御である（読み取りではない）。
	LockBurgerStat(ctx context.Context, burgerID string) error
	// UpdateBurgerStat は、burger_stats の行を stat の値で置き換える（行がなければ作る。upsert）。
	UpdateBurgerStat(ctx context.Context, stat BurgerStat) error
	// CreateBurgerStatRecalcRequest は、burger の統計の再計算を依頼する。すでに依頼があれば、
	// version を進め、失敗の記録(回数・次の再試行の時刻・理由)を消して、最初からやり直す。
	// 書き込みと同じトランザクションから呼ぶ。統計は計算せず、burger の行もロックしない。
	// 存在しない burger は、wrap されたエラーを返す。
	CreateBurgerStatRecalcRequest(ctx context.Context, burgerID string) error
	// DiscardBurgerStatRecalcRequest は、再計算を終えた依頼を消す。取り出したときの version と
	// 一致する場合だけ消し、消せたかどうかを返す(false は、その間に新しい書き込みが入って
	// version が進んだことを表す。依頼は残るので、次の再計算で最新になる)。
	DiscardBurgerStatRecalcRequest(ctx context.Context, burgerID string, version int64) (bool, error)
	// UpdateBurgerStatRecalcFailure は、再計算の失敗を記録する(失敗の回数を 1 増やし、次の再試行の
	// 時刻と理由を書く)。取り出したときの version と一致する場合だけ更新し、更新できたかどうかを
	// 返す(false は、その間に新しい書き込みが入ったことを表す。その依頼は最初からやり直す)。
	UpdateBurgerStatRecalcFailure(ctx context.Context, burgerID string, version int64, failure RecalcFailure) (bool, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// BurgerStats は burger の統計の書き込みオブジェクトである。BurgerStatRepository を持つのは
// この型だけで、usecase は repository に依存せず、統計の書き込みをここに任せる。統計の
// 計算そのもの（CalculateBurgerStat）は純粋な規則で、facts の読み取りと、ロック・保存を
// 組み合わせる再計算の手順は、トランザクションを持つ usecase が組み立てる。レビューの書き込みと
// 退会は、統計を計算せず、同じトランザクションで再計算の依頼(RequestRecalc)を登録するだけで、
// 計算はバックグラウンドのワーカーがあとから行う。
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

// RequestRecalc は、burger の統計の再計算を依頼する(すでに依頼があれば、最初からやり直す)。
// 書き込みと同じトランザクションから呼ぶ。統計は計算しない。
func (s *BurgerStats) RequestRecalc(ctx context.Context, burgerID string) error {
	return s.repo.CreateBurgerStatRecalcRequest(ctx, burgerID)
}

// CompleteRecalc は、再計算を終えた依頼を消す。取り出したときの version と一致する場合だけ消し、
// 消せたかどうかを返す。
func (s *BurgerStats) CompleteRecalc(ctx context.Context, burgerID string, version int64) (bool, error) {
	return s.repo.DiscardBurgerStatRecalcRequest(ctx, burgerID, version)
}

// RecordRecalcFailure は、再計算の失敗を記録する。取り出したときの version と一致する場合だけ
// 記録し、記録できたかどうかを返す。
func (s *BurgerStats) RecordRecalcFailure(ctx context.Context, burgerID string, version int64, failure RecalcFailure) (bool, error) {
	return s.repo.UpdateBurgerStatRecalcFailure(ctx, burgerID, version, failure)
}
