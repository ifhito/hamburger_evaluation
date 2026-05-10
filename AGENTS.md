# AGENTS.md

## Stack

Ruby 3.3 / Rails 8 API と React 19 / TypeScript / Vite の SPA を PostgreSQL 16 で動かす monorepo。

## Build & Test

```bash
# install
cd backend && docker compose build api
cd frontend && pnpm install --frozen-lockfile

# dev
cd backend && docker compose up --build
cd frontend && pnpm run dev

# test
cd backend && docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
cd frontend && pnpm run test

# typecheck
cd frontend && pnpm run type-check

# lint
cd backend && docker compose run --rm api bin/rubocop -f github
cd frontend && pnpm run lint

# format
cd backend && docker compose run --rm api bin/rubocop -A
cd frontend && pnpm exec eslint . --fix
```

## Conventions

- Backend は host Ruby ではなく Docker Compose 経由で検証する。
  なぜ: ローカル Ruby 差異ではなく CI と同じ Rails/PostgreSQL 前提で判断するため。

- 認証は `devise_token_auth` ではなく custom JWT Bearer token を使う。
  なぜ: 現行実装が login/signup のレスポンス token と axios interceptor を前提にしているため。

- Rails domain code は ActiveRecord に直接依存させない。
  なぜ: 評価ロジックや値オブジェクトを DB 永続化の詳細から分離するため。

- Controllers/jobs から直接 read/write の ActiveRecord 呼び出しを増やさない。
  なぜ: read は query、write は repository、use case は service に寄せて境界を保つため。

- Backend API は snake_case、frontend code は camelCase にする。
  なぜ: Rails の自然な JSON 形と TypeScript 側の自然な状態形を HTTP 境界で変換するため。

## Programmatic checks the agent MUST run before finishing

1. `git status --short --branch --untracked-files=all` と `git diff --check`。
2. Backend を変更した場合: `cd backend && docker compose run --rm -e RAILS_ENV=test api bundle exec rspec`。
3. Backend を変更した場合: `cd backend && docker compose run --rm api bin/rubocop -f github` と `cd backend && docker compose run --rm api bin/brakeman --no-pager`。
4. Frontend を変更した場合: `cd frontend && pnpm run type-check && pnpm run lint && pnpm run test`。
5. Routing/build 設定または API 境界を変更した場合: `cd frontend && pnpm run build`。

## Out of scope

- `.env`, `.env.*`, `backend/.env*`, `frontend/.env*`, `secrets/**`, `backend/.kamal/secrets`, `backend/config/master.key` の読み書き。
- ユーザーが明示していない `SETUP.md`, `plans/*.md`, `memory/*`, `plan/*` の変更。
- unrelated files の stage / commit / push。
- `git push --force`, destructive reset, production deploy, secret rotation。
- Claude/Codex/Hermes の global config や `~/.hermes`, `~/.claude` への変更。

## More context (load on demand)

Use the researcher subagent when you need to locate code patterns; never grep yourself in the parent context.

- `@docs/agent/backend.md`
- `@docs/agent/frontend.md`
- `@docs/agent/workflow.md`
- `@harness-audit.md`
