// Package repository は、usecase で宣言された repository の interface を、
// pgx 上の sqlc 生成コード（sqlcgen）を用いて実装する。
//
// sqlc の出力は sqlcgen サブパッケージにあり、手で編集することはない。
// 代わりに db/queries/ を変更して再生成する。中身は後続のストーリーで
// 追加される。
package repository
