# AGENT.md

このリポジトリで作業する AI エージェント向けのプロジェクトガイドです。詳細版は `AGENTS.md` も参照してください。API のエンドポイント一覧とデータベーススキーマは `CLAUDE.md` を参照してください。

## プロジェクト概要

BurgerStack は、ハンバーガーのレビュー・評価を投稿する Web アプリです。

- ユーザー登録 / ログイン / ログアウト
- ショップ一覧・詳細
- バーガーへのレビュー投稿・編集・削除
- レビューをもとにしたバーガー統計更新
- ユーザープロフィール更新・退会

構成は Go API の `backend-go/` と React SPA の `frontend/` に分かれています。

```text
hamburger_evaluation/
├── backend-go/ # Go API (net/http + sqlc + PostgreSQL 16)
├── frontend/   # React 19 + TypeScript + Vite
├── memory/     # プロジェクトメモ
├── plan/       # 計画ドキュメント
└── plans/      # エージェント作業計画
```

## 基本方針

- ユーザーとのやり取りは日本語を基本にする。
- 変更前に既存実装・テスト・ドキュメントを確認する。
- backend の検証は `go-checks.sh` で行い、DB・マイグレーション・sqlc は Docker Compose 経由で行う。
- 認証情報・秘密鍵・トークン値は記録しない。
- 未追跡ファイルは勝手に commit しない。特に `SETUP.md` と `plans/*.md` は明示がない限り対象外にする。

## Backend

### 技術スタック

- Go 1.27(標準 `net/http` のルーティングを使う。Web フレームワークも ORM も使わない)
- PostgreSQL 16(pgx)
- sqlc(SQL からの型安全なコード生成)
- JWT 認証

### アーキテクチャ

クリーンアーキテクチャ(handler → usecase → domain)を採用しています。依存は内側にのみ向きます。

```text
backend-go/
├── cmd/api/main.go     # composition root: 設定、DB プール、配線、サーバ
├── internal/
│   ├── domain/         # エンティティ / 値オブジェクト / ドメインエラー / 書き込みの *Repository の interface と、それを持つ書き込みオブジェクト(複数の集約を跨ぐ更新だけ *Service)。標準ライブラリのみ
│   ├── usecase/        # ユースケース + 読み取りの *Query(利用側で宣言)。repository には依存しない
│   └── adapter/
│       ├── handler/    # net/http のハンドラ、DTO、ルーティング、middleware
│       ├── query/      # usecase の *Query(読み取り)を sqlc で実装
│       ├── repository/ # domain の *Repository(書き込み)を sqlc で実装
│       │   └── sqlcgen/  # sqlc の生成コード。手で編集しない
│       ├── rowmap/     # sqlc の行 → domain の写像(query と repository で共有)
│       └── infra/      # DB プール、JWT、パスワードハッシュ、設定
├── db/
│   ├── migrations/     # SQL マイグレーション
│   └── queries/        # sqlc のクエリ(*.sql)
└── sqlc.yaml
```

### 実装ルール

- `domain` に `net/http`・`database/sql`・`pgx`・`usecase`・`adapter` の依存を持ち込まない。
- 読み取りの `*Query` は `usecase` 側で宣言し、`adapter/query` が実装する。書き込みの `*Repository` は `domain` が宣言し、`adapter/repository` が実装する。repository を呼べるのは `domain` のコードだけで、`usecase` は repository に依存しない(読み取りは `*Query`、書き込みは domain の集約ごとの書き込みオブジェクトを通す。`*Service` は複数の集約を跨ぐ更新だけに使う)。
- 認可の判断は handler ではなく usecase / domain に置く。
- ドメインのルール(検証・権限・導出)の判断は backend の `domain` だけが持つ。frontend は入力・説明・表示・サーバーのエラーの表示だけを行い、ルールを複製しない。
- sqlc の行構造体や `pgx` の型を `adapter/` の外に出さない。ドメインの形と DB の形は別々に設計する。
- `sqlcgen/` は手で編集しない。`db/queries/` を変更して再生成する。
- 詳細は `.agents/skills/backend-go-boundaries` と `.agents/skills/db-design` を参照する。

### Backend コマンド

```bash
# 起動(:8080 で待ち受け。ヘルスチェックは GET /up。JWT_SECRET を export しておく)
cd backend-go
docker compose up --build

# 検証(gofmt / go vet / go build / go test)。リポジトリのルートから実行
.agents/skills/backend-go-change-validation/scripts/go-checks.sh

# マイグレーション
cd backend-go
docker compose run --rm migrate up

# sqlc の再生成(internal/adapter/repository/sqlcgen に差分が出てはならない)
cd backend-go
docker compose run --rm sqlc generate
```

## Frontend

### 技術スタック

- React 19
- TypeScript
- Vite
- React Router
- SWR
- Jotai
- react-hook-form(入力の検証は backend。frontend は 422 のメッセージを表示する)
- axios
- ESLint
- Vitest
- pnpm

### アーキテクチャ

```text
frontend/src/
├── app/          # Router / provider / アプリシェル
├── domains/      # auth, reviews, shops, users などの機能単位
├── api/          # API client / HTTP 境界
├── states/       # グローバル state
└── components/   # 共通 UI
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

# 型チェック
cd frontend
pnpm run type-check

# テスト
cd frontend
pnpm run test

# ビルド
cd frontend
pnpm run build
```

## API / 認証

- API は Go 側が snake_case、frontend 側が camelCase。
- HTTP 境界(`frontend/src/api/client/buildApiClient.ts`)で camelCase / snake_case を変換する。
- 認証は JWT Bearer token。
- `Authorization: Bearer <token>` を前提にする。
- 独自実装の JWT 認証を使っている。

## 品質チェック

backend 変更時は原則として以下を通します。

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
cd backend-go && docker compose run --rm sqlc generate   # db/queries/ を変更したとき
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
