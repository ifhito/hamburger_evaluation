// Package query は、usecase で宣言された *Query（読み取り専用）の interface を、
// pgx 上の sqlc 生成コード（sqlcgen）を用いて実装する。書き込みは
// adapter/repository が担う。
//
// sqlc の出力は adapter/repository/sqlcgen にあり、手で編集することはない。
// 代わりに db/queries/ を変更して再生成する。
package query
