# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A hamburger review/evaluation web app. Users can register, post reviews of burgers at specific shops, and manage their profile.

## Architecture

Two separate sub-projects, each run via Docker:

| Directory | Stack | Role |
|-----------|-------|------|
| `backend-go/` | Go (1.22+), net/http + sqlc + pgx | REST API server |
| `frontend/` | React (TypeScript) + Vite | SPA client |

The backend follows clean architecture (handler → usecase → domain): dependencies point inward, and authorization decisions live in usecase/domain, not in handlers.

## Backend (`backend-go/`)

- Serves on **:8080**; health check at `GET /up`
- Runs with its own dedicated Postgres (host port 5433)
- Authentication: **JWT** — token is returned on login and must be sent as `Authorization: Bearer <token>`. The API fails loudly at boot when `JWT_SECRET` is unset; export it before `docker compose up`.
- API JSON uses **snake_case** on the wire; the frontend converts casing at its HTTP boundary.

```bash
# Start (serves on :8080; health check at GET /up)
cd backend-go
docker compose up --build
```

```bash
# Validation (gofmt / go vet / go build / go test), run from the repo root
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

```bash
# Migrations — by default they target the dev DB; override with MIGRATE_DATABASE_URL
cd backend-go
docker compose run --rm migrate up
docker compose run --rm migrate down -all
```

```bash
# Seed idempotent dev fixtures (admin + alice/bob/charlie, shops, burgers,
# reviews, burger_stats) — run after `migrate up`; override with DATABASE_URL
cd backend-go
docker compose run --rm seed
```

```bash
# Regenerate sqlc code — must produce zero diff under internal/adapter/repository/sqlcgen
cd backend-go
docker compose run --rm sqlc generate
```

```bash
# DB acceptance tests (compose db service must be up; tests skip silently without TEST_DATABASE_URL)
cd backend-go
TEST_DATABASE_URL='postgres://postgres:password@localhost:5433/postgres?sslmode=disable' go test ./db/...
```

### Endpoints

**Health**
- `GET /up` — health check (DB ping)

**Auth**
- `POST /signup` — create account (username, email, password)
- `POST /login` — authenticate and receive JWT token
- `POST /logout` — invalidate current session (auth required)

**Shops**
- `GET /shops` — list shops
- `GET /shops/:id` — get a single shop
- `POST /shops` — submit a shop (auth required)

**Reviews**
- `GET /reviews` — list all reviews
- `GET /reviews/:id` — get a single review
- `POST /reviews` — create a review (auth required)
- `PUT /reviews/:id` — update a review (auth required)
- `DELETE /reviews/:id` — delete a review (auth required)

**Users**
- `GET /users` — list all users
- `PUT /users/:id` — update a user (auth required; self-only, enforced in usecase)
- `DELETE /users/:id` — delete a user (auth required; self-only, enforced in usecase)

**Admin** (auth required; admin-only decision enforced in usecase)
- `GET /admin/shops` — list shops for moderation
- `PUT /admin/shops/:id` — update a shop
- `POST /admin/shops/:id/approve` — approve a submitted shop
- `POST /admin/shops/:id/reject` — reject a submitted shop

## Frontend (`frontend/`)

- Built with **Feature-Sliced Design (FSD)** — but intentionally limited to three layers only: `app`, `pages`, `shared`
- No `features`, `entities`, or `widgets` layers — keep it small
- Storybook is configured for component development
- Start with plain HTML (no custom design system yet)

```bash
# Start (serves on :5173)
cd frontend
docker compose up --build
```

```bash
# Validation — build is the only check; `tsc -b` inside it doubles as the type check.
# There are no lint or test scripts in package.json.
cd frontend
pnpm run build
```

**Directory layout** (`src/`):
```
app/
  router/        # React Router config
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
    api.ts       # API client
    date.ts
    types/
      review.ts
```

## Database Schema

Six tables, defined by the migrations in `backend-go/db/migrations/`:

- **users** — id, email, username, password_digest, admin flag, soft delete (discarded_at)
- **shops** — name, moderation status (pending/active/rejected), moderation_note, creator FK
- **burgers** — burgers linked to shops via the join table
- **shops_burgers** *(join table)* — shop_id (FK), burger_id (FK)
- **reviews** — rating, comment, user FK, burger FK
- **burger_stats** — review-derived aggregates per burger

### Relationships

```
users    1 ──0..* reviews
burgers  1 ──0..* reviews
shops   *──────* burgers  (via shops_burgers)
```
