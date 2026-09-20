package usecase

import "math"

// shop と review の一覧 use case で共有する、ページネーションの
// 境界値（Rails parity）。
const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// clampPage は、生の page/perPage のクエリ値を、共通のフォールバック規則で
// repository の limit/offset に変換する。page < 1 は 1 になり、perPage < 1 は
// 20 になり、perPage は 100 が上限となる。遠く離れたページは空のリストを
// 返す。掛け算の前に page を clamp することで、積（最大でも (2^31-1)*100）が
// int64 に収まり、offset を clamp することで、その結果を変えずに int32 に
// 収まる。
func clampPage(page, perPage int) (limit, offset int32) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	if page > math.MaxInt32 {
		page = math.MaxInt32
	}
	off := min(int64(page-1)*int64(perPage), math.MaxInt32)
	return int32(perPage), int32(off)
}
