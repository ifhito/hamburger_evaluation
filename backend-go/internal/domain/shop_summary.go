package domain

import "math"

// ShopSummary は、ショップに紐づくレビューから求める集計で、一覧と詳細に添える。
//
// 集計の対象は、ショップ詳細に出るレビューと同じ範囲(削除されていないレビューのうち、書いた
// 利用者も退会していないもの)である。範囲の外のレビューは、件数にも平均にも写真にも入らない。
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

// RoundAverageRating は、評価の平均を、小数 1 桁に丸める(0.05 は切り上げ)。
func RoundAverageRating(average float64) float64 {
	return math.Round(average*10) / 10
}

// NewShopSummary は、レビューの件数・評価の平均(まだ丸めていない値。レビューがなければ nil)・
// 写真のキーから集計を作る。
func NewShopSummary(reviewCount int64, average *float64, photoKey *string) ShopSummary {
	summary := ShopSummary{ReviewCount: reviewCount, PhotoKey: photoKey}
	if reviewCount > 0 && average != nil {
		rounded := RoundAverageRating(*average)
		summary.AverageRating = &rounded
	}
	return summary
}

// ShopListing は、一覧に出るショップと、その集計である。
type ShopListing struct {
	Shop
	Summary ShopSummary
}
