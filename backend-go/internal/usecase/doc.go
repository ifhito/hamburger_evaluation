// Package usecase は、アプリケーションの use case（auth、shops、reviews、
// users）と、それらが consume する interface（読み取りの Query は
// adapter/query、パスワードのハッシュ化とトークンは adapter/infra、写真の
// blob storage は adapter/storage で実装される）を保持する。
//
// 読み取りは usecase が宣言する *Query を通す。書き込みは domain の書き込み
// オブジェクト（domain.Shops など）を通し、repository には依存しない。repository の
// interface は domain が宣言し、それを呼ぶのは domain のコードだけである。
//
// レビューの保存とバーガーの統計の再計算、ユーザーの退会と統計の再計算のように、両方成功した
// ときだけ確定し、途中で失敗したら両方取り消したい手順は、usecase が UnitOfWork の中で組み立てる。
// UnitOfWork(作業のひとまとまり。ここからここまでの書き込みと読み取りを、まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組み。途中でエラーになれば全体を取り消す(rollback)ので、片方だけ反映されることがない)。
// 宣言は usecase が持ち(unit_of_work.go)、実装は adapter/uow が担う。UnitOfWork の中で渡される
// Tx(同じトランザクションに結び付いた書き込みと読み取り)の書き込みは domain の書き込み
// オブジェクトで、読み取りは usecase の Query なので、この手順も repository には依存しない。
// 統計を計算し直す手順は BurgerStatsRecalculator(「再計算する役」の意味)が持つ。
//
// import するのは domain、photo（標準ライブラリと golang.org/x/image だけに
// 依存する、アップロード画像の検証・正規化）、標準ライブラリだけである。
// このパッケージ自身は HTTP も SQL ドライバも使わず、adapter パッケージから
// の import もない。
package usecase
