# コンテキストマップ

Hamburger Evaluation の軽量 DDD 境界を示します。
Rails の単一アプリ内で運用するため、厳密なモジュール分離ではなく、依存方向と責務の分離を明確にすることを目的にします。

## 境界づけられたコンテキスト

```text
[Controllers]
    |
    | commands
    v
[Reviews Application Services] ---- uses port/repository ----> [Shops / Catalog]
    |
    | review changed
    v
[BurgerStats Projection]
    |
    | uses pure scoring policy
    v
[Reviews Domain Scoring]
```

## Reviews コンテキスト

責務:

- レビュー作成、更新、削除
- レビュー検索条件の受け取り
- Rating の妥当性
- ReviewerTrust の算出ルール
- BurgerScore の算出ルール

配置方針:

- 純粋な計算・値オブジェクトは `app/domain/reviews/`
- ユースケースは `app/services/reviews/`
- DB 検索・永続化は `app/queries/reviews/` または `app/repositories/reviews/`

Reviews domain に置くもの:

- `Reviews::Rating`
- `Reviews::BurgerScore`
- `Reviews::BurgerScoreCalculator`
- `Reviews::ReviewerTrust`
- `Reviews::ReviewerTrustEvaluator`

Reviews domain に置かないもの:

- ActiveRecord query
- `Review.kept`, `joins`, `includes`, `where`, `pluck`
- `Shop.find`
- transaction / lock

## Shops / Catalog コンテキスト

責務:

- 店舗一覧、店舗詳細
- 店舗が提供するバーガーの特定
- レビュー作成時に必要な評価対象バーガーの解決

Reviews から見た依存:

- Reviews は `shop_id` と `burger_name` を渡し、評価対象バーガーを取得または作成してもらう
- Reviews のユースケースは `Shop.find` や `shop.burgers.find_by` を直接呼ばない
- これらは repository/port に隠す

## BurgerStats コンテキスト

責務:

- レビュー変更に伴うバーガー統計の再計算
- 読み取り用 projection としての `BurgerStat` 更新

依存関係:

- BurgerStats は Reviews のスコア計算ポリシーを利用する
- ドメイン計算器には ActiveRecord model ではなく `Reviews::ReviewFact` / `Reviews::ReviewerHistory` を渡す
- `BurgerStat` projection の読み取り・永続化詳細は `BurgerStats::BurgerStatRepository` に閉じ込める

## 依存方向のルール

許可する依存:

- Controller -> Application Service / Query
- Application Service -> Domain object
- Application Service -> Repository/Query port
- Repository/Query -> ActiveRecord
- Projection Service -> Domain scoring policy

避ける依存:

- Domain -> ActiveRecord
- Domain -> Controller
- Domain -> Serializer
- Domain -> Job
- Model callback -> 複雑なユースケース直接実行
- Application Service -> 複数の ActiveRecord 詳細を直接操作

## 現在の改善済み方針

- `Reviews::ReviewQuery` は `app/queries/reviews/` に置く
- レビュー検索は domain ではなく query の責務とする
- レビュー作成時の店舗検索、ロック、バーガー検索・作成、レビュー永続化は repository に寄せる
- `Reviews::CreateReviewService` はユースケースの流れを表し、永続化詳細へ直接依存しない
- `Reviews::BurgerScoreCalculator` は `Burger` ではなく `ReviewFact` を入力に取る
- `Reviews::ReviewerTrustEvaluator` は `User` ではなく `ReviewerHistory` を入力に取る
- `BurgerStats::BurgerStatRepository` が統計 projection の読み取り・永続化を担当する
- `Review` model callback は job を直接 enqueue せず、`Reviews::ReviewEvents` にレビュー変更を通知する

## 今後の追加改善候補

1. `ShopsAndBurger` のドメイン上の呼称を Offering / ShopBurger として整理し、必要に応じてモデル名も変更する
2. Auth を Identity / Authentication コンテキストとして分離する
3. Users のプロフィール更新・退会ユースケースを Users コンテキストとして明確化する
