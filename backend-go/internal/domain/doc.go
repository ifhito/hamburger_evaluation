// Package domain は entity、value object、domain error を保持する。
//
// 標準ライブラリだけを import する。HTTP も SQL ドライバも持たず、
// usecase や adapter のパッケージからの import も持たない。後続の story で
// 追加される。
package domain
