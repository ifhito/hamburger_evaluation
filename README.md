# Hamburger Evaluation

ハンバーガーのレビュー投稿アプリです。ユーザー登録、ログイン、レビュー投稿、プロフィール更新ができます。構成は `backend-go/` の Go API と `frontend/` の React SPA に分かれています。

## Stack

- `backend-go/`: Go 1.27, net/http + sqlc + pgx, PostgreSQL 16, JWT auth
- `frontend/`: React 19, TypeScript, Vite, React Router, SWR, Jotai, axios, Storybook

## Repository Layout

```text
.
├── backend-go/  # Go API
├── frontend/    # React SPA
└── plan/        # Planning docs
```

## Main Features

- JWT ベースのサインアップ / サインイン / サインアウト
- ショップ一覧、ショップ詳細、ショップ申請と管理者による承認 / 却下
- レビュー一覧、詳細、作成、編集、削除
- ユーザー詳細、プロフィール更新、退会
- レビュー由来のバーガー統計

## Local Development

フロントエンドとバックエンドは別々に起動します。

### 1. Backend

`JWT_SECRET` が未設定だと API は起動時にエラーになります。事前に export してください。

```bash
cd backend-go
export JWT_SECRET=<任意のシークレット>
docker compose up --build
```

API は `http://localhost:8080` で起動します（ヘルスチェックは `GET /up`）。専用の PostgreSQL はホストポート 5433 で公開されます。

別ターミナルで初回セットアップ（マイグレーションと開発用シードデータ投入）を行います。

```bash
cd backend-go
docker compose run --rm migrate up
docker compose run --rm seed
```

### 2. Frontend

```bash
cd frontend
docker compose up --build
```

フロントエンドは `http://localhost:5173` で起動します。

## Testing

### Backend

リポジトリルートから検証スクリプト（gofmt / go vet / go build / go test）を実行します。

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

DB 受け入れテストは compose の db サービス起動中に実行します（`TEST_DATABASE_URL` 未設定時はスキップされます）。

```bash
cd backend-go
TEST_DATABASE_URL='postgres://postgres:password@localhost:5433/postgres?sslmode=disable' go test ./db/...
```

### Frontend

lint（ESLint）、型チェック、テスト（Vitest）、ビルドを実行します。CI でも同じ 4 つを実行しています。

```bash
cd frontend
pnpm run lint
pnpm run type-check
pnpm run test
pnpm run build
```

UI 確認には Storybook が使えます。

```bash
cd frontend
pnpm run storybook
```

## API Overview

- `GET /up`
- `POST /signup`
- `POST /signup/confirm`
- `POST /login`
- `POST /logout`
- `GET /.well-known/oauth-authorization-server`
- `GET /oauth/authorize`
- `POST /oauth/token`
- `POST /oauth/revoke`
- `GET /shops`
- `POST /shops`
- `GET /shops/:id`
- `GET /reviews`
- `GET /reviews/:id`
- `POST /reviews`
- `PUT /reviews/:id`
- `DELETE /reviews/:id`
- `GET /users/:id`
- `PUT /users/:id`
- `DELETE /users/:id`
- `GET /admin/shops`
- `PUT /admin/shops/:id`
- `POST /admin/shops/:id/approve`
- `POST /admin/shops/:id/reject`

## Notes

- API の JSON は snake_case です。フロントエンドのコードは camelCase で、変換は HTTP 境界（`frontend/src/api/client/buildApiClient.ts`）で行います。
- 認証付き API は `Authorization: Bearer <token>` を前提にしています。
- `/admin/*` と `/users/:id` の更新・削除の認可判定（管理者のみ・本人のみ）は usecase 層で行います。
- `GET /users/:id` は認証が任意で、email と admin を返すのは本人が閲覧したときだけです（他人・匿名には id と username のみ。判断は domain 層）。
- フロントエンドは `app` / `domains` / `api` / `states` / `components` / `lib` / `locale` の構成です。詳細は `frontend/README.md` を参照してください。
