# AGENTS.md

AI エージェント・CI 向けのクイックリファレンスです。

## リポジトリ構成

```
hamburger_evaluation/
├── backend/    # Rails 8 API (Ruby 3.3.10)
└── frontend/   # React 19 + TypeScript + Vite (Node 22)
```

## バックエンド (backend/)

### 技術スタック

- Ruby 3.3.10 / Rails 8.0.4 (API mode)
- PostgreSQL 16
- JWT 認証 (bcrypt + jwt gem)
- dry-struct / dry-types (Value Object・Parameter DTO)
- Pundit (認可)
- RSpec + FactoryBot (テスト)

### アーキテクチャ (軽量 DDD)

```
app/
├── controllers/          # 薄いコントローラー — 認可・Parameter生成・Service呼び出し
├── domain/reviews/       # Value Object・FinderQuery (Reviews:: 名前空間)
├── services/             # ユースケース単位のServiceクラス
├── repositories/         # CUD操作のRepository
├── parameters/           # dry-struct による入力DTO
├── policies/             # Pundit ポリシー
├── models/               # ActiveRecord (ビジネスロジックなし)
└── serializers/          # JSONシリアライザ
```

### 主要コマンド

```bash
# Docker で起動
docker compose up          # from backend/

# テスト (カバレッジ 80% 以上が必須)
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# Lint
bin/rubocop -f github

# セキュリティスキャン
bin/brakeman --no-pager
```

## フロントエンド (frontend/)

### 技術スタック

- React 19 / TypeScript / Vite
- SWR (データフェッチ)
- Jotai (認証状態グローバル管理)
- react-hook-form + Zod (フォームバリデーション)
- axios + camelcase-keys/snakecase-keys (HTTP境界の命名変換)
- ESLint + TypeScript strict (静的解析)
- Vitest (ユニットテスト)
- pnpm (パッケージマネージャー)

### アーキテクチャ (domains ベース)

```
src/
├── app/
│   ├── router/           # React Router — ProtectedRoute / GuestRoute
│   └── App.tsx           # JotaiProvider > AuthProvider > RouterProvider
├── domains/
│   ├── auth/             # AuthProvider・Jotai atom・pages
│   ├── reviews/          # api / hooks / pages
│   ├── shops/            # api / hooks / pages
│   └── users/            # api / hooks / pages
├── api/client/           # buildApiClient (axios interceptors)
├── states/               # authAtom (Jotai)
└── components/           # 共有 UI コンポーネント + Storybook stories
```

### 主要コマンド

```bash
# Docker で起動
docker compose up          # from frontend/

# 品質チェック (CI と同じ)
pnpm run lint              # ESLint
pnpm run type-check        # tsc --noEmit
pnpm run test              # Vitest

# 開発
pnpm run dev               # http://localhost:5173
pnpm run build             # プロダクションビルド
```

## CI

ルート `.github/workflows/ci.yml` に定義。PR / main push 時に実行。

| ジョブ | 対象 | コマンド |
|--------|------|---------|
| backend_scan | backend | brakeman |
| backend_lint | backend | rubocop |
| backend_test | backend | rspec |
| frontend_type_check | frontend | tsc |
| frontend_lint | frontend | eslint |
| frontend_test | frontend | vitest |

## 認証フロー

> **注意 (SETUP.md からの意図的な逸脱)**: SETUP.md では `devise_token_auth` を使い  
> `access-token / client / uid` ヘッダーでセッション管理するよう指定されているが、  
> このプロジェクトでは **カスタム JWT Bearer トークン** を採用している。  
> 理由: devise_token_auth のステートフルなセッション管理を避け、シンプルなステートレス認証を優先した。

1. POST `/signup` or `/login` → レスポンスボディで JWT トークン返却
2. フロントは localStorage に保存 (`src/domains/auth/storage.ts`)
3. Jotai atom (`authUserAtom`, `authTokenAtom`) でグローバル共有
4. axios interceptor が `Authorization: Bearer <token>` ヘッダーを自動付与

## API 命名規則

- バックエンド: snake_case
- フロントエンド: camelCase
- 変換は axios interceptor が自動で行う (リクエスト: snakecaseKeys, レスポンス: camelcaseKeys)
