package query

// trimPage は、limit+1 件で取得した行を limit 件に切り詰め、続き（次のページ）が
// あるかを返す。一覧の query は、次のページの有無を知るために 1 件多く取得する。
// limit は usecase の clampPage で高々 100 に補正済みなので、+1 で桁あふれしない。
func trimPage[T any](rows []T, limit int32) ([]T, bool) {
	if len(rows) > int(limit) {
		return rows[:limit], true
	}
	return rows, false
}
