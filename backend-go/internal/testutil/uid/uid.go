// Package uid は、テストで使う決定的な UUID(v4 の形式の正規形)を作る。
// ユーザーの id が UUID になったので、テストの連番の id を置き換えるために使う。
package uid

import "fmt"

// N は n から決まる UUID の正規形(小文字・ハイフン区切り)を返す。同じ n は同じ値になる。
func N(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}
