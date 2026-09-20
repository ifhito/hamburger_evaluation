# 重み付きバーガースコアリングシステム — DDD設計

---

## 1. ユビキタス言語（Ubiquitous Language）

| 用語 | 定義 |
|------|------|
| **Review（レビュー）** | あるユーザーが特定バーガーに対して行った評価行為 |
| **Rating（評価値）** | 1〜5の整数で表すバーガーへの点数評価 |
| **ReviewerTrust（レビュアー信頼度）** | そのユーザーの評価がどれだけ信頼できるかを表す度合い |
| **ReviewWeight（レビュー重み）** | 1件のレビューがスコア計算に与える影響度 (信頼度 × 時間減衰) |
| **BurgerScore（バーガースコア）** | 全レビューの重み付き平均と信頼性を合成したバーガーの総合評価値 |
| **Confidence（信頼性）** | BurgerScoreがどれくらい信頼できるかを示す指標(0.0〜1.0) |
| **TimeDecay（時間減衰）** | 古いレビューほど重みを小さくする係数 |

---

## 2. 境界づけられたコンテキスト（Bounded Contexts）

```
┌─────────────────────────┐   ┌──────────────────────────────┐
│     Review Context      │   │       Burger Context          │
│                         │   │                               │
│  ・Review の投稿・管理   │──▶│  ・Burger のスコア算出        │
│  ・Rating の検証         │   │  ・BurgerStat の更新          │
│                         │   │  ・スコア信頼性の評価          │
└─────────────────────────┘   └──────────────────────────────┘
         │                                    ▲
         │ review.user                        │ BurgerScoreCalculator
         ▼                                    │
┌─────────────────────────┐                  │
│      User Context       │──────────────────┘
│                         │  ReviewerTrustEvaluator
│  ・User 管理             │
│  ・ReviewerTrust 計算    │
└─────────────────────────┘
         │
         ▼
┌─────────────────────────┐
│      Shop Context       │
│                         │
│  ・Shop 管理             │
│  ・Burger との紐付け     │
└─────────────────────────┘
```

---

## 3. 集約設計（Aggregate Design）

```
Review Aggregate (Root: Review)
─────────────────────────────────────────────────────────────
  Review [Entity]
    - id
    - comment
    - created_at
    - user_id (→ User Aggregate)
    - burger_id (→ Burger Aggregate)
    └── Rating [Value Object]   ← Reviewに埋め込まれた評価値
          - value: 1..5
          - excellent?(), poor?()


Burger Aggregate (Root: Burger)
─────────────────────────────────────────────────────────────
  Burger [Entity]
    - id, name
    └── BurgerStat [Entity / Read Model]
          - review_count
          - average_rating
          - weighted_score   ← NEW
          - confidence       ← NEW
  ※ BurgerScore [Value Object] はメモリ上でのみ存在し、
     BurgerStat に永続化される（Read Model パターン）


User Aggregate (Root: User)
─────────────────────────────────────────────────────────────
  User [Entity]
    - id, email, username
    ※ ReviewerTrust [Value Object] はメモリ上でのみ存在し、
       永続化しない（都度計算）


Shop Aggregate (Root: Shop)
─────────────────────────────────────────────────────────────
  Shop [Entity]
    - id, name
  ShopsAndBurger [Entity] (Join)
    - shop_id, burger_id
```

---

## 4. ドメインイベントとフロー（Domain Events）

```
[User]  POST /reviews
    │
    ▼
Review.save()
    │
    ├── after_create  ──▶ ReviewCreated (イベント相当)
    │                         │
    │                         ▼
    │                  BurgerStatUpdateJob.perform_later(burger_id)
    │                         │
    │                         ▼
    │                  BurgerStat.recalculate_for(burger)
    │                         │
    │                  ┌──────┴──────────────────────────────┐
    │                  │  BurgerScoreCalculator.call(burger)  │ ← Domain Service
    │                  │    │                                 │
    │                  │    ├── burger.reviews.each do |r|    │
    │                  │    │     ReviewerTrustEvaluator      │ ← Domain Service
    │                  │    │       .call(r.user)             │
    │                  │    │     → ReviewerTrust             │ ← Value Object
    │                  │    │                                 │
    │                  │    ├── weight = trust × recency      │
    │                  │    └── → BurgerScore                 │ ← Value Object
    │                  └──────────────────────────────────────┘
    │                         │
    │                         ▼
    │                  BurgerStat.update!(
    │                    weighted_score: score.weighted_average,
    │                    confidence:     score.confidence
    │                  )
    │
    └── after_discard ──▶ (同じフロー)
```

---

## 5. Value Object / Domain Service の責務マップ

```
┌────────────────────────────────────────────────────────────────────┐
│ Value Objects（不変・同値性・ドメイン語彙）                          │
├──────────────────┬─────────────────────────────────────────────────┤
│ Rating           │ 1〜5の整数をラップ。excellent?/poor? などのドメイン│
│                  │ メソッドを持つ。不変(freeze)。                    │
├──────────────────┼─────────────────────────────────────────────────┤
│ ReviewerTrust    │ score(Float) + level(Symbol)。                   │
│                  │ ReviewerTrustEvaluatorが生成する。不変。           │
├──────────────────┼─────────────────────────────────────────────────┤
│ BurgerScore      │ weighted_average + confidence + sample_size。    │
│                  │ BurgerScoreCalculatorが生成する。不変。           │
│                  │ reliable?() で信頼性チェック可能。                │
└──────────────────┴─────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│ Domain Services（ステートレス・計算・外部依存なし）                   │
├──────────────────────────┬─────────────────────────────────────────┤
│ ReviewerTrustEvaluator   │ User → ReviewerTrust                    │
│                          │ レビュー数・評価分散からTrustを計算        │
├──────────────────────────┼─────────────────────────────────────────┤
│ BurgerScoreCalculator    │ Burger → BurgerScore                    │
│                          │ 全レビューを重み付き集計。                 │
│                          │ trust_evaluatorをDI可能。                │
└──────────────────────────┴─────────────────────────────────────────┘
```

---

## 6. ディレクトリ構造

```
backend/app/
  domain/
    values/
      rating.rb
      reviewer_trust.rb
      burger_score.rb
    services/
      reviewer_trust_evaluator.rb
      burger_score_calculator.rb

  models/
    burger_stat.rb      ← recalculate_for を更新

  serializers/
    review_serializer.rb  ← weighted_score / confidence を追加

db/migrate/
  YYYYMMDD_add_score_to_burger_stats.rb

spec/
  domain/
    values/
      rating_spec.rb
      reviewer_trust_spec.rb
      burger_score_spec.rb
    services/
      reviewer_trust_evaluator_spec.rb
      burger_score_calculator_spec.rb
  models/
    burger_stat_spec.rb  ← 更新
```

---

## 7. 実装詳細

### `app/domain/values/rating.rb`
```ruby
class Rating
  VALID_RANGE = 1..5

  attr_reader :value

  def initialize(value)
    raise ArgumentError, "Invalid rating: #{value}" unless VALID_RANGE.cover?(value.to_i)
    @value = value.to_i
    freeze
  end

  def excellent? = value >= 4
  def poor?      = value <= 2
  def ==(other)  = other.is_a?(Rating) && value == other.value
  def to_f       = value.to_f
  def to_s       = value.to_s
end
```

### `app/domain/values/reviewer_trust.rb`
```ruby
class ReviewerTrust
  LEVELS = {
    newcomer: { min_reviews: 0,  base_score: 0.5 },
    regular:  { min_reviews: 3,  base_score: 0.7 },
    veteran:  { min_reviews: 10, base_score: 0.9 },
    expert:   { min_reviews: 20, base_score: 1.0 }
  }.freeze

  attr_reader :score, :level

  def initialize(score:, level:)
    @score = score.clamp(0.0, 1.0)
    @level = level
    freeze
  end

  def to_f = score
  def to_s = "#{level}(#{(score * 100).round(1)}%)"
end
```

### `app/domain/values/burger_score.rb`
```ruby
class BurgerScore
  attr_reader :weighted_average, :confidence, :sample_size

  def initialize(weighted_average:, confidence:, sample_size:)
    @weighted_average = weighted_average.round(2)
    @confidence       = confidence.clamp(0.0, 1.0).round(4)
    @sample_size      = sample_size
    freeze
  end

  def reliable? = confidence >= 0.6

  def self.empty
    new(weighted_average: 0.0, confidence: 0.0, sample_size: 0)
  end
end
```

### `app/domain/services/reviewer_trust_evaluator.rb`
```ruby
class ReviewerTrustEvaluator
  LOW_VARIANCE_THRESHOLD = 0.3
  LOW_VARIANCE_PENALTY   = 0.7

  def call(user)
    reviews = user.reviews.kept
    base    = base_score(reviews.count)
    factor  = variance_factor(reviews)
    level   = determine_level(reviews.count)

    ReviewerTrust.new(score: base * factor, level: level)
  end

  private

  def base_score(count)
    ReviewerTrust::LEVELS
      .select { |_, v| count >= v[:min_reviews] }
      .values.last[:base_score]
  end

  def variance_factor(reviews)
    return 1.0 if reviews.count < 3
    ratings = reviews.pluck(:rating).map(&:to_f)
    mean    = ratings.sum / ratings.size
    var     = ratings.sum { |r| (r - mean)**2 } / ratings.size
    var < LOW_VARIANCE_THRESHOLD ? LOW_VARIANCE_PENALTY : 1.0
  end

  def determine_level(count)
    ReviewerTrust::LEVELS
      .select { |_, v| count >= v[:min_reviews] }
      .keys.last
  end
end
```

### `app/domain/services/burger_score_calculator.rb`
```ruby
class BurgerScoreCalculator
  RECENCY_HALF_LIFE_DAYS = 180.0

  def initialize(trust_evaluator: ReviewerTrustEvaluator.new)
    @trust_evaluator = trust_evaluator
  end

  def call(burger)
    reviews = burger.reviews.kept.includes(:user)
    return BurgerScore.empty if reviews.empty?

    weighted = reviews.map { |r| [r.rating.to_f, weight_for(r)] }
    total_weight = weighted.sum(&:last)
    weighted_avg = weighted.sum { |rating, w| rating * w } / total_weight
    confidence   = calculate_confidence(reviews.count, total_weight)

    BurgerScore.new(
      weighted_average: weighted_avg,
      confidence:       confidence,
      sample_size:      reviews.count
    )
  end

  private

  def weight_for(review)
    @trust_evaluator.call(review.user).to_f * recency_factor(review.created_at)
  end

  def recency_factor(created_at)
    days_ago = (Time.current - created_at) / 1.day
    Math.exp(-days_ago * Math.log(2) / RECENCY_HALF_LIFE_DAYS)
  end

  def calculate_confidence(count, total_weight)
    review_factor = [count / 10.0, 1.0].min
    weight_factor = [total_weight / count.to_f, 1.0].min
    (review_factor * 0.6 + weight_factor * 0.4).clamp(0.0, 1.0)
  end
end
```

### `app/models/burger_stat.rb` 更新箇所
```ruby
def self.recalculate_for(burger)
  score  = BurgerScoreCalculator.new.call(burger)
  active = burger.reviews.kept

  find_or_initialize_by(burger: burger).update!(
    review_count:    active.count,
    average_rating:  active.average(:rating).to_f.round(2),
    weighted_score:  score.weighted_average,
    confidence:      score.confidence
  )
end
```

---

## 8. テスト方針（TDD）

**Value Object spec（DBなし・純粋ユニットテスト）**
```ruby
# rating_spec.rb
it "raises ArgumentError on invalid value"
it "is frozen after initialization"
it "#excellent? returns true for 4 and 5"
it "#poor? returns true for 1 and 2"
it "equality by value"
```

**Domain Service spec（FactoryBot使用）**
```ruby
# reviewer_trust_evaluator_spec.rb
it "returns :newcomer trust for user with 0 reviews"
it "returns :expert trust for user with 20+ reviews"
it "applies LOW_VARIANCE_PENALTY when user always gives same rating"

# burger_score_calculator_spec.rb
it "returns BurgerScore.empty for burger with no reviews"
it "weights expert reviewers more than newcomers"
it "weights recent reviews more than old reviews"
it "accepts injected trust_evaluator (stub)"
```

---

## 9. 変更ファイル一覧

| ファイル | 操作 |
|--------|------|
| `app/domain/values/rating.rb` | 新規 |
| `app/domain/values/reviewer_trust.rb` | 新規 |
| `app/domain/values/burger_score.rb` | 新規 |
| `app/domain/services/reviewer_trust_evaluator.rb` | 新規 |
| `app/domain/services/burger_score_calculator.rb` | 新規 |
| `db/migrate/YYYYMMDD_add_score_to_burger_stats.rb` | 新規 |
| `app/models/burger_stat.rb` | 更新 |
| `app/serializers/review_serializer.rb` | 更新 |
| `spec/domain/values/rating_spec.rb` | 新規 |
| `spec/domain/values/reviewer_trust_spec.rb` | 新規 |
| `spec/domain/values/burger_score_spec.rb` | 新規 |
| `spec/domain/services/reviewer_trust_evaluator_spec.rb` | 新規 |
| `spec/domain/services/burger_score_calculator_spec.rb` | 新規 |
| `spec/models/burger_stat_spec.rb` | 更新 |

---

## 10. 検証

```bash
# ドメイン層のみ
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec spec/domain/

# 全テスト
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# マイグレーション
docker compose run --rm api rails db:migrate
```
