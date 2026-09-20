# Lens: Consistency

層をまたぐ**ルールの一貫性**(判断の置き場所が 1 か所か。下の 9・10)も、このレンズで見る。

複数ステップの書き込みはすべてアトミックに、並行書き込みはすべて安全に、
それ以外はすべてトランザクションの外に。各書き込みパスに問う: 「途中で
死んだらどんな状態が残るか?」「同時に 2 つ走ったらどうなるか?」

## 探すもの

1. **アトミック性の欠如** — トランザクションなしの関連書き込み(親 + 子、
   書き込み + プロジェクション、削除 + クリーンアップ)。半分だけの
   コミットは破損である。
2. **read-modify-write の競合** — load、compute、save は並行更新を失う。
   アトミックな SQL(`SET count = count + 1`)、期待状態への `WHERE` ガード、
   または tx 内の `SELECT ... FOR UPDATE` を優先する。
3. **tx 内の無関係な仕事** — ロック保持中の HTTP 呼び出し、ファイル I/O、
   sleep。遅い依存先がデータベースのストールになる。
4. **ロールバックが保証されない** — `Commit` の前に `defer tx.Rollback(ctx)`
   のない pgx `Begin`。開いたままの tx をリークさせる early return。
5. **冪等性のないリトライ** — リトライされると二重 INSERT や二重カウントに
   なる操作。リトライを支える unique 制約か upsert を探す。
6. **check-then-act の一意性** — unique インデックス + `ON CONFLICT` の
   代わりに SELECT-then-INSERT。
7. **古いプロジェクション** — 派生データ(カウント、統計)がソースを変更した
   tx の外で更新される、あるいは古いスナップショットから再計算される。
8. **マイグレーションの安全性** — バックフィル + 制約を不可逆な 1 ステップに
   まとめる。ホットなテーブルをロックする長時間マイグレーション。
9. **ルールの二重管理** — frontend の検証・権限の条件・定数・導出が、backend の `domain` の
   判断と重なっている(例: Zod のスキーマによる検証、`review.userId === authUser.id` の比較、
   `PER_PAGE` と件数の比較、backend が返す値の再計算)。判断は backend の domain だけが持ち、
   frontend は backend が返した値(422 のメッセージ、`can_*`、`has_more` など)を表示する。
   複製は、片方だけ直したときに食い違う。
10. **repository への依存** — usecase が repository を宣言・保持・呼び出している(`.repo.` の
    呼び出し、`*Repository` の型・フィールド、`domain.*Repository` の参照)。repository は
    domain のサービスからだけ使い、usecase は読み取りを `*Query`、書き込みを domain の
    サービスで行う。

## 指摘しないもの

- 単一ステートメントの書き込み(すでにアトミック)。
- ステールさが文書化され再構築パスのある、意図的に結果整合な
  プロジェクション。
- コミット後の付随的な仕事(通知、キャッシュ無効化)— それが正しい置き場所。
- HTML 標準の属性(`type="email"`、`required`)、表示のための整形(日付・桁)、ルートの導線としての
  redirect、規則を利用者に伝える説明文(対応する backend の定数がコメントで示されているもの)。

## Grep の起点

```bash
grep -rn 'Begin\|BeginTx' backend-go/internal/ --include='*.go'
grep -rn 'ON CONFLICT\|FOR UPDATE' backend-go/db/queries/
grep -rnE '\.repo\.|domain\.[A-Za-z]*Repository|[A-Za-z]+Repository +(interface|struct)' backend-go/internal/usecase --include='*.go' | grep -v _test
grep -rnE 'superRefine|\.min\(|\.max\(|\.email\(|\.refine\(|userId ===|PER_PAGE' frontend/src
```
