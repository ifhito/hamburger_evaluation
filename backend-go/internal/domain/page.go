package domain

// 一覧のページ送りの、業務の規則(1 ページに何件まで見せるか)。ページ送りする一覧
// (GET /shops・GET /reviews・GET /oauth/grants)は、すべてこの規則に従う。
// query の文字列を整数に読むこと(handler)や、続きがあるかを知るために 1 件多く取り出すこと
// (usecase・adapter)は、この規則を実現する手段なので、ここでは持たない。
// frontend も件数を知らず、API の X-Has-More に従うだけである。
const (
	// DefaultPerPage は、1 ページの件数の指定がない(または 1 未満の)ときの件数である。
	DefaultPerPage = 20
	// MaxPerPage は、1 ページの件数の上限である。これを超える指定は、この件数に丸められる。
	MaxPerPage = 100
)

// NormalizePage は、一覧の page / perPage(query の値。範囲外でもよく、0 は指定なしを表す)を、
// サービスが許す範囲に丸める。page < 1 は 1、perPage < 1 は DefaultPerPage、perPage が MaxPerPage を
// 超えるときは MaxPerPage にする。エラーにはしない(範囲外の指定は、丸めて受け付ける)。
// page の上限は決めない(遠く離れたページは、空のリストになる)。
func NormalizePage(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	return page, perPage
}
