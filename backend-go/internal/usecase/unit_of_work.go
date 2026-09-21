package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerStatsQuery は、burger の統計の再計算に必要な、読み取り専用の契約である。
// UnitOfWork の中では、トランザクションに束縛された実装が渡されるので、同じ
// トランザクションの未コミットの書き込みが見える。読み取り専用で、書き込みの
// メソッドは置かない（書き込みは domain.BurgerStats を通す）。
type BurgerStatsQuery interface {
	// ListBurgerReviewFacts は、burger の統計の元になる kept な review を facts として返す。
	// discard 済みの review と、discard 済みの user の review は除外する。各 fact には、その
	// review の author が、すべての burger にわたってつけた kept な rating（reviewer の履歴）が付く。
	ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error)
	// ListReviewedBurgerIDsByUser は、user の kept な review が付く burger の id を、重複なしで
	// burger_id の昇順に返す（ユーザーの退会で統計を再計算する対象）。
	ListReviewedBurgerIDsByUser(ctx context.Context, userID string) ([]string, error)
}

// Tx は UnitOfWork.Do の中で使う、トランザクションに束縛された書き込みと読み取りである。
// 書き込みは domain の書き込みオブジェクト（自分の集約の repository だけを持つ）を通し、
// usecase は repository に依存しない。読み取りも、同じトランザクションで行う。
type Tx struct {
	Reviews     *domain.Reviews
	Users       *domain.Users
	BurgerStats *domain.BurgerStats
	Stats       BurgerStatsQuery
}

// UnitOfWork は、トランザクションの境界を usecase が宣言するための契約である。
// Do は、トランザクションを開始して fn を実行し、fn がエラーを返したら全体を rollback し、
// 成功したら commit する。fn の中の書き込みと読み取りは、すべて同じトランザクションで行われる。
// 複数の集約を更新する手順（例: review の書き込みと burger の統計の再計算）は、usecase が
// この中で組み立てる。実装は adapter が担う。
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Clock は現在時刻の取得元である。統計の再計算に使う時刻を、usecase が固定できるようにする
// （テストで固定の時刻を渡す）。
type Clock interface {
	Now() time.Time
}

// BurgerStatsRecalculator は、burger の統計を再計算する手順である。UnitOfWork.Do の中で、
// 書き込みと同じトランザクションから呼ぶ。手順は「burger の行をロック → kept な review の
// facts を読む → domain の CalculateBurgerStat で計算 → 保存」で、ロックの取り方は
// 再計算を同期で行っていたときと同じである。
type BurgerStatsRecalculator struct {
	clock Clock
}

// NewBurgerStatsRecalculator は clock を使う BurgerStatsRecalculator を返す。
func NewBurgerStatsRecalculator(clock Clock) *BurgerStatsRecalculator {
	return &BurgerStatsRecalculator{clock: clock}
}

// Recalculate は burger の統計を再計算して保存する。tx は UnitOfWork.Do が渡したものでなければ
// ならない。最初に burger の行をロックする（並行する再計算が、相手のコミット前の review が
// 欠けた facts で上書きしないため）。トランザクションがすでに持っているロックの再取得は
// no-op である。1 つのトランザクションで複数の burger を再計算するときは、burger_id の昇順に
// 呼ばなければならない（デッドロックの回避）。対象の review がゼロ件でも、ゼロの統計を保存する
// （Rails BurgerScore.empty）。
func (r *BurgerStatsRecalculator) Recalculate(ctx context.Context, tx Tx, burgerID string) error {
	if err := tx.BurgerStats.Lock(ctx, burgerID); err != nil {
		return fmt.Errorf("recalculate burger stats: lock burger: %w", err)
	}
	facts, err := tx.Stats.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return fmt.Errorf("recalculate burger stats: list facts: %w", err)
	}
	// マイクロ秒に切り詰める（timestamptz の精度）。保存される calculated_at が、スコアの
	// 計算に使った時刻そのものになり、テストは保存された行と、この時刻からスコアを再計算できる。
	now := r.clock.Now().Truncate(time.Microsecond)
	if err := tx.BurgerStats.Save(ctx, domain.CalculateBurgerStat(burgerID, facts, now)); err != nil {
		return fmt.Errorf("recalculate burger stats: save: %w", err)
	}
	return nil
}

// RecalculateReviewedBy は、user の kept な review が付くすべての burger の統計を、burger_id の
// 昇順に再計算する（ユーザーの退会。並行する退会がデッドロックしないよう、ロックの順序を固定する）。
// 昇順は、読み取りの実装が保証するが、ここでも並べ直して、順序を usecase の責務として明示する。
// burger の id は UUID の正規形（小文字・同じ長さ）なので、文字列としての昇順は、データベースの
// uuid 型の昇順と同じになる（読み取りの ORDER BY と、ここでの並べ直しで順序が食い違わない）。
func (r *BurgerStatsRecalculator) RecalculateReviewedBy(ctx context.Context, tx Tx, userID string) error {
	burgerIDs, err := tx.Stats.ListReviewedBurgerIDsByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("list review burgers: %w", err)
	}
	slices.Sort(burgerIDs)
	for _, burgerID := range burgerIDs {
		if err := r.Recalculate(ctx, tx, burgerID); err != nil {
			return err
		}
	}
	return nil
}
