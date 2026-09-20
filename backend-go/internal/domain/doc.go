// Package domain は entity、value object、domain error、そして書き込みの
// 窓口（repository の interface と、それを呼ぶサービス）を保持する。
//
// repository の interface（*Repository。Create* / Update* / Discard* だけを
// 持つ書き込み専用の契約）はこの package が宣言し、呼び出すのは同じ package の
// サービス（ShopService、ReviewService、UserService）だけである。usecase は
// repository に依存せず、書き込みをこれらのサービスに任せる。実装は
// adapter/repository が担う。
//
// ドメインサービスのルール（肥大化を防ぐための決まり。構造検査のテストで一部を強制する）:
//   - 役割: repository を呼ぶ唯一の場所で、永続化を伴う書き込みの窓口である。業務の判断は、
//     可能な限りエンティティ・値オブジェクトのメソッド（永続化に触れない純粋な処理）に置き、
//     サービスは「その結果を永続化につなぐ」薄い層に保つ。
//   - 粒度: 集約ごとに 1 つ（ShopService、ReviewService、UserService）。持つ repository は
//     自分の集約のものだけである（検査で強制）。複数の集約にまたがる手順は、片方のサービスに
//     押し込まず、usecase が複数のサービスを呼んで組み立てる。
//   - 持つもの: 書き込みの操作（Create / Update / Discard に対応するもの）と、それに付随する
//     業務の不変条件・状態遷移。持たないもの: 読み取り（usecase の *Query）、認可（誰が
//     できるか。usecase かエンティティのメソッド）、HTTP・DTO・写真などの外部 I/O。
//   - 1 メソッド = 1 つの業務上の書き込み。公開メソッドは 1 サービスあたり 8 個まで（検査で
//     強制）。超えるときは、ロジックをエンティティへ移す、集約を分ける、を先に検討し、
//     上限を上げる場合は理由をレビューで示す。
//   - 新しい書き込みが必要になったら、まずエンティティ・値オブジェクトで表現できるかを
//     考え、永続化が要るときだけサービスにメソッドを足す。
//
// 標準ライブラリだけを import する。HTTP も SQL ドライバも import せず、
// usecase や adapter のパッケージも import しない。
package domain
