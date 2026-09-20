# CLAUDE.md

このファイルは、このリポジトリで作業する Claude Code などのコーディングエージェント向けのガイダンスを提供します。

## プロジェクト概要

Hamburger Evaluation は、ハンバーガーのレビューと評価を行う Web アプリケーションです。

ユーザーは次のことができます:

- サインアップ、ログイン、ログアウト;
- ショップ一覧とショップ詳細の閲覧;
- バーガーレビューの投稿・編集・削除;
- プロフィールの更新・削除;
- レビューから算出されるバーガー統計の閲覧。

リポジトリは Rails API の backend と React SPA の frontend に分かれています。

```text
hamburger_evaluation/
├── backend/    # Ruby on Rails 8 API
├── frontend/   # React 19 + TypeScript + Vite
├── memory/     # project notes
├── plan/       # planning documents
└── plans/      # agent-generated plans
```

## コミュニケーション

ユーザーの主な使用言語は日本語です。ユーザーから別途指定がない限り、要約・状況報告・確認のための質問は日本語で行うことを優先してください。

## 重要な作業ルール

- 振る舞いを変更する前に、既存のコード・テスト・ドキュメントを確認する。
- シークレット、トークン、認証情報、Rails / JWT のシークレット値を記録しない。
- Backend の検証は、ホストの Ruby ではなく Docker Compose 経由で実行しなければならない。
- 明示的に依頼されない限り、無関係なファイルや未追跡ファイルを commit しない。
- このリポジトリには未追跡の `SETUP.md` や `plans/*.md` が存在することがあり、無関係な commit にはデフォルトで含めない。
- commit する前に、status と diff を注意深く確認する。

## Backend

### 技術スタック

- Ruby 3.3.10
- Rails 8 API mode
- PostgreSQL 16
- JWT 認証
- Pundit による認可
- パラメータ DTO と値オブジェクトのための dry-struct / dry-types
- RSpec / FactoryBot / SimpleCov
- RuboCop
- Brakeman

### アーキテクチャ

Backend は、Rails の慣習に近い形を保ちつつ、軽量な DDD と依存性逆転を採用しています。

```text
backend/app/
├── controllers/    # HTTP boundary: auth, policy checks, params, service calls
├── domain/         # domain logic / value objects; no direct ActiveRecord dependency
├── parameters/     # dry-struct input DTOs
├── queries/        # read/query boundary; where/includes/find for read paths
├── repositories/   # persistence boundary; CUD and ActiveRecord details
├── services/       # application use cases
├── jobs/           # async work; model lookup should go through repositories
├── policies/       # Pundit policies
├── serializers/    # JSON serializers
└── models/         # ActiveRecord models kept as thin as practical
```

### Backend 設計ルール

- `backend/app/domain` に ActiveRecord への直接依存を持ち込まない。
- controller と job から model の永続化呼び出しを直接行うことを避ける。
  - そこで `.find`, `.where`, `.includes`, `.find_by`, `.save`, `.update!`, `.discard`, `.upsert` を直接使うことを避ける。
  - 読み取りは `queries/` へ、永続化操作は `repositories/` へ移す。
- Service はユースケースを表現し、永続化の詳細は repository に委譲する。
- 軽量な Rails 流の依存性注入は許容される。
  - 例: `repository: Reviews::ReviewRepository.new`
- 必要性が明確でない限り、完全な DI コンテナや厳格な port / interface 層を導入しない。

### Backend コマンド

以下は `backend/` から実行します。

```bash
# Start backend
cd backend
docker compose up --build

# Full test suite. RAILS_ENV=test is required because compose defaults may differ.
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# RuboCop
cd backend
docker compose run --rm api bin/rubocop -f github

# Brakeman
cd backend
docker compose run --rm api bin/brakeman --no-pager
```

SimpleCov は最低カバレッジを強制します。一部の spec のみを実行した場合、カバレッジが全体のしきい値を下回っているという理由だけで失敗することがあります。テストの失敗として扱う前に、フルスイートで確認してください。

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

### Frontend の構成

```text
frontend/src/
├── app/          # router, providers, app shell
├── domains/      # auth, reviews, shops, users
├── api/          # API client / HTTP boundary
├── states/       # global state
└── components/   # shared UI components
```

### Frontend コマンド

以下は `frontend/` から実行します。

```bash
# Start frontend
cd frontend
docker compose up --build

# Lint
cd frontend
pnpm run lint

# Type check
cd frontend
pnpm run type-check

# Test
cd frontend
pnpm run test

# Build
cd frontend
pnpm run build
```

## API と認証

- Backend の API は snake_case を使う。
- Frontend のコードは camelCase を使う。
- casing の変換は HTTP 境界の責務とする。
- 認証は `devise_token_auth` ではなく、独自実装の JWT Bearer token を使う。
- 認証が必要なリクエストは `Authorization: Bearer <token>` を送信すること。

主なエンドポイント:

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

## 品質ゲート

Backend の変更では、通常は次を実行する:

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Frontend の変更では、通常は次を実行する:

```bash
cd frontend
pnpm run lint
pnpm run type-check
pnpm run test
```

## Git と PR のワークフロー

- 変更の前後に `git status --short --branch` を確認する。
- commit は依頼された範囲に絞る。
- 無関係な未追跡ファイルを含めない。
- commit の前に `git diff --check` または `git diff --cached --check` を実行する。
- PR ブランチを push した後、`gh` が使える場合は `gh pr view` で PR を確認する。
