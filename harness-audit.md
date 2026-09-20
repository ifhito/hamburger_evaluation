# ハーネス監査

初版: 2026-05-10 / 更新: 2026-09-20(Go 前提のハーネスの実態に合わせて書き直した)
リポジトリ: `hamburger_evaluation`
モード: 読み取り専用の調査(この監査ファイルを除く)。

## 1. リポジトリの構成

これは 2 部構成の monorepo です:

- `backend-go/`: Go の API アプリケーション。
- `frontend/`: React SPA アプリケーション。

その他のプロジェクト領域:

- `memory/`, `plan/`, `plans/`: プロジェクトメモとエージェントが生成した計画成果物。
- `.claude/`: Claude Code のプロジェクト設定(`agents/`, `hooks/`, `skills/`, `workflows/`, `settings.json`)。
- `.agents/`: Claude / Codex / Hermes で共有する `skills/` と、Hermes 用の設定・agent 定義(`hermes/`)。
- `docs/`: エージェント向けの文書(`docs/agent/`、`docs/agent-onboarding.md`)。

## 2. Backend スタック

場所: `backend-go/`

- 言語: Go 1.22(`go.mod`)。
- HTTP: 標準 `net/http` のルーティング。Web フレームワークも ORM も使わない。
- データベース: PostgreSQL 16(pgx)。クエリは sqlc で生成する。マイグレーションは `db/migrations/`。
- パッケージマネージャー: Go modules。
- 認証: 独自 JWT Bearer token(`golang-jwt/jwt`)。パスワードのハッシュは `golang.org/x/crypto/bcrypt`。
- 写真の保存: disk(既定)または S3 互換のストレージ(`aws-sdk-go-v2`)。
- 検証: `gofmt`、`go vet`、`go build`、`go test`。テストの方針は `.agents/skills/backend-go-boundaries` に書かれている(usecase は手書きのフェイク、handler は `net/http/httptest`、repository は実 PostgreSQL に対する統合テスト)。

重要なコマンドに関する注意: DB を使うコマンド(起動、マイグレーション、sqlc、seed)は、`backend-go/` から Docker Compose 経由で実行する。DB の統合テストは `TEST_DATABASE_URL` を渡さないと黙ってスキップされる。

## 3. Frontend スタック

場所: `frontend/`

- 言語: TypeScript 5.6。
- フレームワーク / ランタイム: React 19 + Vite 6。
- ルーター: React Router 6。
- 状態 / データ: Jotai, SWR。
- フォーム / バリデーション: react-hook-form + Zod。
- HTTP: camelcase / snakecase の境界変換を行う axios。
- パッケージマネージャー: pnpm 10。
- テストランナー: Vitest。
- Linter: ESLint 10 + typescript-eslint + react-hooks / react-refresh プラグイン。
- 型チェッカー: `tsc --noEmit --project tsconfig.app.json`。
- ビルド: `tsc -b && vite build`。
- package scripts に Storybook が存在する。

## 4. 既存のエージェント向け指示

追跡されているファイル:

- `AGENTS.md`: エージェントと CI 向けの、日本語による詳細なクイックリファレンス。
- `AGENT.md`: AI エージェント向けのプロジェクトガイド。
- `CLAUDE.md`: Claude Code 向けのガイダンス。プロジェクト概要、アーキテクチャ、コマンド、エンドポイント、スキーマを持つ。
- `docs/agent/*.md`、`docs/agent-onboarding.md`: 領域別のメモと、ハーネスの入口。
- `.claude/settings.json`: 追跡されている Claude のプロジェクト設定。

存在しないファイル:

- `.cursorrules`
- `.github/copilot-instructions.md`

`.claude/settings.json` の内容:

- 許可(allow): 読み取り・編集、`git` / `gh` の一般的な操作、`docker`、`go` / `gofmt` / `sqlc`、`pnpm run …`、検証スクリプト、フックの実行。
- 確認(ask): `gh pr edit`、`gh pr close`、`gh pr merge`。
- 拒否(deny): 秘密情報ファイルの読み取り(下の「セキュリティ関連ファイル」)、`rm -rf`、`git push --force`、`git rebase`。
- フック: `Stop` で `python3 .claude/hooks/stop-sensors.py` を実行する。

subagent(`.claude/agents/`):

- `orchestrator`: 実装とレビューを統括する(worktree の作成、draft PR、レビュー、検証、自動修正)。
- `implementer`: スコープを絞ったタスクを実装し、検証して報告する。stage / commit / push はしない。
- `reviewer`: 差分を repo の境界に照らして批判的にレビューする(読み取り専用)。
- `verifier`: レビューの指摘を実際のコードと突き合わせて、確認 / 反証 / 不確定に分ける(読み取り専用)。
- `researcher`: 実装前に、コードのパターンを探して行番号つきで示す。
- `tester`: 対象を絞った frontend のテストを実行し、失敗だけを要約する。

skill(`.agents/skills/`。`.claude/skills/` からは symlink で参照する):

- `backend-go-boundaries` / `backend-go-change-validation` / `db-design`: Go API の境界、検証、DB 設計。
- `frontend-spa-boundaries` / `frontend-change-validation`: SPA の境界と検証。
- `focused-review` / `review-fix` / `pr-self-review`: レビューと自動修正。
- `github-story` / `pr-template` / `pr-hygiene`: story の起票、PR 本文、コミットと PR の衛生。

workflow(`.claude/workflows/`):

- `review-fix.js`: 未コミットの変更を reviewer でレビューし、verifier で指摘を検証(V1)して、自動修正してよいものを implementer で直す(最大 3 ラウンド)。

## 5. CI

CI は `.github/workflows/ci.yml` で定義されており、pull request と `main` への push で実行される。

Frontend ジョブ:

- `frontend_type_check`: `pnpm run type-check`
- `frontend_lint`: `pnpm run lint`
- `frontend_test`: `pnpm run test`
- `frontend_build`: `pnpm run build`

Node は `.node-version` から、pnpm は v10 でインストールする。

**Go API(`backend-go/`)を検証する CI ジョブは、現時点で存在しない。** Go の検証は、手元の `go-checks.sh` とフックの sensor に依存している。

## 6. 境界とアーキテクチャ

Backend の境界:

```text
backend-go/cmd/api                        composition root: 設定、DB プール、配線、サーバ
backend-go/internal/domain                エンティティ / 値オブジェクト / ドメインエラー。標準ライブラリのみ
backend-go/internal/usecase               ユースケースと永続化のインターフェース(利用側で宣言)
backend-go/internal/adapter/handler       net/http のハンドラ、DTO、ルーティング、middleware
backend-go/internal/adapter/repository    usecase のインターフェースを sqlc で実装
backend-go/internal/adapter/repository/sqlcgen   sqlc の生成コード。手で編集しない
backend-go/internal/adapter/infra         DB プール、JWT、パスワードハッシュ、設定
backend-go/db/migrations                  SQL マイグレーション
backend-go/db/queries                     sqlc のクエリ
```

Frontend の境界:

```text
frontend/src/app           router/providers/app shell
frontend/src/domains       feature domains: auth, reviews, shops, users
frontend/src/api           HTTP boundary and casing conversion
frontend/src/states        shared Jotai state
frontend/src/components    shared UI components
```

## 7. 非標準またはプロジェクト固有の規約

- Backend の DB を使うコマンドは Docker Compose を使う。DB の統合テストは `TEST_DATABASE_URL` を渡して実行する。
- 認証は、独自 JWT Bearer token を使っている。
- Backend の API payload は snake_case、frontend のコードは camelCase であり、変換は HTTP 境界(`frontend/src/api/client/buildApiClient.ts`)で行われる。
- Backend はクリーンアーキテクチャ(handler → usecase → domain)で、依存は内側にのみ向く。`domain` は標準ライブラリだけを import する。
- 永続化のインターフェースは usecase 側で宣言する。読み取りは `*Query`、書き込みは `*Repository` に分ける(`.agents/skills/backend-go-boundaries`)。
- sqlc の生成コードは手で編集せず、`db/queries/` を変更して再生成する。ドメインの形と DB の形は別々に設計する(`.agents/skills/db-design`)。
- コード内の文章(コメント、Go の doc コメント、テスト名)は日本語で書く。PR の本文も日本語で、固定のセクション構成に従う(`.agents/skills/pr-template`)。
- 既存の未追跡の `SETUP.md` と `plans/*.md` は、明示的に求められない限り commit してはならない。

## 8. 確認されたプログラム的チェック

Backend(リポジトリのルートから):

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh   # gofmt / go vet / go build / go test
```

Backend(`backend-go/` から。`db/queries/` を変更したとき):

```bash
docker compose run --rm sqlc generate   # internal/adapter/repository/sqlcgen に差分が出てはならない
```

Frontend(`frontend/` から):

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

`.claude/hooks/stop-sensors.py`(`Stop` フックで実行される)の sensor:

- 常に確認するもの: `git status`、秘密情報らしいパスが作業ツリーにないこと、対象外のパス(`plans/`、`memory/`、`plan/`、`SETUP.md`)が stage されていないこと(`AGENT_ALLOW_OUT_OF_SCOPE_STAGED=1` で解除できる)、`git diff --check`。
- `backend-go/` に変更があるとき: `domain` と `usecase` から `net/http`・`database/sql`・`pgx`・`adapter` への import がないこと、usecase の永続化インターフェースで読み取りと書き込みが混ざっていないこと(Query / Repository の分割)、`gofmt` / `go vet` / `go build` / `go test`。
- `frontend/` のソースや設定に変更があるとき: `type-check` / `lint` / `test` / `build`。

## 9. セキュリティ関連ファイル

ハーネスは、少なくとも次のファイルの読み取りを拒否すべきである:

```text
.env
.env.*
backend-go/.env
backend-go/.env.*
backend/.env
backend/.env.*
backend/.kamal/secrets
backend/.kamal/secrets/**
backend/config/master.key
frontend/.env
frontend/.env.*
secrets/**
**/secrets/**
```

`.env*` は `.gitignore` と重なっているが、Claude の permissions でも明示的に deny すべきである。
