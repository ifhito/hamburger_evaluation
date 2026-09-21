package domain

import (
	"math"
	"regexp"
	"strconv"
)

// 一覧のページ送りの規則。ページ送りする一覧(GET /shops・GET /reviews・GET /oauth/grants)は、すべて
// この規則を使う。usecase・handler・adapter は、ここの関数を呼ぶだけで、件数の既定値・上限・不正な値の丸め方・
// 続きがあるかの判定を、自分では持たない(frontend も、件数を知らず、API の X-Has-More に従うだけである)。
const (
	// DefaultPerPage は、1 ページの件数の指定がない(または 1 未満の)ときの件数である。
	DefaultPerPage = 20
	// MaxPerPage は、1 ページの件数の上限である。これを超える指定は、この件数に丸められる。
	MaxPerPage = 100
)

// pageIntegerPattern は、page / per_page が整数として正しい構文(符号は省略可、あとは ASCII の数字だけ)である
// ことを判定する。桁数は問わない。
var pageIntegerPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)

// ParsePageParams は、一覧の page / per_page の、query の生の値(省略は空文字)を整数にする。
// 空の値は 0(指定なしを表し、PageBounds で既定値になる)で、エラーではない。`[+-]?[0-9]+` の形の値が整数で、
// int の範囲を超える整数は、strconv.Atoi が返す範囲内の値(math.MaxInt / math.MinInt)にそのまま丸められる
// (そのあとの丸めは PageBounds が行う)。形が合わない値があれば、*ValidationError を返す(両方が不正なら、
// page、per_page の順に両方のメッセージを並べる)。
func ParsePageParams(rawPage, rawPerPage string) (page, perPage int, err error) {
	var msgs []string
	parse := func(raw, msg string) int {
		if raw == "" {
			return 0
		}
		if !pageIntegerPattern.MatchString(raw) {
			msgs = append(msgs, msg)
			return 0
		}
		n, _ := strconv.Atoi(raw) // 構文は検証済みなので、エラーは範囲外だけである。そのとき Atoi は範囲内の値を返す
		return n
	}
	page = parse(rawPage, "Page must be an integer")
	perPage = parse(rawPerPage, "Per page must be an integer")
	if len(msgs) > 0 {
		return 0, 0, &ValidationError{Messages: msgs}
	}
	return page, perPage, nil
}

// PageBounds は、page / perPage(ParsePageParams の結果。範囲外でもよい)を、一覧の読み取りに渡す
// limit(1 ページの件数)と offset(飛ばす件数)にする。page < 1 は 1、perPage < 1 は DefaultPerPage、
// perPage が MaxPerPage を超えるときは MaxPerPage に丸める。遠く離れたページは、空のリストになる。
// 掛け算の前に page を丸めることで、積(最大でも (2^31-1)*100)が int64 に収まり、offset を丸めることで、
// 結果を変えずに offset が int32 に収まる。
func PageBounds(page, perPage int) (limit, offset int32) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	if page > math.MaxInt32 {
		page = math.MaxInt32
	}
	off := min(int64(page-1)*int64(perPage), math.MaxInt32)
	return int32(perPage), int32(off)
}

// PageFetchLimit は、1 ページの件数 limit の一覧を読み取るときに、実際に取り出す件数である。次のページが
// あるかを知るために、1 件多く取り出す(TrimPage が切り詰める)。limit は PageBounds の結果(高々 MaxPerPage)
// なので、+1 で桁あふれしない。
func PageFetchLimit(limit int32) int32 {
	return limit + 1
}

// TrimPage は、PageFetchLimit 件で取り出した rows を limit 件に切り詰め、続き(次のページ)があるかを返す。
func TrimPage[T any](rows []T, limit int32) ([]T, bool) {
	if len(rows) > int(limit) {
		return rows[:limit], true
	}
	return rows, false
}
