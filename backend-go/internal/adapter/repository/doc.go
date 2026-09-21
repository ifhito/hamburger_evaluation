// Package repository は、domain で宣言された repository の interface を、
// pgx 上の sqlc 生成コード（sqlcgen）を用いて実装する。
//
// 読み取りは adapter/query が担い、この package は書き込み（CUD と、書き込みの前段の
// 排他ロック）だけを実装する。burger の統計の再計算は行わない（トランザクションを持つ
// usecase が、UnitOfWork の中で、BurgerStatRepository と組み合わせて行う）。呼び出すのは domain のコード（書き込みオブジェクトの domain.Shops など）だけで、
// usecase から直接呼ばれることはない。
//
// sqlc の出力は sqlcgen サブパッケージにあり、手で編集することはない。
// 代わりに db/queries/ を変更して再生成する。
package repository
