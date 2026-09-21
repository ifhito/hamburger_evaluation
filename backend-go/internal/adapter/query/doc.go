// Package query は、usecase で宣言された *Query（読み取り専用）の interface を、
// pgx 上の sqlc 生成コード（sqlcgen）を用いて実装する。書き込みは
// adapter/repository が担う。
//
// sqlc の出力は adapter/repository/sqlcgen にあり、手で編集することはない。
// 代わりに db/queries/ を変更して再生成する。
//
// この package のテストは、読み取り（絞り込み・順序・ページング・可視性・join）だけを実際の
// PostgreSQL で検証し、データは SQL の INSERT で用意する。書き込みの adapter/repository を
// import しない（テストを含む。usecase の構造検査が守る）。
package query
