// Package domain は entity、value object、domain error を保持する。
//
// 標準ライブラリだけを import する。HTTP も SQL ドライバも import せず、
// usecase や adapter のパッケージも import しない。後続の story で
// 追加される。
package domain
