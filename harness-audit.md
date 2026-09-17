# Harness Audit

Date: 2026-05-10
Repository: `hamburger_evaluation`
Mode: read-only investigation, except this audit file.

## 1. Repository Shape

This is a two-part monorepo:

- `backend/`: Rails API application.
- `frontend/`: React SPA application.

Other project areas:

- `memory/`, `plan/`, `plans/`: project notes and agent-generated planning artifacts.
- `.claude/`: Claude Code project settings currently exist.
- No `.agents/` directory exists yet.
- No root `docs/` directory exists; backend domain docs live under `backend/docs/domain/`.

Current local worktree already has unrelated changes:

```text
M AGENT.md
M CLAUDE.md
?? SETUP.md
?? plans/enumerated-twirling-toast.md
?? plans/parsed-munching-hedgehog.md
?? plans/setup-md-misty-charm.md
```

## 2. Backend Stack

Location: `backend/`

- Language: Ruby 3.3.10 (`.ruby-version`, `backend/.ruby-version`).
- Framework: Rails 8.0.4 API mode.
- Database: PostgreSQL 16.
- Package manager: Bundler.
- Auth: custom JWT Bearer token (`jwt`, `bcrypt`), not `devise_token_auth`.
- Authorization: Pundit.
- DDD/data-shaping libs: `dry-struct`, `dry-types`, `dry-monads`.
- Test runner: RSpec / rspec-rails.
- Test helpers: FactoryBot, shoulda-matchers, database_cleaner-active_record.
- Coverage: SimpleCov with `minimum_coverage 80` in `backend/spec/spec_helper.rb`.
- Linter: RuboCop via `rubocop-rails-omakase`.
- Security scanner: Brakeman.

Important command note: backend checks should run through Docker Compose from `backend/`, not host Ruby.

## 3. Frontend Stack

Location: `frontend/`

- Language: TypeScript 5.6.
- Framework/runtime: React 19 + Vite 6.
- Router: React Router 6.
- State/data: Jotai, SWR.
- Forms/validation: react-hook-form + Zod.
- HTTP: axios with camelcase/snakecase boundary conversion.
- Package manager: pnpm 10.
- Test runner: Vitest.
- Linter: ESLint 10 + typescript-eslint + react-hooks/react-refresh plugins.
- Type checker: `tsc --noEmit --project tsconfig.app.json`.
- Build: `tsc -b && vite build`.
- Storybook exists in package scripts.

## 4. Existing Agent Instructions

Tracked files:

- `AGENTS.md`: detailed Japanese quick reference for agents and CI.
- `AGENT.md`: project guide for AI agents; currently modified locally.
- `CLAUDE.md`: Claude/Codex guidance; currently modified locally.
- `.claude/settings.json`: tracked Claude project settings.

Missing files:

- `.cursorrules`
- `.github/copilot-instructions.md`

Existing `.claude/settings.json` is minimal and currently allows:

- `WebSearch`
- Serena MCP list_dir
- `Bash(find:*)`
- `Bash(ls:*)`
- `Bash(cat:*)`

No project subagents exist under `.claude/agents/`.

## 5. CI

CI is defined in `.github/workflows/ci.yml` and runs on pull requests and pushes to `main`.

Backend jobs:

- `backend_scan`: `bin/brakeman --no-pager`
- `backend_lint`: `bin/rubocop -f github`
- `backend_test`: PostgreSQL 16 service, `bundle exec rails db:test:prepare`, then `bundle exec rspec`

Frontend jobs:

- `frontend_type_check`: `pnpm run type-check`
- `frontend_lint`: `pnpm run lint`
- `frontend_test`: `pnpm run test`
- `frontend_build`: `pnpm run build`

CI installs Ruby from `.ruby-version`, Node from `.node-version`, and pnpm v10.

## 6. Boundaries and Architecture

Backend boundary:

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

Frontend boundary:

```text
frontend/src/app           router/providers/app shell
frontend/src/domains       feature domains: auth, reviews, shops, users
frontend/src/api           HTTP boundary and casing conversion
frontend/src/states        shared Jotai state
frontend/src/components    shared UI components
```

## 7. Non-Standard or Project-Specific Conventions

- Backend validation uses Docker Compose, not host Ruby.
- Authentication intentionally uses custom JWT Bearer tokens instead of SETUP.md's `devise_token_auth` pattern.
- Backend API payloads are snake_case; frontend code is camelCase; conversion happens at the HTTP boundary.
- Rails models should remain thin; controllers/jobs should not directly perform persistence queries/mutations when a query/repository boundary exists.
- Domain code should not directly depend on ActiveRecord models.
- SimpleCov can make targeted RSpec runs exit non-zero when examples pass but global coverage is below 80%; full suite is the authoritative coverage check.
- Existing untracked `SETUP.md` and `plans/*.md` should not be committed unless explicitly requested.
- PR #3 is currently the consolidated open PR; older #1 and #2 were closed after absorption/supersession.

## 8. Programmatic Checks Found

Backend, from `backend/`:

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Frontend, from `frontend/`:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

Architecture checks are currently convention/spec based, not a dedicated single command. Relevant backend specs cover repository/query/service boundaries under `backend/spec/{domain,queries,repositories,services,jobs}`.

## 9. Security-Relevant Files

The harness should deny reading at least:

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

These overlap with `.gitignore`, but Claude permissions should also deny them explicitly.
