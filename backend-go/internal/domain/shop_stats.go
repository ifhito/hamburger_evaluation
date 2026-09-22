package domain

import (
	"context"
	"time"
)

// ShopReviewFact は、ショップの集計の元になるレビュー 1 件の、計算用の値である(バーガーの統計の
// ReviewFact に当たる)。読み取りは、集計の値を求めず、この値を返すだけにし、集計の式は
// CalculateShopStat だけが持つ。式を変えても、レビューの読み取りの SQL とスキーマを変えずに済む。
type ShopReviewFact struct {
	// ID は、レビューの UUID の正規形である。写真の選び方で、投稿日時が同じレビューの順序を決める。
	ID     string
	Rating float64
	// CreatedAt は、レビューの投稿日時である(新しさの重みと、写真の選び方に使う)。
	CreatedAt time.Time
	// PhotoKey は、レビューの写真の保存キーで、写真がないときは nil である。
	PhotoKey *string
	// ReviewerHistory は、そのレビューの投稿者が、すべてのバーガーに付けた有効な評価である(投稿者の
	// 信頼度の計算に使う。バーガーの統計と同じ意味で、集計するレビューの範囲とは関係しない)。
	ReviewerHistory ReviewerHistory
}

// ShopStat は、ショップ 1 件の集計(レビューの件数・評価の平均・ショップの写真)と、それを計算した時刻である。
// 集計テーブル(shop_stats)の 1 行に保存される。
type ShopStat struct {
	ShopID string
	// ReviewCount は、対象のレビューの件数(単純な件数)である。
	ReviewCount int64
	// AverageRating は、対象のレビューの評価の加重平均(小数 1 桁)で、レビューがないときは nil。
	AverageRating *float64
	// PhotoKey は、ショップの写真の保存キーで、写真つきのレビューがないときは nil。
	PhotoKey     *string
	CalculatedAt time.Time
}

// CalculateShopStat は、ショップの集計の元になるレビュー(計算用の値 ShopReviewFact の一覧)から、
// now 時点の集計を計算する。ショップの集計の意味は、すべてここで決まる(集計の式を変えるときは、ここだけを
// 変える)。データベースには触れない純粋な計算である。
//
//   - 数える範囲: facts に含まれるレビュー。読み取りは、ショップ詳細に出るレビューと同じ範囲(削除されて
//     いないレビューのうち、書いた利用者も退会していないもの)を渡す。範囲の外のレビューは、件数にも
//     平均にも写真にも入らない。バーガーは複数のショップにありうるので、ショップのレビューは、そのショップの
//     すべてのバーガーのレビューをまとめた 1 つの集合である
//   - 件数: 対象のレビューの件数(単純な件数)
//   - 平均: 対象のレビューを 1 つの集合として、バーガーのスコアと同じ重み付け(投稿者の信頼度 × 半減期 180 日の
//     新しさの減衰)で加重平均し、小数 1 桁に丸める(RoundAverageRating)。バーガーごとの平均の平均ではない。
//     レビューがなければ nil
//   - 写真: 写真つきのレビューのうち、最も新しいレビューの写真(投稿日時が新しい順、同じ時刻なら id が
//     大きい順)。写真つきのレビューがなければ nil。ショップ専用の写真は持たない
func CalculateShopStat(shopID string, facts []ShopReviewFact, now time.Time) ShopStat {
	stat := ShopStat{ShopID: shopID, ReviewCount: int64(len(facts)), CalculatedAt: now}
	if len(facts) == 0 {
		return stat
	}
	weighted := make([]ReviewFact, len(facts))
	var latest *ShopReviewFact
	for i := range facts {
		fact := &facts[i]
		weighted[i] = ReviewFact{Rating: fact.Rating, CreatedAt: fact.CreatedAt, ReviewerHistory: fact.ReviewerHistory}
		if fact.PhotoKey != nil && (latest == nil || newerPhoto(*fact, *latest)) {
			latest = fact
		}
	}
	average, _ := weightedAverageRating(weighted, now)
	average = RoundAverageRating(average)
	stat.AverageRating = &average
	if latest != nil {
		key := *latest.PhotoKey
		stat.PhotoKey = &key
	}
	return stat
}

// newerPhoto は、a のレビューが b より新しい(投稿日時が後、同じなら id が大きい)かを返す。id は UUID の
// 正規形(小文字・同じ長さ)なので、文字列の大小は、データベースの uuid 型の大小と同じである。
func newerPhoto(a, b ShopReviewFact) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

// ShopRecalcRequest は、集計の再計算を待っているショップ 1 件の依頼である(待ち行列の 1 件)。バーガーの
// 統計を計算し直したワーカーが、同じトランザクションで登録し、ショップの集計のワーカーが取り出して
// 集計を計算し直す。同じショップの依頼は、待ち行列に 1 件だけあり、同じショップの別のバーガーからの依頼も
// 1 件にまとまる。バーガーの依頼(RecalcRequest)と同じ仕組みで、version・attempts の意味も同じである。
type ShopRecalcRequest struct {
	ShopID string
	// Version は、依頼が登録されるたびに、時間をまたいで単調に増える番号である。再計算を終えて依頼を消すときは、
	// 取り出したときの Version と一致する場合だけ消す。再計算の最中に新しい依頼が入ると Version が進むので、
	// その依頼は消えずに残り、次の再計算で最新になる。
	Version int64
	// Attempts は、これまでに再計算に失敗した回数である。
	Attempts int
}

// ---- repository の契約(実装は adapter/repository) ----

// ShopStatRepository は、ショップの集計に対する書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード(書き込みオブジェクトの ShopStats)だけで、usecase は直接呼ばない。集計の元になる
// レビューの読み取りは、usecase が宣言する ShopStatsQuery が担う。書き込み専用で、読み取りのメソッドは置かない。
type ShopStatRepository interface {
	// LockShopStat は、ショップの行をロック(FOR NO KEY UPDATE)して、そのショップの集計の再計算を、
	// 1 つずつ順番に行えるようにする。再計算は「元のレビューを全部読んでから、集計を上書きする」処理なので、
	// ロックがないと、同時に走る 2 つのトランザクションが、相手の追加分を知らないまま読み、後から書いた側が
	// 古い集計で上書きしてしまう(更新の取りこぼし)。ロックは、呼び出し側のトランザクションが終わるまで
	// 保持される。ショップがないときは、wrap された ErrShopNotFound を返す。
	LockShopStat(ctx context.Context, shopID string) error
	// UpdateShopStat は、ショップの集計の行を stat の値で置き換える(行がなければ作る)。
	UpdateShopStat(ctx context.Context, stat ShopStat) error
	// CreateShopStatRecalcRequest は、shop の集計の再計算を依頼する。すでに依頼があれば、version を進め、
	// 失敗の記録(回数・次の再試行の時刻・理由)を消して、最初からやり直す。集計は計算せず、shop の行も
	// ロックしない。存在しない shop は、wrap されたエラーを返す。
	CreateShopStatRecalcRequest(ctx context.Context, shopID string) error
	// DiscardShopStatRecalcRequest は、再計算を終えた依頼を消す。取り出したときの version と一致する場合だけ
	// 消し、消せたかどうかを返す(false は、その間に新しい依頼が入って version が進んだことを表す。依頼は
	// 残るので、次の再計算で最新になる)。
	DiscardShopStatRecalcRequest(ctx context.Context, shopID string, version int64) (bool, error)
	// UpdateShopStatRecalcFailure は、再計算の失敗を記録する(失敗の回数を 1 増やし、次の再試行の時刻と理由を
	// 書く)。取り出したときの version と一致する場合だけ更新し、更新できたかどうかを返す。
	UpdateShopStatRecalcFailure(ctx context.Context, shopID string, version int64, failure RecalcFailure) (bool, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// ShopStats は、ショップの集計の書き込みオブジェクトである。ShopStatRepository を持つのはこの型だけで、
// usecase は repository に依存せず、集計の書き込みをここに任せる。集計を求める計算そのもの
// (CalculateShopStat)は、データベースに触れない純粋な規則である。レビューの読み取りと、ロック・保存を
// 組み合わせた再計算の手順は、トランザクションを持つ usecase が組み立てる。
type ShopStats struct {
	repo ShopStatRepository
}

// NewShopStats は repo を使う ShopStats を返す。
func NewShopStats(repo ShopStatRepository) *ShopStats {
	return &ShopStats{repo: repo}
}

// Lock は、ショップの集計を 1 つずつ順番に計算し直せるように、ショップの行をロックする。
func (s *ShopStats) Lock(ctx context.Context, shopID string) error {
	return s.repo.LockShopStat(ctx, shopID)
}

// Save は、計算済みの集計を保存する(行がなければ作る)。
func (s *ShopStats) Save(ctx context.Context, stat ShopStat) error {
	return s.repo.UpdateShopStat(ctx, stat)
}

// RequestRecalc は、shop の集計の再計算を依頼する(すでに依頼があれば、最初からやり直す)。
// バーガーの統計の再計算と同じトランザクションから呼ぶ。集計は計算しない。
func (s *ShopStats) RequestRecalc(ctx context.Context, shopID string) error {
	return s.repo.CreateShopStatRecalcRequest(ctx, shopID)
}

// CompleteRecalc は、再計算を終えた依頼を消す。取り出したときの version と一致する場合だけ消し、
// 消せたかどうかを返す。
func (s *ShopStats) CompleteRecalc(ctx context.Context, shopID string, version int64) (bool, error) {
	return s.repo.DiscardShopStatRecalcRequest(ctx, shopID, version)
}

// RecordRecalcFailure は、再計算の失敗を記録する。取り出したときの version と一致する場合だけ
// 記録し、記録できたかどうかを返す。
func (s *ShopStats) RecordRecalcFailure(ctx context.Context, shopID string, version int64, failure RecalcFailure) (bool, error) {
	return s.repo.UpdateShopStatRecalcFailure(ctx, shopID, version, failure)
}
