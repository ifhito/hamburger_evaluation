// Package usecase は、アプリケーションの use case（auth、shops、reviews、
// users）と、それらが consume する interface（読み取りの Query は
// adapter/query、パスワードのハッシュ化とトークンは adapter/infra、写真の
// blob storage は adapter/storage で実装される）を保持する。
//
// 読み取りは usecase が宣言する *Query を通す。書き込みは domain のサービス
// （domain.ShopService など）を通し、repository には依存しない。repository の
// interface は domain が宣言し、それを呼ぶのは domain のサービスだけである。
//
// import するのは domain、photo（標準ライブラリと golang.org/x/image だけに
// 依存する、アップロード画像の検証・正規化）、標準ライブラリだけである。
// このパッケージ自身は HTTP も SQL ドライバも使わず、adapter パッケージから
// の import もない。
package usecase
