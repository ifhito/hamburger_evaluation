# 店舗・バーガー名でレビュー登録

## Context

現在 `POST /reviews` は `burger_id` を直接受け取るが、ユーザーはバーガーIDを知らない。
店舗を選択してバーガー名を自由入力でき、Rails 側が burger を find_or_create する形に変更する。

---

## 変更概要

| 層 | 変更内容 |
|----|---------|
| DB | `burgers` に `name` カラムを追加 |
| Rails | `GET /shops` エンドポイント追加、`reviews#create` のパラメータを `shop_id + burger_name` に変更 |
| Frontend | `ReviewCreateInput` 型変更、`useReviewForm` 更新、review-new ページの UI 変更 |

---

## Backend

### 1. Migration: `burgers` に `name` を追加

**ファイル:** `backend/db/migrate/20260319000000_add_name_to_burgers.rb`

```ruby
def up
  add_column :burgers, :name, :string
  Burger.reset_column_information
  Burger.find_each { |b| b.update_column(:name, "Burger ##{b.id}") }
  change_column_null :burgers, :name, false
end

def down
  remove_column :burgers, :name
end
```

### 2. ShopsController の追加

**ファイル:** `backend/app/controllers/shops_controller.rb`（新規）

```ruby
class ShopsController < ApplicationController
  def index
    shops = Shop.all.order(:name)
    render json: shops.map { |s| { id: s.id, name: s.name } }
  end
end
```

`ApplicationController` は Authenticatable を include していないので認証不要。

**routes.rb に追加:**
```ruby
resources :shops, only: [:index]
```

### 3. ReviewsController の変更

**ファイル:** `backend/app/controllers/reviews_controller.rb`

`create` アクションを以下に変更:
```ruby
def create
  shop = Shop.find(params[:review][:shop_id])
  burger = find_or_create_burger(shop, params[:review][:burger_name])
  review = current_user.reviews.build(rating: review_params[:rating], comment: review_params[:comment], burger: burger)
  if review.save
    review.burger.reload
    render json: ReviewSerializer.new(review).as_json, status: :created
  else
    render json: { errors: review.errors.full_messages }, status: :unprocessable_entity
  end
rescue ActiveRecord::RecordNotFound
  render json: { error: "Shop not found" }, status: :not_found
end
```

private に追加:
```ruby
def find_or_create_burger(shop, burger_name)
  burger = shop.burgers.find_by(name: burger_name)
  unless burger
    burger = Burger.create!(name: burger_name)
    ShopsAndBurger.create!(shop: shop, burger: burger)
  end
  burger
end

def review_params
  params.require(:review).permit(:rating, :comment)
end
```

### 4. テストの更新

**ファイル:** `backend/spec/requests/reviews_spec.rb`

- `valid_params` を `{ review: { rating: 4, comment: "...", shop_id: shop.id, burger_name: "Big Mac" } }` に変更
- `let(:burger)` を `let(:shop)` + `ShopsAndBurger` 経由に変更

**ファイル:** `backend/spec/factories/burgers.rb`

```ruby
factory :burger do
  name { Faker::Food.dish }
end
```

---

## Frontend

### 1. 型の追加・変更

**`shared/lib/types/shop.ts`（新規）:**
```ts
export interface Shop { id: number; name: string }
```

**`shared/lib/types/review.ts`:**
`ReviewCreateInput` を変更:
```ts
export interface ReviewCreateInput {
  rating: number
  comment: string
  shop_id: number
  burger_name: string
}
```

### 2. API クライアント（`shared/lib/api.ts`）

`shopsApi` を追加:
```ts
export const shopsApi = {
  list(): Promise<Shop[]> { return request('/shops') },
}
```

### 3. フック

**`shared/lib/hooks/useShops.ts`（新規）:**
```ts
export function useShops() {
  return useQuery({ queryKey: ['shops'], queryFn: () => shopsApi.list() })
}
```

**`shared/lib/hooks/useReviewForm.ts`:**

`ReviewFormState` の `burger_id: string` を `shop_id: number` + `burger_name: string` に変更。
`validate(['shop_id', 'burger_name'])` で create 時の必須チェック。

### 4. review-new ページの UI 変更

**`pages/review-new/index.tsx`:**

```
[Rating]
[Comment]
[ 店舗を選択 ▼ ]     ← useShops() のデータで select
[ バーガー名を入力 ]   ← 自由入力
[Submit]
```

---

## 変更ファイル一覧

| ファイル | 変更種別 | ステータス |
|---------|---------|-----------|
| `backend/db/migrate/20260319000000_add_name_to_burgers.rb` | 新規 | ✅ 完了 |
| `backend/app/models/burger.rb` | 変更（validates :name 追加） | ✅ 完了 |
| `backend/app/controllers/shops_controller.rb` | 新規 | ✅ 完了 |
| `backend/config/routes.rb` | 変更（shops 追加） | ✅ 完了 |
| `backend/app/controllers/reviews_controller.rb` | 変更（create ロジック） | ✅ 完了 |
| `backend/spec/factories/burgers.rb` | 変更（name 追加） | ✅ 完了 |
| `backend/spec/requests/reviews_spec.rb` | 変更（params 更新） | ✅ 完了 |
| `frontend/src/shared/lib/types/shop.ts` | 新規 | ✅ 完了 |
| `frontend/src/shared/lib/types/review.ts` | 変更（ReviewCreateInput） | ✅ 完了 |
| `frontend/src/shared/lib/api.ts` | 変更（shopsApi 追加） | ✅ 完了 |
| `frontend/src/shared/lib/hooks/useShops.ts` | 新規 | ✅ 完了 |
| `frontend/src/shared/lib/hooks/useReviewForm.ts` | 変更（フィールド変更） | ✅ 完了 |
| `frontend/src/pages/review-new/index.tsx` | 変更（UI 変更） | ✅ 完了 |

---

## 検証

```bash
# Rails マイグレーション
docker compose run --rm api rails db:migrate

# テスト（全件 59 examples, 0 failures / カバレッジ 91.18%）
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# 動作確認
# 1. GET /shops でショップ一覧が返る
# 2. POST /reviews に shop_id + burger_name を送りレビュー作成できる
# 3. 同じ shop_id + burger_name で再度送っても burger が重複しない
# 4. フロントで店舗選択 → バーガー名入力 → 送信 → review-detail に遷移する
```
