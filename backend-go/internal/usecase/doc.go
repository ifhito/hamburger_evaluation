// Package usecase は、アプリケーションの use case と、それらが consume する
// repository の interface（adapter/repository で実装される）を保持する。
//
// import するのは domain と標準ライブラリだけである。HTTP も SQL ドライバも
// 使わず、adapter パッケージからの import もない。中身は後続の story で
// 埋められる。
package usecase
