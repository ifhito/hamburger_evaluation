# CLAUDE.md

このファイルは、本リポジトリでコードを扱う際に Claude Code (claude.ai/code) に向けたガイダンスを提供する。

## プロジェクト概要

ハンバーガーのレビュー・評価 Web アプリ。ユーザーは登録し、特定の店舗のバーガーに対するレビューを投稿し、自分のプロフィールを管理できる。

## アーキテクチャ

独立した 2 つのサブプロジェクトからなり、それぞれ Docker で実行する。

| ディレクトリ | スタック | 役割 |
|-----------|-------|------|
| `backend-go/` | Go (1.22+), net/http + sqlc + pgx | REST API サーバー |
| `frontend/` | React (TypeScript) + Vite | SPA クライアント |

バックエンドは clean architecture (handler → usecase → domain) に従う。依存関係は内側に向かい、認可の判断は handler ではなく usecase/domain に置く。

## バックエンド (`backend-go/`)

- **:8080** で提供する。ヘルスチェックは `GET /up`
- 専用の Postgres で動作する (ホストポートは 5433)
- 認証: **JWT** — トークンはログイン時に返され、`Authorization: Bearer <token>` として送信しなければならない。`JWT_SECRET` が未設定の場合、API は起動時に fail-loud する。`docker compose up` の前に export すること。
- API の JSON はワイヤー上で **snake_case** を使う。フロントエンドは snake_case のワイヤー型をエンドツーエンドでそのまま使う (ケース変換レイヤーはない)。

```bash
# 起動 (:8080 で提供。ヘルスチェックは GET /up)
cd backend-go
docker compose up --build
```

```bash
# 検証 (gofmt / go vet / go build / go test)。リポジトリのルートから実行する
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

```bash
# マイグレーション — デフォルトでは dev DB が対象。MIGRATE_DATABASE_URL で上書きできる
cd backend-go
docker compose run --rm migrate up
docker compose run --rm migrate down -all
```

```bash
# 冪等な開発用 fixture (admin + alice/bob/charlie、shops、burgers、
# reviews、burger_stats) を seed する — `migrate up` の後に実行。DATABASE_URL で上書きできる
cd backend-go
docker compose run --rm seed
```

```bash
# sqlc のコードを再生成する — internal/adapter/repository/sqlcgen 配下に差分が出てはならない
cd backend-go
docker compose run --rm sqlc generate
```

```bash
# DB の受け入れテスト (compose の db サービスが起動している必要がある。TEST_DATABASE_URL がなければテストは黙ってスキップされる)
cd backend-go
TEST_DATABASE_URL='postgres://postgres:password@localhost:5433/postgres?sslmode=disable' go test ./db/...
```

### エンドポイント

**ヘルス**
- `GET /up` — ヘルスチェック (DB ping)

**認証**
- `POST /signup` — アカウントを作成する (username、email、password)
- `POST /login` — 認証して JWT トークンを受け取る
- `POST /logout` — 現在のセッションを無効化する (認証必須)

**店舗**
- `GET /shops` — 店舗の一覧を取得する
- `GET /shops/:id` — 店舗を 1 件取得する
- `POST /shops` — 店舗を投稿する (認証必須)

**レビュー**
- `GET /reviews` — 全レビューの一覧を取得する
- `GET /reviews/:id` — レビューを 1 件取得する
- `POST /reviews` — レビューを作成する (認証必須)
- `PUT /reviews/:id` — レビューを更新する (認証必須)
- `DELETE /reviews/:id` — レビューを削除する (認証必須)

**ユーザー**
- `GET /users` — 全ユーザーの一覧を取得する
- `PUT /users/:id` — ユーザーを更新する (認証必須。本人のみ、usecase で強制)
- `DELETE /users/:id` — ユーザーを削除する (認証必須。本人のみ、usecase で強制)

**管理者** (認証必須。管理者のみという判断は usecase で強制)
- `GET /admin/shops` — モデレーション用に店舗の一覧を取得する
- `PUT /admin/shops/:id` — 店舗を更新する
- `POST /admin/shops/:id/approve` — 投稿された店舗を承認する
- `POST /admin/shops/:id/reject` — 投稿された店舗を却下する

## フロントエンド (`frontend/`)

- **Feature-Sliced Design (FSD)** で構築 — ただし意図的に `app`、`pages`、`shared` の 3 レイヤーのみに限定している
- `features`、`entities`、`widgets` の各レイヤーは持たない — 小さく保つ
- コンポーネント開発用に Storybook が設定されている
- まずはプレーンな HTML で始める (独自のデザインシステムはまだない)

```bash
# 起動 (:5173 で提供)
cd frontend
docker compose up --build
```

```bash
# 検証 — チェックは build のみで、その中の `tsc -b` が型チェックを兼ねる。
# package.json に lint や test のスクリプトはない。
cd frontend
pnpm run build
```

**ディレクトリ構成** (`src/`)：
```
app/
  router/        # React Router の設定
  providers/
  styles/
pages/
  review-list/
  review-detail/
  review-new/
  review-edit/
  signup/ signin/ signout/
  user-detail/ user-update/
shared/
  ui/            # Button, Input, Textarea, RatingSelect
  lib/
    api.ts       # API クライアント
    date.ts
    types/
      review.ts
```

## データベーススキーマ

6 つのテーブルで、`backend-go/db/migrations/` のマイグレーションで定義されている：

- **users** — id、email、username、password_digest、admin フラグ、soft delete (discarded_at)
- **shops** — name、モデレーションステータス (pending/active/rejected)、moderation_note、作成者 FK
- **burgers** — 結合テーブルを介して店舗に紐づくバーガー
- **shops_burgers** *(結合テーブル)* — shop_id (FK)、burger_id (FK)
- **reviews** — rating、comment、user FK、burger FK
- **burger_stats** — バーガーごとのレビュー由来の集計

### リレーションシップ

```
users    1 ──0..* reviews
burgers  1 ──0..* reviews
shops   *──────* burgers  (via shops_burgers)
```
