# AGENT.md

このリポジトリで作業する AI エージェント向けのプロジェクトガイドです。詳細版は `AGENTS.md` も参照してください。

## プロジェクト概要

Hamburger Evaluation は、ハンバーガーのレビュー・評価を投稿する Web アプリです。

- ユーザー登録 / ログイン / ログアウト
- ショップ一覧・詳細
- バーガーへのレビュー投稿・編集・削除
- レビューをもとにしたバーガー統計更新
- ユーザープロフィール更新・退会

構成は Rails API の `backend/` と React SPA の `frontend/` に分かれています。

```text
hamburger_evaluation/
├── backend/    # Ruby on Rails 8 API
├── frontend/   # React 19 + TypeScript + Vite
├── memory/     # プロジェクトメモ
├── plan/       # 計画ドキュメント
└── plans/      # エージェント作業計画
```

## 基本方針

- ユーザーとのやり取りは日本語を基本にする。
- 変更前に既存実装・テスト・ドキュメントを確認する。
- backend の検証はホスト Ruby ではなく Docker Compose 経由で行う。
- 認証情報・秘密鍵・トークン値は記録しない。
- 未追跡ファイルは勝手に commit しない。特に `SETUP.md` と `plans/*.md` は明示がない限り対象外にする。

## Backend

### 技術スタック

- Ruby 3.3.10
- Rails 8 API mode
- PostgreSQL 16
- JWT 認証
- Pundit
- dry-struct / dry-types
- RSpec / FactoryBot / SimpleCov
- RuboCop
- Brakeman

### アーキテクチャ

Rails らしさを残した軽量 DDD / 依存性逆転を採用しています。

```text
backend/app/
├── controllers/    # HTTP 境界。認可・パラメータ生成・Service 呼び出しに寄せる
├── domain/         # Value Object / domain logic。ActiveRecord に直接依存しない
├── parameters/     # dry-struct による入力 DTO
├── queries/        # 読み取り用 query。検索・includes・where を集約
├── repositories/   # CUD / 永続化境界。ActiveRecord 操作を集約
├── services/       # ユースケース単位の application service
├── jobs/           # 非同期処理。model lookup は repository 経由に寄せる
├── policies/       # Pundit policy
├── serializers/    # JSON serializer
└── models/         # ActiveRecord model。ビジネスロジックは薄く保つ
```

### 実装ルール

- domain 層に `ActiveRecord`, `Review`, `Shop`, `Burger`, `User`, `BurgerStat` などの model 直依存を持ち込まない。
- controller / job から `.find`, `.where`, `.includes`, `.find_by`, `.save`, `.update!`, `.discard` などの ActiveRecord 操作を直接呼ばない。必要なら `queries/` または `repositories/` に寄せる。
- service はユースケースを表現し、永続化の詳細は repository に委譲する。
- repository / query は Rails の concrete class を default 引数で注入してよい。
  - 例: `repository: Reviews::ReviewRepository.new`
- 厳密な DI container や port interface は、必要性が明確になるまでは導入しない。

### Backend コマンド

すべて `backend/` で実行します。

```bash
# 起動
cd backend
docker compose up --build

# テスト: SimpleCov 80% 以上が必須
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# RuboCop
cd backend
docker compose run --rm api bin/rubocop -f github

# Brakeman
cd backend
docker compose run --rm api bin/brakeman --no-pager
```

## Frontend

### 技術スタック

- React 19
- TypeScript
- Vite
- React Router
- SWR
- Jotai
- react-hook-form + Zod
- axios
- ESLint
- Vitest
- pnpm

### アーキテクチャ

```text
frontend/src/
├── app/          # Router / provider / app shell
├── domains/      # auth, reviews, shops, users などの機能単位
├── api/          # API client / HTTP 境界
├── states/       # global state
└── components/   # shared UI
```

### Frontend コマンド

すべて `frontend/` で実行します。

```bash
# 起動
cd frontend
docker compose up --build

# lint
cd frontend
pnpm run lint

# type check
cd frontend
pnpm run type-check

# test
cd frontend
pnpm run test

# build
cd frontend
pnpm run build
```

## API / 認証

- API は Rails 側が snake_case、frontend 側が camelCase。
- HTTP 境界で camelCase / snake_case を変換する。
- 認証は JWT Bearer token。
- `Authorization: Bearer <token>` を前提にする。
- `devise_token_auth` ではなく、カスタム JWT 認証を使っている。

主な API:

```text
POST   /signup
POST   /login
POST   /logout
GET    /shops
GET    /shops/:id
GET    /reviews
GET    /reviews/:id
POST   /reviews
PUT    /reviews/:id
DELETE /reviews/:id
GET    /users
PUT    /users/:id
DELETE /users/:id
```

## 品質チェック

backend 変更時は原則として以下を通します。

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

frontend 変更時は原則として以下を通します。

```bash
cd frontend
pnpm run lint
pnpm run type-check
pnpm run test
```

## Git / PR 方針

- 作業前後に `git status --short --branch` を確認する。
- backend のみの依頼なら frontend や未追跡ドキュメントを巻き込まない。
- commit 前に `git diff --check` または `git diff --cached --check` を確認する。
- PR 更新時は push 後に `gh pr view` などで PR 状態を確認する。
