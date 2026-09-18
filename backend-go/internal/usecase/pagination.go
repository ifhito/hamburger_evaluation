package usecase

import "math"

// Pagination bounds shared by the shop and review list use cases (Rails
// parity).
const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// clampPage converts the raw page/perPage query values into repository
// limit/offset with the shared fallback rules: page < 1 becomes 1,
// perPage < 1 becomes 20, and perPage is capped at 100. Far-out pages
// yield an empty list; clamping page before the multiplication keeps the
// product (at most (2^31-1)*100) inside int64, and clamping the offset
// keeps it in int32 without changing that outcome.
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
