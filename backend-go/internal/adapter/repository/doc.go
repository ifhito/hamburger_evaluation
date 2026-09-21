// Package repository は、domain で宣言された repository の interface を、
// pgx 上の sqlc 生成コード（sqlcgen）を用いて実装する。
//
// 読み取りは adapter/query が担い、この package は書き込み（登録・更新・論理削除と、書き込みの前に
// 行う排他ロック)だけを実装する。バーガーの統計の再計算は行わない。レビューの書き込みと同じ
// トランザクションで、usecase が UnitOfWork(ここからここまでをまとめて 1 つのトランザクションに
// する範囲を、usecase が指定する仕組み)の中で、BurgerStatRepository と組み合わせて行う。
// 呼び出すのは domain のコード（書き込みオブジェクトの domain.Shops など）だけで、
// usecase から直接呼ばれることはない。
//
// sqlc の出力は sqlcgen サブパッケージにあり、手で編集することはない。
// 代わりに db/queries/ を変更して再生成する。
//
// この package のテストは、書き込み（CUD）の結果を、adapter/query に頼らず SQL の SELECT で
// 直接確かめる。読み取りの adapter/query を import しない（テストを含む。usecase の構造検査が
// 守る）。読み取りの検証は adapter/query のテストが担い、両方を通した確認は handler の
// 統合テストが担う。
package repository
