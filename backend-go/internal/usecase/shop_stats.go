package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopStatsQuery は、ショップの集計を計算し直すために必要な、読み取り専用の窓口である。UnitOfWork の中では、
// そのトランザクションに結び付いた実装が渡されるので、同じトランザクションの、まだ確定していない書き込みも
// 読み取れる。書き込みのメソッドは置かない(書き込みは domain.ShopStats を通す)。
type ShopStatsQuery interface {
	// ListShopReviewFacts は、ショップの集計の元になるレビューを、計算用の値(domain.ShopReviewFact)にして返す。
	// 削除済みのレビューと、削除済みのユーザーが書いたレビューは含めない(ショップ詳細に出るレビューと同じ範囲)。
	// バーガーは複数のショップにありうるので、ショップのすべてのバーガーのレビューをまとめて返す。それぞれの値には、
	// そのレビューの投稿者が、すべてのバーガーに付けた有効な評価(投稿者の信頼度の計算に使う履歴)を添える。
	ListShopReviewFacts(ctx context.Context, shopID string) ([]domain.ShopReviewFact, error)
	// ListBurgerShopIDs は、バーガーが紐づくショップの ID を、重複なしで昇順に返す。バーガーの統計を計算し直した
	// あと、集計の再計算を依頼する対象を知るために使う。
	ListBurgerShopIDs(ctx context.Context, burgerID string) ([]string, error)
	// ListDueShopRecalcRequests は、再計算の時期が来ているショップの依頼を、上限 batch 件まで返す。
	// 次の再試行の時刻が now 以前(または未設定)で、失敗の回数が maxAttempts に達していないものが
	// 対象である(達したものは打ち切り)。トランザクションの外の読み取りでもよい。
	ListDueShopRecalcRequests(ctx context.Context, now time.Time, maxAttempts, batch int) ([]domain.ShopRecalcRequest, error)
}

// ShopStatsRecalculator(「ショップの集計を再計算する役」の意味)は、ショップの集計の再計算に関する手順を
// まとめる。
//
// 集計は、レビューの書き込みの中では計算しない。書き込みは、バーガーの統計の再計算の依頼を登録するだけで、
// バーガーの統計を計算し直したワーカー(StatsWorker)が、同じトランザクションで、そのバーガーが紐づくショップの
// 集計の再計算の依頼を登録する(RequestRecalculationForBurger)。ショップの集計は、その依頼を取り出した
// ワーカー(ShopStatsWorker)が、あとから Recalculate で計算する。そのため、書き込みの応答は集計の計算を待たず、
// 集計は少し遅れて(通常は数秒以内に)反映される(結果整合。バーガーの統計と同じ)。
//
// 依頼をショップごとにまとめる(同じショップの別のバーガーからの依頼も 1 件になる)のは、ショップの集計が、
// そのショップのすべてのレビュー(と、投稿者の過去の評価)を読む重い計算で、バーガーごとに繰り返さないためである。
type ShopStatsRecalculator struct {
	clock Clock
}

// NewShopStatsRecalculator は clock を使う ShopStatsRecalculator を返す。
func NewShopStatsRecalculator(clock Clock) *ShopStatsRecalculator {
	return &ShopStatsRecalculator{clock: clock}
}

// RequestRecalculationForBurger は、burger が紐づくすべてのショップの集計の再計算を、shop_id の昇順に依頼する。
// tx は UnitOfWork.Do が渡したものでなければならず、バーガーの統計の再計算と同じトランザクションで登録される
// (どちらかが失敗すれば、両方が取り消される)。集計は計算せず、ショップの行もロックしない。ショップに紐づかない
// バーガーは、依頼を登録せず、成功する。昇順は、複数の行を、いつも同じ順序で登録して、並行する処理どうしが
// 互いの行を逆順に待ってデッドロックすることがないようにするためである。
func (r *ShopStatsRecalculator) RequestRecalculationForBurger(ctx context.Context, tx Tx, burgerID string) error {
	shopIDs, err := tx.ShopStatsReads.ListBurgerShopIDs(ctx, burgerID)
	if err != nil {
		return fmt.Errorf("request shop stats recalculation: list burger shops: %w", err)
	}
	slices.Sort(shopIDs)
	for _, shopID := range shopIDs {
		if err := tx.ShopStats.RequestRecalc(ctx, shopID); err != nil {
			return fmt.Errorf("request shop stats recalculation: %w", err)
		}
	}
	return nil
}

// Recalculate はショップの集計を計算し直して保存する(ショップの集計のワーカーが使う)。tx は UnitOfWork.Do が
// 渡したものでなければならない。手順は「ショップの行をロックする → 集計の元データを読む → domain の計算
// (CalculateShopStat)で集計を求める → 保存する」である。
//
// 最初にショップの行をロックするのは、同じショップの集計を同時に計算し直す 2 つの処理(複数のインスタンスの
// ワーカーなど)が、互いの追加分を知らないまま「読んでから上書き」して、片方の更新を取りこぼすのを防ぐため。
// ショップがなくなっていたとき(依頼のあとに消された)は、何もせずに成功する(依頼は、呼び出し側が消す)。
// 対象のレビューが 0 件でも、件数 0 の集計を保存する。
func (r *ShopStatsRecalculator) Recalculate(ctx context.Context, tx Tx, shopID string) error {
	if err := tx.ShopStats.Lock(ctx, shopID); err != nil {
		if errors.Is(err, domain.ErrShopNotFound) {
			return nil
		}
		return fmt.Errorf("recalculate shop stats: lock shop: %w", err)
	}
	facts, err := tx.ShopStatsReads.ListShopReviewFacts(ctx, shopID)
	if err != nil {
		return fmt.Errorf("recalculate shop stats: list facts: %w", err)
	}
	// データベースの時刻型(timestamptz)はマイクロ秒までしか持てない。ここで切り詰めておくと、保存された
	// 計算時刻が、重みの計算に使った時刻とちょうど一致し、保存された値から集計を検算できる。
	now := r.clock.Now().Truncate(time.Microsecond)
	if err := tx.ShopStats.Save(ctx, domain.CalculateShopStat(shopID, facts, now)); err != nil {
		return fmt.Errorf("recalculate shop stats: save: %w", err)
	}
	return nil
}
