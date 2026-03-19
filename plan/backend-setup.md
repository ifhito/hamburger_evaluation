# backend/ Rails 環境構築プラン

## 構成概要

| 項目 | 内容 |
|------|------|
| Ruby | 3.3 |
| Rails | 8.0.4（API モード） |
| DB | PostgreSQL 16 |
| 認証 | JWT（`jwt` gem + `bcrypt`、Devise 不使用） |
| 論理削除 | `discard` gem（users・reviews に適用） |
| CORS | `rack-cors`（localhost:3001 を許可） |

---

## 追加 Gem

| Gem | 用途 |
|-----|------|
| `pg` | PostgreSQL アダプター |
| `bcrypt` | パスワードのハッシュ化（`has_secure_password`） |
| `jwt` | JWT トークンの生成・検証 |
| `rack-cors` | フロントエンドとの CORS 設定 |
| `discard` | 論理削除（users・reviews） |

---

## Docker 構成

```
backend/
  Dockerfile          # Ruby 3.3-slim ベース（開発用）
  docker-compose.yml  # api + db（postgres:16）サービス
  entrypoint.sh       # server.pid 削除 → exec "$@"
```

### 起動コマンド

```bash
cd backend/
docker compose up
```

### 環境変数（docker-compose.yml で設定済み）

| 変数 | 値 |
|------|----|
| `POSTGRES_HOST` | `db` |
| `POSTGRES_USER` | `postgres` |
| `POSTGRES_PASSWORD` | `password` |
| `RAILS_ENV` | `development` |

---

## DB 設計（5テーブル）

```
users: id, email(UNIQUE), password_digest, username, discarded_at, timestamps
shops: id, name, timestamps
burgers: id, timestamps
shops_and_burgers: id, shop_id(FK), burger_id(FK), timestamps
reviews: id, rating, comment, user_id(FK), burger_id(FK), discarded_at, timestamps
```

### ER図

→ `backend/docs/DB/er-diagram.md` 参照

### マイグレーション実行手順

```bash
# DB 作成
docker compose run --rm api rails db:create

# マイグレーション実行
docker compose run --rm api rails db:migrate
```

---

## モデル設計

### User
```ruby
include Discard::Model
has_secure_password
has_many :reviews, dependent: :destroy
```

### Burger
```ruby
has_many :reviews, dependent: :destroy
has_many :shops_and_burgers, dependent: :destroy
has_many :shops, through: :shops_and_burgers
```

### Shop
```ruby
has_many :shops_and_burgers, dependent: :destroy
has_many :burgers, through: :shops_and_burgers
```

### ShopsAndBurger
```ruby
belongs_to :shop
belongs_to :burger
```

### Review
```ruby
include Discard::Model
belongs_to :user
belongs_to :burger
```

---

## API エンドポイント

### 認証（auth_controller.rb）

| メソッド | パス | 認証不要 | 内容 |
|----------|------|----------|------|
| POST | `/signup` | ✅ | アカウント作成 → JWT 返却 |
| POST | `/login` | ✅ | ログイン → JWT 返却 |
| POST | `/logout` | ✅ | ログアウト |

### レビュー（reviews_controller.rb）

| メソッド | パス | 認証不要 | 内容 |
|----------|------|----------|------|
| GET | `/reviews` | ✅ | 一覧取得（論理削除済み除外） |
| GET | `/reviews/:id` | ✅ | 1件取得 |
| POST | `/reviews` | — | 作成 |
| PUT | `/reviews/:id` | — | 更新（自分のレビューのみ） |
| DELETE | `/reviews/:id` | — | 論理削除（自分のレビューのみ） |

### ユーザー（users_controller.rb）

| メソッド | パス | 認証不要 | 内容 |
|----------|------|----------|------|
| GET | `/users` | ✅ | 一覧取得（論理削除済み除外） |
| PUT | `/users/:id` | — | 自分のプロフィール更新 |
| DELETE | `/users/:id` | — | 退会（論理削除） |

---

## JWT 認証の仕組み

```
Concern: app/controllers/concerns/authenticatable.rb
  ↓
before_action :authenticate_user!
  ↓
Authorization: Bearer <token> ヘッダーを検証
  ↓
User.kept.find(payload["user_id"]) で current_user をセット
```

- トークン有効期限: 24時間
- 論理削除済みユーザーはトークンがあっても認証失敗

---

## 動作確認

```bash
# ヘルスチェック
curl http://localhost:3000/up

# サインアップ
curl -X POST http://localhost:3000/signup \
  -H "Content-Type: application/json" \
  -d '{"username":"test","email":"test@example.com","password":"password123","password_confirmation":"password123"}'

# ログイン → token 取得
curl -X POST http://localhost:3000/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# レビュー一覧（認証不要）
curl http://localhost:3000/reviews

# レビュー作成（token 必要）
curl -X POST http://localhost:3000/reviews \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"rating":5,"comment":"最高でした","burger_id":1}'
```
