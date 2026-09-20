# ハーネス監査

日付: 2026-05-10
リポジトリ: `hamburger_evaluation`
モード: 読み取り専用の調査(この監査ファイルを除く)。

## 1. リポジトリの構成

これは 2 部構成の monorepo です:

- `backend/`: Rails API アプリケーション。
- `frontend/`: React SPA アプリケーション。

その他のプロジェクト領域:

- `memory/`, `plan/`, `plans/`: プロジェクトメモとエージェントが生成した計画成果物。
- `.claude/`: Claude Code のプロジェクト設定が現在存在する。
- `.agents/` ディレクトリはまだ存在しない。
- ルートの `docs/` ディレクトリは存在しない。backend のドメインドキュメントは `backend/docs/domain/` 配下にある。

現在のローカル worktree には、すでに無関係な変更がある:

```text
M AGENT.md
M CLAUDE.md
?? SETUP.md
?? plans/enumerated-twirling-toast.md
?? plans/parsed-munching-hedgehog.md
?? plans/setup-md-misty-charm.md
```

## 2. Backend スタック

場所: `backend/`

- 言語: Ruby 3.3.10(`.ruby-version`, `backend/.ruby-version`)。
- フレームワーク: Rails 8.0.4 API mode。
- データベース: PostgreSQL 16。
- パッケージマネージャー: Bundler。
- 認証: 独自 JWT Bearer token(`jwt`, `bcrypt`)。`devise_token_auth` ではない。
- 認可: Pundit。
- DDD / データ整形ライブラリ: `dry-struct`, `dry-types`, `dry-monads`。
- テストランナー: RSpec / rspec-rails。
- テストヘルパー: FactoryBot, shoulda-matchers, database_cleaner-active_record。
- カバレッジ: `backend/spec/spec_helper.rb` の `minimum_coverage 80` による SimpleCov。
- Linter: `rubocop-rails-omakase` 経由の RuboCop。
- セキュリティスキャナー: Brakeman。

重要なコマンドに関する注意: backend のチェックは、ホストの Ruby ではなく、`backend/` から Docker Compose 経由で実行すべきである。

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
- `AGENT.md`: AI エージェント向けのプロジェクトガイド。現在ローカルで変更されている。
- `CLAUDE.md`: Claude / Codex 向けのガイダンス。現在ローカルで変更されている。
- `.claude/settings.json`: 追跡されている Claude のプロジェクト設定。

存在しないファイル:

- `.cursorrules`
- `.github/copilot-instructions.md`

既存の `.claude/settings.json` は最小限で、現在は次を許可している:

- `WebSearch`
- Serena MCP list_dir
- `Bash(find:*)`
- `Bash(ls:*)`
- `Bash(cat:*)`

`.claude/agents/` 配下にプロジェクトの subagent は存在しない。

## 5. CI

CI は `.github/workflows/ci.yml` で定義されており、pull request と `main` への push で実行される。

Backend ジョブ:

- `backend_scan`: `bin/brakeman --no-pager`
- `backend_lint`: `bin/rubocop -f github`
- `backend_test`: PostgreSQL 16 service、`bundle exec rails db:test:prepare`、続いて `bundle exec rspec`

Frontend ジョブ:

- `frontend_type_check`: `pnpm run type-check`
- `frontend_lint`: `pnpm run lint`
- `frontend_test`: `pnpm run test`
- `frontend_build`: `pnpm run build`

CI は、Ruby を `.ruby-version` から、Node を `.node-version` から、pnpm を v10 でインストールする。

## 6. 境界とアーキテクチャ

Backend の境界:

```text
backend/app/controllers    HTTP boundary; auth/policy/params/service calls
backend/app/domain         domain logic/value objects; should not depend on ActiveRecord
backend/app/parameters     dry-struct input DTOs
backend/app/queries        read/query boundary
backend/app/repositories   persistence/CUD boundary
backend/app/services       application use cases
backend/app/jobs           async work, should use repository boundaries
backend/app/policies       Pundit policies
backend/app/serializers    JSON output
backend/app/models         thin ActiveRecord models
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

- Backend の検証は、ホストの Ruby ではなく Docker Compose を使う。
- 認証は、SETUP.md の `devise_token_auth` パターンではなく、意図的に独自 JWT Bearer token を使っている。
- Backend の API payload は snake_case、frontend のコードは camelCase であり、変換は HTTP 境界で行われる。
- Rails の model は薄く保つべきである。query / repository の境界が存在する場合、controller / job は永続化のクエリや更新を直接行うべきではない。
- Domain のコードは ActiveRecord model に直接依存すべきではない。
- SimpleCov により、example は通っていても全体カバレッジが 80% を下回っている場合、対象を絞った RSpec の実行が非ゼロで終了することがある。カバレッジの判断基準となるのはフルスイートである。
- 既存の未追跡の `SETUP.md` と `plans/*.md` は、明示的に求められない限り commit してはならない。
- PR #3 が現在、統合された open な PR である。古い #1 と #2 は、取り込みまたは置き換えられた後に close された。

## 8. 確認されたプログラム的チェック

Backend(`backend/` から):

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Frontend(`frontend/` から):

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

アーキテクチャのチェックは、現時点では専用の単一コマンドではなく、規約 / spec に基づくものである。関連する backend の spec は、`backend/spec/{domain,queries,repositories,services,jobs}` 配下の repository / query / service の境界を対象としている。

## 9. セキュリティ関連ファイル

ハーネスは、少なくとも次のファイルの読み取りを拒否すべきである:

```text
.env
.env.*
backend/.env
backend/.env.*
frontend/.env
frontend/.env.*
secrets/**
backend/.kamal/secrets
backend/config/master.key
```

これらは `.gitignore` と重なっているが、Claude の permissions でも明示的に deny すべきである。
