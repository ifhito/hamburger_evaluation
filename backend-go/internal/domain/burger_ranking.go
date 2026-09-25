package domain

// BurgerRanking は GET /burgers の一覧に現れる 1 件である。review が 1 件もない
// (burger_stats が未計算、または削除で 0 件に戻った)バーガーは対象外(query 側が除外する)。
// これは保存された値を読んだだけの純粋な射影で、業務の判断は持たない(repository も
// 書き込みオブジェクトもない)。
type BurgerRanking struct {
	// ID は burger の UUID の正規形である。
	ID   string
	Name string
	// Shop は、この burger に紐づく代表のショップである。複数の active な shop に
	// 紐づくときは、作成が最も古い(created_at 昇順、同時刻は id 昇順)ものを採る。
	// これは、匿名の閲覧者に対して ReviewShopFor が選ぶショップ(見えるショップの
	// 先頭)と同じ選び方である(この一覧には閲覧者がなく、常に匿名と同じ扱いになる)。
	// PhotoKey は最新の有効な写真付きレビューのキー、PhotoURL は公開 URL。写真なしは nil。
	PhotoKey      *string
	PhotoURL      *string
	Shop          ShopRef
	AverageRating float64
	WeightedScore float64
	ReviewCount   int64
}
