// Package domain は entity、value object、domain error、そして書き込みの
// 契約（repository の interface と、それを持つ書き込みオブジェクト）を保持する。
//
// repository の interface（*Repository。Create* / Update* / Discard* と、書き込みの前段の
// 排他ロックの Lock* だけを持つ書き込み専用の契約）はこの package が宣言し、呼び出せるのはこの package の
// コードだけである（Service という型に限らない）。usecase は repository に
// 依存せず、書き込みをこの package のオブジェクトに任せる。実装は
// adapter/repository が担う。
//
// ファイルは集約ごとに 1 つにまとめる（user.go、review.go、shop.go、signup_verification.go、
// mail_delivery.go）。1 ファイルの中は
// 「エンティティ・値オブジェクト・規則 → repository の interface（と、その引数だけに使う
// パラメータ型）→ 書き込みオブジェクト」の順に並べる。エンティティ・値オブジェクト・規則は、
// repository の interface と書き込みオブジェクトを参照しない。この境界はファイルにも
// package にも現れないので、構造検査のテスト（usecase の TestPersistenceInterfaceNaming）で守る。
//
// 書き込みの置き場所は、更新が何個の集約に触れるかで決まる:
//   - 1 つの集約だけを更新する書き込み: 集約ごとの書き込みオブジェクト（Shops、
//     Reviews、Users、SignupVerifications、MailDeliveries）に置く。Service は作らない。
//   - 複数の集約を跨ぐ更新: 手順の途中で読み取り（usecase の *Query）を挟むかどうかで、置き場所が
//     分かれる。読み取りを挟まない、書き込みだけの手順は、domain の Service（*Service）に置く。
//     読み取りを挟む手順（例: 統計の再計算 = burger の行のロック → 統計の元になるレビューの読み取り →
//     統計の保存。退会 → 影響する burger の一覧の読み取り → 各 burger の再計算の依頼の登録）は、repository だけを持つ
//     Service では表現できない（repository は読み取りを持たない）ので、トランザクションを持つ
//     usecase が、各集約の書き込みオブジェクト（BurgerStats など）と Query を組み合わせて、
//     UnitOfWork(ここからここまでの書き込みと読み取りを、まとめて 1 つのトランザクションにする範囲を、
//     usecase が指定する仕組み。途中でエラーになれば全体を取り消す)の中で組み立てる。
//     現時点では、後者だけがあり、Service は 1 つもない。
//
// 書き込みオブジェクトと Service のルール（肥大化を防ぐための決まり。構造検査のテストで
// 一部を強制する）:
//   - 単一の集約の書き込みに Service を作らない。*Service という名前の型は、2 種類以上の
//     repository を持つときだけ許す（検査で強制）。1 種類だけなら、その集約の書き込み
//     オブジェクトに置く。
//   - 書き込みオブジェクトは、自分の集約の *Repository だけを持つ（検査で強制）。
//     名前は集約の複数形にする（Shops なら ShopRepository。検査で強制）。他の集約の
//     repository を持たない。他の集約に触れる手順は Service に置く。
//   - 持つもの: 書き込みの操作（Create / Update / Discard に対応するもの）と、それに付随する
//     業務の不変条件・状態遷移。持たないもの: 読み取り（usecase の *Query）、認可（誰が
//     できるか。usecase かエンティティのメソッド）、HTTP・DTO・写真などの外部 I/O。
//   - 業務の判断は、可能な限りエンティティ・値オブジェクトのメソッド（永続化に触れない
//     純粋な処理）に置き、書き込みオブジェクトと Service は「その結果を永続化につなぐ」
//     薄い層に保つ。新しい書き込みが必要になったら、まずエンティティ・値オブジェクトで
//     表現できるかを考え、永続化が要るときだけメソッドを足す。
//   - 1 メソッド = 1 つの業務上の書き込み。公開メソッドは 1 つの型あたり 8 個まで（検査で
//     強制）。超えるときは、ロジックをエンティティへ移す、集約を分ける、を先に検討し、
//     上限を上げる場合は理由をレビューで示す。
//
// 標準ライブラリだけを import する。HTTP も SQL ドライバも import せず、
// usecase や adapter のパッケージも import しない。
package domain
