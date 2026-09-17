# CLAUDE.md

This file provides guidance to Claude Code and other coding agents when working in this repository.

## Project Overview

Hamburger Evaluation is a hamburger review and evaluation web application.

Users can:

- sign up, log in, and log out;
- browse shops and shop details;
- post, edit, and delete burger reviews;
- update or delete their profile;
- view review-derived burger statistics.

The repository is split into a Rails API backend and a React SPA frontend.

```text
hamburger_evaluation/
├── backend/    # Ruby on Rails 8 API
├── frontend/   # React 19 + TypeScript + Vite
├── memory/     # project notes
├── plan/       # planning documents
└── plans/      # agent-generated plans
```

## Communication

The primary user language is Japanese. Prefer Japanese for summaries, status reports, and clarifying questions unless the user asks otherwise.

## Important Working Rules

- Inspect existing code, tests, and docs before changing behavior.
- Do not record secrets, tokens, credentials, or Rails/JWT secret values.
- Backend validation must run through Docker Compose, not the host Ruby installation.
- Do not commit unrelated or untracked files unless explicitly asked.
- In this repository, untracked `SETUP.md` and `plans/*.md` may exist and should not be included in unrelated commits by default.
- Before committing, check status and diff carefully.

## Backend

### Stack

- Ruby 3.3.10
- Rails 8 API mode
- PostgreSQL 16
- JWT authentication
- Pundit authorization
- dry-struct / dry-types for parameter DTOs and value objects
- RSpec / FactoryBot / SimpleCov
- RuboCop
- Brakeman

### Architecture

The backend uses lightweight DDD and dependency inversion while staying close to Rails conventions.

```text
backend/app/
├── controllers/    # HTTP boundary: auth, policy checks, params, service calls
├── domain/         # domain logic / value objects; no direct ActiveRecord dependency
├── parameters/     # dry-struct input DTOs
├── queries/        # read/query boundary; where/includes/find for read paths
├── repositories/   # persistence boundary; CUD and ActiveRecord details
├── services/       # application use cases
├── jobs/           # async work; model lookup should go through repositories
├── policies/       # Pundit policies
├── serializers/    # JSON serializers
└── models/         # ActiveRecord models kept as thin as practical
```

### Backend Design Rules

- Do not introduce direct ActiveRecord dependencies into `backend/app/domain`.
- Avoid direct model persistence calls from controllers and jobs.
  - Avoid direct `.find`, `.where`, `.includes`, `.find_by`, `.save`, `.update!`, `.discard`, `.upsert` there.
  - Move reads to `queries/` and persistence operations to `repositories/`.
- Services should express use cases and delegate persistence details to repositories.
- Lightweight Rails-style dependency injection is acceptable.
  - Example: `repository: Reviews::ReviewRepository.new`
- Do not introduce a full DI container or strict port/interface layer unless the need is clear.

### Backend Commands

Run these from `backend/`.

```bash
# Start backend
cd backend
docker compose up --build

# Full test suite. RAILS_ENV=test is required because compose defaults may differ.
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# RuboCop
cd backend
docker compose run --rm api bin/rubocop -f github

# Brakeman
cd backend
docker compose run --rm api bin/brakeman --no-pager
```

SimpleCov enforces minimum coverage. Partial spec runs may fail only because coverage is below the global threshold; verify with the full suite before treating that as a test failure.

## Frontend

### Stack

- React 19
- TypeScript
- Vite
- React Router
- SWR
- Jotai
- react-hook-form + Zod
- axios
- ESLint
- Vitest
- pnpm

### Frontend Layout

```text
frontend/src/
├── app/          # router, providers, app shell
├── domains/      # auth, reviews, shops, users
├── api/          # API client / HTTP boundary
├── states/       # global state
└── components/   # shared UI components
```

### Frontend Commands

Run these from `frontend/`.

```bash
# Start frontend
cd frontend
docker compose up --build

# Lint
cd frontend
pnpm run lint

# Type check
cd frontend
pnpm run type-check

# Test
cd frontend
pnpm run test

# Build
cd frontend
pnpm run build
```

## API and Authentication

- Backend API uses snake_case.
- Frontend code uses camelCase.
- The HTTP boundary is responsible for casing conversion.
- Authentication uses custom JWT Bearer tokens, not `devise_token_auth`.
- Authenticated requests should send `Authorization: Bearer <token>`.

Main endpoints:

```text
POST   /signup
POST   /login
POST   /logout
GET    /shops
GET    /shops/:id
GET    /reviews
GET    /reviews/:id
POST   /reviews
PUT    /reviews/:id
DELETE /reviews/:id
GET    /users
PUT    /users/:id
DELETE /users/:id
```

## Quality Gates

For backend changes, normally run:

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

For frontend changes, normally run:

```bash
cd frontend
pnpm run lint
pnpm run type-check
pnpm run test
```

## Git and PR Workflow

- Check `git status --short --branch` before and after changes.
- Keep commits scoped to the requested area.
- Do not include unrelated untracked files.
- Before committing, run `git diff --check` or `git diff --cached --check`.
- After pushing a PR branch, verify the PR with `gh pr view` when `gh` is available.
