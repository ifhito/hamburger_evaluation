package usecase

import (
	"math"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// clampPage は、生の page/perPage のクエリ値を、query（ShopQuery.ListShops /
// ReviewQuery.ListReviews など）に渡す limit/offset に変換する。1 ページの件数の
// 既定値・上限と、範囲外の値の丸め方は、業務の規則として domain.NormalizePage が決める
// （page < 1 は 1、perPage < 1 は 20、perPage の上限は 100）。ここが持つのは、その結果を
// DB の int32 の limit/offset にする手段だけである。遠く離れたページは空のリストを
// 返す。掛け算の前に page を clamp することで、積（最大でも (2^31-1)*100）が
// int64 に収まり、offset を clamp することで、その結果を変えずに offset が
// int32 に収まる。
func clampPage(page, perPage int) (limit, offset int32) {
	page, perPage = domain.NormalizePage(page, perPage)
	if page > math.MaxInt32 {
		page = math.MaxInt32
	}
	off := min(int64(page-1)*int64(perPage), math.MaxInt32)
	return int32(perPage), int32(off)
}
