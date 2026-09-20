// Package domain は entity、value object、domain error、そして書き込みの
// 窓口（repository の interface と、それを呼ぶサービス）を保持する。
//
// repository の interface（*Repository。Create* / Update* / Discard* だけを
// 持つ書き込み専用の契約）はこの package が宣言し、呼び出すのは同じ package の
// サービス（ShopService、ReviewService、UserService）だけである。usecase は
// repository に依存せず、書き込みをこれらのサービスに任せる。実装は
// adapter/repository が担う。
//
// 標準ライブラリだけを import する。HTTP も SQL ドライバも import せず、
// usecase や adapter のパッケージも import しない。
package domain
