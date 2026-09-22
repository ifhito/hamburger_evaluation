package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// BurgerStatsQuery は、バーガーの統計を計算し直すために必要な、読み取り専用の窓口である。
// UnitOfWork(まとめて 1 つのトランザクションにする範囲)の中では、そのトランザクションに結び付いた
// 実装が渡されるので、同じトランザクションの、まだ確定していない書き込みも読み取れる(統計の
// 元データに、直前に登録・削除したレビューを反映させるために必要)。書き込みのメソッドは置かない
// (書き込みは domain.BurgerStats を通す)。
type BurgerStatsQuery interface {
	// ListBurgerReviewFacts は、バーガーの統計の元になるレビューを、計算用の値(domain.ReviewFact)
	// にして返す。削除済みのレビューと、削除済みのユーザーが書いたレビューは含めない。それぞれの
	// 値には、そのレビューの投稿者が、すべてのバーガーに付けた有効な評価(投稿者の信頼度の計算に使う)
	// を添える。
	ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error)
	// ListReviewedBurgerIDsByUser は、ユーザーの有効なレビューが付いているバーガーの ID を、重複なしで
	// 昇順に返す。ユーザーが退会したとき、統計を計算し直す対象を知るために使う。
	ListReviewedBurgerIDsByUser(ctx context.Context, userID string) ([]string, error)
	// ListDueRecalcRequests は、再計算の時期が来ている依頼を、上限 batch 件まで返す。
	// 次の再試行の時刻が now 以前(または未設定)で、失敗の回数が maxAttempts に達していないものが
	// 対象である(達したものは打ち切り)。トランザクションの外の読み取りでもよい。
	ListDueRecalcRequests(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error)
}

// Tx は、UnitOfWork.Do の中で使う、同じトランザクションに結び付いた書き込みと読み取りの組である
// (名前は Transaction の略)。
// 書き込みは domain の書き込みオブジェクト(自分の集約の repository だけを持つ)を通し、usecase は
// repository を直接扱わない。読み取りも同じトランザクションで行うので、書き込みの結果が見える。
type Tx struct {
	Reviews     *domain.Reviews
	Users       *domain.Users
	BurgerStats *domain.BurgerStats
	Stats       BurgerStatsQuery
	// ShopStats と ShopStatsReads は、ショップの集計の書き込み(再計算の依頼の登録・削除、集計の保存)と、
	// その元データ・依頼の読み取りである。
	ShopStats           *domain.ShopStats
	ShopStatsReads      ShopStatsQuery
	SignupVerifications *domain.SignupVerifications
	PendingSignups      SignupVerificationQuery
	OAuthGrants         *domain.OAuthGrants
	// UserIdentities は、外部のサービス(Google など)のアカウントとの結び付きである。外部のサービスでの
	// 新規登録で、ユーザーの作成と同じトランザクションで記録する。
	UserIdentities *domain.UserIdentities
	// LoginHandoffs と PendingHandoff は、外部のサービスでのサインインの結果を画面へ渡すコードの、書き込みと読み取りである。
	// コードを使う手順(ロック → 読み取り → 後続の処理 → 削除)を、1 つのトランザクションにするために使う。
	LoginHandoffs  *domain.LoginHandoffs
	PendingHandoff LoginHandoffQuery
	// UserReads は、トランザクションの接続で、ユーザーを読む窓口である。トランザクションの中で、プールから
	// もう 1 つ接続を取ると、同じ行を待つ処理が接続を使い切ったときに、先頭の処理が接続を取れず、全体が止まる
	// (デッドロック)。トランザクションの中の読み取りは、この窓口を使う。
	UserReads UserQuery
}

// UnitOfWork(作業のひとまとまり)は、「ここからここまでの書き込みと読み取りを、まとめて 1 つの
// トランザクションにする」範囲を、usecase が指定するための仕組みである。
//
// トランザクションとは、全部成功したときだけ確定し、途中で失敗したら全部なかったことにできる、
// データベース操作のひとまとまりのこと。Do は、それを開始して fn を実行し、fn がエラーを返したら
// 全体を取り消し(rollback)、成功したら確定する(commit)。fn の中の書き込みと読み取りは、すべて
// 同じトランザクションで行われる。したがって、レビューの保存と統計の再計算のように、複数の
// 集約を更新する手順を fn の中に書けば、片方だけが反映されることがない。実装は adapter が持つ。
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Clock は現在時刻の取得元である。再計算に使う時刻を差し替えられるようにして、テストで時刻を
// 固定できるようにする(time.Now を直接呼ぶと、保存された統計を後から再現できない)。
type Clock interface {
	Now() time.Time
}

// BurgerStatsRecalculator(「バーガーの統計を再計算する役」の意味)は、バーガーの統計の再計算に関する
// 手順をまとめる。
//
// 再計算は、書き込み(レビューの投稿・編集・削除、退会)の中では行わない。書き込みは、同じ
// トランザクションで「再計算の依頼」を登録するだけ(RequestRecalculation)で、統計の計算は、
// バックグラウンドのワーカー(StatsWorker)が、あとから Recalculate で行う。そのため、書き込みの
// 応答は統計の計算を待たず、統計は少し遅れて(通常は数秒以内に)反映される(結果整合)。
type BurgerStatsRecalculator struct {
	clock Clock
}

// NewBurgerStatsRecalculator は clock を使う BurgerStatsRecalculator を返す。
func NewBurgerStatsRecalculator(clock Clock) *BurgerStatsRecalculator {
	return &BurgerStatsRecalculator{clock: clock}
}

// RequestRecalculation は、burger の統計の再計算を依頼する。tx は UnitOfWork.Do が渡したもので
// なければならず、書き込みと同じトランザクションで登録される(書き込みがロールバックされれば、
// 依頼も残らない)。統計は計算せず、burger の行もロックしない。
func (r *BurgerStatsRecalculator) RequestRecalculation(ctx context.Context, tx Tx, burgerID string) error {
	if err := tx.BurgerStats.RequestRecalc(ctx, burgerID); err != nil {
		return fmt.Errorf("request burger stats recalculation: %w", err)
	}
	return nil
}

// RequestRecalculationReviewedBy は、user の kept な review が付くすべての burger の再計算を、
// burger_id の昇順に依頼する(ユーザーの退会)。複数の行を、いつも同じ順序で登録すれば、並行する
// 退会どうしが、互いの行を逆順に待ってデッドロックすることがない。昇順は、読み取りの実装も
// 保証するが、ここでも並べ直して、順序を usecase の責務として明示する。burger の id は UUID の
// 正規形（小文字・同じ長さ）なので、文字列としての昇順は、データベースの uuid 型の昇順と同じになる。
func (r *BurgerStatsRecalculator) RequestRecalculationReviewedBy(ctx context.Context, tx Tx, userID string) error {
	burgerIDs, err := tx.Stats.ListReviewedBurgerIDsByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("list review burgers: %w", err)
	}
	slices.Sort(burgerIDs)
	for _, burgerID := range burgerIDs {
		if err := r.RequestRecalculation(ctx, tx, burgerID); err != nil {
			return err
		}
	}
	return nil
}

// Recalculate はバーガーの統計を計算し直して保存する(ワーカーが使う)。tx は UnitOfWork.Do が
// 渡したものでなければならない。手順は「バーガーの行をロックする → 統計の元データを読む → domain の
// 計算(CalculateBurgerStat)で統計を求める → 保存する」である。
//
// 最初にバーガーの行をロックするのは、同じバーガーの統計を同時に計算し直す 2 つの処理(複数の
// インスタンスのワーカーなど)が、互いの追加分を知らないまま「読んでから上書き」して、片方の更新を
// 取りこぼすのを防ぐため。同じトランザクションがすでに持っているロックを取り直しても待たされない。
//
// 1 つのトランザクションで複数のバーガーを計算し直すときは、バーガー ID の昇順に呼ぶこと。
// 別々の処理が同じバーガーを逆の順序でロックすると、互いに相手のロックを待ち合って止まる
// (デッドロック)ため。対象のレビューが 0 件でも、件数 0 の統計を保存する(統計の行が
// なくなると、画面に出す値が決まらなくなる)。
func (r *BurgerStatsRecalculator) Recalculate(ctx context.Context, tx Tx, burgerID string) error {
	if err := tx.BurgerStats.Lock(ctx, burgerID); err != nil {
		return fmt.Errorf("recalculate burger stats: lock burger: %w", err)
	}
	facts, err := tx.Stats.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return fmt.Errorf("recalculate burger stats: list facts: %w", err)
	}
	// データベースの時刻型(timestamptz)はマイクロ秒までしか持てない。ここで切り詰めておくと、
	// 保存された計算時刻が、スコアの計算に使った時刻とちょうど一致し、保存された値から統計を
	// 検算できる。
	now := r.clock.Now().Truncate(time.Microsecond)
	if err := tx.BurgerStats.Save(ctx, domain.CalculateBurgerStat(burgerID, facts, now)); err != nil {
		return fmt.Errorf("recalculate burger stats: save: %w", err)
	}
	return nil
}
