package domain

// ShopSummary は、ショップの集計(件数・平均・写真)で、一覧と詳細に添える。保存された集計(ShopStat)を
// 読み取ったもの(読み取りの結果)である。集計は、レビューの書き込みのあとに、バックグラウンドのワーカーが
// 計算し直す(結果整合)ので、書き込みの直後は、数秒のあいだ、古いことがある。まだ集計されていない
// ショップは、件数 0・平均と写真なしの、空の集計になる。集計の意味(数える範囲・平均・写真の選び方)は、
// CalculateShopStat が持つ。
type ShopSummary struct {
	// ReviewCount は、対象のレビューの件数である。
	ReviewCount int64
	// AverageRating は、対象のレビューの評価の平均(小数 1 桁)である。レビューがないときは nil。
	AverageRating *float64
	// PhotoKey は、ショップの写真として使う写真の保存キーである(写真つきで最も新しいレビューの写真)。
	// 写真つきのレビューがないときは nil。ショップ専用の写真は持たない。
	PhotoKey *string
	// PhotoURL は PhotoKey の公開 URL で、写真がないときは nil である。usecase が
	// (写真の保存先を介して)導く。domain は URL を組み立てない。
	PhotoURL *string
}

// RoundAverageRating は、評価の平均を、小数 1 桁に丸める(0.05 は切り上げ。バーガーの統計と同じ丸め方)。
func RoundAverageRating(average float64) float64 {
	return roundHalfAwayFromZero(average, 10)
}

// ShopListing は、一覧に出るショップと、その集計である。
type ShopListing struct {
	Shop
	Summary ShopSummary
}
