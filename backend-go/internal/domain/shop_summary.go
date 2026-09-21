package domain

// ShopSummary は、ショップに紐づくレビューから求める集計で、一覧と詳細に添える。
//
// 集計の対象は、ショップ詳細に出るレビューと同じ範囲(削除されていないレビューのうち、書いた
// 利用者も退会していないもの)である。範囲の外のレビューは、件数にも平均にも写真にも入らない。
// ここは集計の意味の定義(範囲・丸め・写真の選び方)で、範囲の絞り込みと写真の選び方は、1 回の集約で
// 済ませるため、adapter/query の SQL(ListShopSummaries)が実装している。丸めだけは、ここが持つ。
type ShopSummary struct {
	// ReviewCount は、対象のレビューの件数である。
	ReviewCount int64
	// AverageRating は、対象のレビューの評価の平均(小数 1 桁。RoundAverageRating)である。
	// レビューがないときは nil。
	AverageRating *float64
	// PhotoKey は、ショップの写真として使う写真の保存キーである。
	// ショップの写真は、対象のレビューのうち、写真つきで最も新しいレビューの写真とする
	// (投稿の新しい順、同じ時刻なら id の新しい順)。写真つきのレビューがないときは nil。
	// ショップ専用の写真は持たない。
	PhotoKey *string
	// PhotoURL は PhotoKey の公開 URL で、写真がないときは nil である。usecase が
	// (写真の保存先を介して)導く。domain は URL を組み立てない。
	PhotoURL *string
}

// RoundAverageRating は、評価の平均を、小数 1 桁に丸める(0.05 は切り上げ。バーガーの統計と同じ丸め方)。
func RoundAverageRating(average float64) float64 {
	return roundHalfAwayFromZero(average, 10)
}

// NewShopSummary は、レビューの件数・評価の平均(まだ丸めていない値。レビューがないときは使わない)・
// 写真のキーから集計を作る。レビューがなければ、平均は nil になる。
func NewShopSummary(reviewCount int64, average float64, photoKey *string) ShopSummary {
	summary := ShopSummary{ReviewCount: reviewCount, PhotoKey: photoKey}
	if reviewCount > 0 {
		rounded := RoundAverageRating(average)
		summary.AverageRating = &rounded
	}
	return summary
}

// ShopListing は、一覧に出るショップと、その集計である。
type ShopListing struct {
	Shop
	Summary ShopSummary
}
