# AGENTS.md

## 技術スタック

Go 1.22+ の API(標準 `net/http` + sqlc + pgx)と React 19 / TypeScript / Vite の SPA を PostgreSQL 16 で動かす monorepo。API のエンドポイント一覧とデータベーススキーマは `CLAUDE.md` を参照する。

## ビルドとテスト

```bash
# インストール
cd backend-go && docker compose build api-go
cd frontend && pnpm install --frozen-lockfile

# 開発サーバー起動(API は JWT_SECRET が未設定だと起動時に落ちる。docker compose up の前に export する)
cd backend-go && docker compose up --build
cd frontend && pnpm run dev

# テスト
.agents/skills/backend-go-change-validation/scripts/go-checks.sh   # gofmt / go vet / go build / go test(リポジトリのルートから)
cd frontend && pnpm run test

# 型チェック
cd frontend && pnpm run type-check

# lint
cd frontend && pnpm run lint

# フォーマット
cd backend-go && gofmt -w .
cd frontend && pnpm exec eslint . --fix

# sqlc の再生成(db/queries/ を変更したとき)
cd backend-go && docker compose run --rm sqlc generate
```

## 規約

- Backend の DB を使うコマンド(起動、マイグレーション、sqlc、seed)は Docker Compose 経由で実行する。
  なぜ: 開発用 DB と同じ PostgreSQL 16 の前提で判断するため。

- DB の統合テストは、compose の `db` サービスを起動し `TEST_DATABASE_URL` を渡して実行する。
  なぜ: `TEST_DATABASE_URL` がないとテストは黙ってスキップされ、検証したつもりになるため。

- 認証は custom JWT Bearer token を使う。
  なぜ: 現行実装が login と signup の確認(`POST /signup/confirm`)のレスポンス token と axios interceptor を前提にしているため。

- Backend はクリーンアーキテクチャ(handler → usecase → domain)を守り、`domain` は標準ライブラリだけを import する。
  なぜ: 評価ロジックや値オブジェクトを HTTP や DB 永続化の詳細から分離するため。

- usecase は repository に依存しない。永続化を読むときは `*Query`(usecase が宣言)、書くときは domain の集約ごとの書き込みオブジェクト(`*Repository` の interface は domain が宣言し、呼べるのは domain のコードだけ。`*Service` は複数の集約を跨ぐ更新だけ)を通す。詳細は `.agents/skills/backend-go-boundaries` を参照する。
  なぜ: 読み取りは query、書き込みは domain の書き込みオブジェクト経由に分けて、usecase から永続化の詳細を切り離して境界を保つため。

- ドメインのルール(何が有効か、誰に何が許されるか、導出)の判断は backend の `domain` だけが持つ。frontend は入力・説明・表示・サーバーのエラーの表示だけを行い、検証・権限の条件・定数・導出を複製しない。詳細は `.agents/skills/frontend-spa-boundaries` を参照する。
  なぜ: 複製は、片方だけ直したときに食い違い、二重管理になるため。

- sqlc の生成コード(`sqlcgen/`)は手で編集せず、`db/queries/` を変更して再生成する。
  なぜ: SQL と生成コードの食い違いを防ぐため。

- Backend API は snake_case、frontend code は camelCase にする。変換は HTTP 境界(`frontend/src/api/client/buildApiClient.ts`)で行う。
  なぜ: Go API の自然な JSON 形と TypeScript 側の自然な状態形を HTTP 境界で変換するため。

## エージェントが終了前に必ず実行しなければならないプログラム的チェック

1. `git status --short --branch --untracked-files=all` と `git diff --check`。
2. Backend を変更した場合: `.agents/skills/backend-go-change-validation/scripts/go-checks.sh`(リポジトリのルートから)。
3. `backend-go/internal/adapter/repository/`・`backend-go/internal/adapter/query/`・`backend-go/db/` を変更した場合: DB の統合テストを、`TEST_DATABASE_URL` を渡して実行する(`cd backend-go && docker compose run --rm -e JWT_SECRET=dummy -e TEST_DATABASE_URL='postgres://postgres:password@db:5432/postgres?sslmode=disable' api-go go test -race -count=1 ./...`)。手順 2 の `go-checks.sh` は `TEST_DATABASE_URL` なしで走るため、これらのテストは黙ってスキップされる。
4. `backend-go/db/queries/` を変更した場合: `cd backend-go && docker compose run --rm sqlc generate` を実行し、`internal/adapter/repository/sqlcgen` に差分が出ないこと。
5. Frontend を変更した場合: `cd frontend && pnpm run type-check && pnpm run lint && pnpm run test`。
6. Routing/build 設定または API 境界を変更した場合: `cd frontend && pnpm run build`。

## 対象外

- `.env`, `.env.*`, `backend-go/.env*`, `frontend/.env*`, `design/.env*`, `secrets/**` の読み書き。
- ユーザーが明示していない `SETUP.md`, `plans/*.md`, `memory/*`, `plan/*` の変更。
- 無関係なファイルの stage / commit / push。
- `git push --force`, destructive reset, production deploy, secret rotation。
- Claude/Codex/Hermes の global config や `~/.hermes`, `~/.claude` への変更。

## 追加コンテキスト(必要に応じて読み込む)

コードパターンを探す必要があるときは researcher subagent を使うこと。親コンテキストで自分で grep してはならない。

- `@docs/agent/backend.md`
- `@docs/agent/frontend.md`
- `@docs/agent/workflow.md`
- `@harness-audit.md`
