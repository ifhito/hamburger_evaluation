# Backend

Rails 8 の API サーバーです。ユーザー認証、レビュー CRUD、ショップ参照、ユーザー更新を担当します。

## Stack

- Ruby on Rails 8
- PostgreSQL 16
- JWT authentication
- RSpec / FactoryBot / SimpleCov
- Docker Compose

## Main Responsibilities

- `POST /signup`, `POST /login`, `POST /logout`
- `GET /shops`, `GET /shops/:id`
- `GET /reviews`, `GET /reviews/:id`, `POST /reviews`, `PUT /reviews/:id`, `DELETE /reviews/:id`
- `GET /users`, `PUT /users/:id`, `DELETE /users/:id`
- バーガー統計更新ジョブ

## Directory Guide

```text
backend/
├── app/
│   ├── controllers/
│   ├── models/
│   ├── queries/
│   ├── serializers/
│   └── jobs/
├── config/
├── db/
├── docs/API/
└── spec/
```

## Run Locally

```bash
docker compose up --build
```

API server: `http://localhost:3000`

初回セットアップ:

```bash
docker compose run --rm api bundle exec rails db:prepare
```

## Test

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
```

個別実行:

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec spec/requests/reviews_spec.rb
```

## API Docs

YAML ベースの API ドキュメントは `docs/API/` にあります。

## Auth

- ログイン成功時に JWT を返します。
- 認証が必要な API は `Authorization: Bearer <token>` を送ります。

## Notes

- `config/master.key` はコミットしていません。
- ローカルの生成物やログは `.gitignore` で除外しています。
- CORS 設定は `config/initializers/cors.rb` にあります。
