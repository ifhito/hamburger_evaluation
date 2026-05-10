# ユビキタス言語

この文書は、Hamburger Evaluation のドメイン用語をコード上の責務と対応づけるための用語集です。
実装名ではなく、チームが会話で使う業務上の言葉を優先します。

## Reviews コンテキスト

### レビュー (Review)

ユーザーがバーガーに対して投稿する評価です。
評価値、コメント、投稿者、評価対象バーガーを持ちます。

コード上の代表:

- `Review`
- `Reviews::CreateReviewService`
- `Reviews::UpdateReviewService`
- `Reviews::DeleteReviewService`

### 評価値 (Rating)

レビューに付ける 1 から 5 の整数評価です。
入力境界では文字列や数値を受け取る可能性がありますが、ドメイン上は有効範囲を満たす値として扱います。

コード上の代表:

- `Reviews::Rating`

### レビュー検索条件 (Review Search Criteria)

レビュー一覧を絞り込む条件です。
現在は評価値、キーワード、店舗 ID を持ちます。
これはドメイン計算ではなく読み取り用クエリの関心事です。

コード上の代表:

- `Reviews::SearchParameter`
- `Reviews::ReviewQuery`

### レビュアー信頼度 (Reviewer Trust)

レビュアーのレビュー履歴から算出する、レビュー重み付け用の信頼度です。
レビュー件数と評価値のばらつきをもとに newcomer / regular / veteran / expert の水準とスコアを決めます。

コード上の代表:

- `Reviews::ReviewerTrust`
- `Reviews::ReviewerTrustEvaluator`

### バーガースコア (Burger Score)

バーガーに付いたレビューを、レビュアー信頼度と新しさで重み付けして算出した評価結果です。
重み付き平均、信頼度、サンプル数を持ちます。

コード上の代表:

- `Reviews::BurgerScore`
- `Reviews::BurgerScoreCalculator`
- `Reviews::ReviewFact`
- `Reviews::ReviewerHistory`

## Shops / Catalog コンテキスト

### 店舗 (Shop)

バーガーを提供する店舗です。
レビュー作成時には、店舗内で評価対象となるバーガーを特定するために使われます。

コード上の代表:

- `Shop`
- `ShopsController`

### バーガー (Burger)

レビューの評価対象となる商品です。
同じ名前のバーガーでも、店舗との提供関係を通じて扱います。

コード上の代表:

- `Burger`

### 提供バーガー (Offering / Shop Burger)

店舗とバーガーの提供関係です。
現在のモデル名は `ShopsAndBurger` ですが、会話上は「提供バーガー」または「店舗のバーガー」と呼びます。

コード上の代表:

- `ShopsAndBurger`

## BurgerStats コンテキスト

### バーガー統計 (Burger Stat)

バーガーごとの集計済み読み取りモデルです。
レビュー作成・更新・削除後に非同期ジョブで再計算します。

コード上の代表:

- `BurgerStat`
- `BurgerStatUpdateJob`
- `BurgerStats::RecalculateBurgerStatService`

### 信頼度 (Confidence)

バーガースコアの確からしさを表す 0.0 から 1.0 の値です。
レビュー数と重みの十分さをもとに算出します。

コード上の代表:

- `Reviews::BurgerScore#confidence`
- `BurgerStat#confidence`

## 実装用語として扱うもの

以下は業務用語ではなく実装上の言葉です。domain 配下には置かず、query / repository / service 側に閉じ込めます。

- Finder
- Query
- Repository
- Serializer
- Parameter
- ActiveRecord scope
- includes / joins / where / pluck
- transaction / lock
