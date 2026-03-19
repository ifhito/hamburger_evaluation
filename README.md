# Hamburger Evaluation

ハンバーガーのレビュー投稿アプリです。ユーザー登録、ログイン、レビュー投稿、プロフィール更新ができます。構成は `backend/` の Rails API と `frontend/` の React SPA に分かれています。

## Stack

- `backend/`: Ruby on Rails 8 API, PostgreSQL, JWT auth, RSpec
- `frontend/`: React 19, TypeScript, Vite, React Router, TanStack Query, Storybook

## Repository Layout

```text
.
├── backend/   # Rails API
├── frontend/  # React SPA
└── plan/      # Planning docs
```

## Main Features

- JWT ベースのサインアップ / サインイン / サインアウト
- ショップ一覧、ショップ詳細
- レビュー一覧、詳細、作成、編集、削除
- ユーザー詳細、プロフィール更新、退会
- バーガー統計の更新ジョブ

## Local Development

フロントエンドとバックエンドは別々に起動します。

### 1. Backend

```bash
cd backend
docker compose up --build
```

API は `http://localhost:3000` で起動します。

別ターミナルで初回セットアップを行います。

```bash
cd backend
docker compose run --rm api bundle exec rails db:prepare
```

### 2. Frontend

```bash
cd frontend
docker compose up --build
```

フロントエンドは `http://localhost:5173` で起動します。

## Testing

### Backend

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
```

### Frontend

現状は Storybook ベースの UI 確認が中心です。

```bash
cd frontend
docker compose run --rm frontend npm run storybook
```

## API Overview

- `POST /signup`
- `POST /login`
- `POST /logout`
- `GET /shops`
- `GET /shops/:id`
- `GET /reviews`
- `GET /reviews/:id`
- `POST /reviews`
- `PUT /reviews/:id`
- `DELETE /reviews/:id`
- `GET /users`
- `PUT /users/:id`
- `DELETE /users/:id`

詳細は `backend/docs/API/` を参照してください。

## Notes

- `.gitignore` で `frontend/node_modules`, `frontend/dist`, `backend/log`, `backend/tmp`, `backend/coverage`, `backend/config/master.key` などは除外しています。
- 認証付き API は `Authorization: Bearer <token>` を前提にしています。
- フロントエンドは小さめの FSD 構成として `app`, `pages`, `shared` に絞っています。
