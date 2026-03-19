# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A hamburger review/evaluation web app. Users can register, post reviews of burgers at specific shops, and manage their profile.

## Architecture

Three separate sub-projects, each run via Docker:

| Directory | Stack | Role |
|-----------|-------|------|
| `backend/` | Ruby on Rails (API mode) | REST API server |
| `frontend/` | React (TypeScript) | SPA client |
| `database/` | PostgreSQL | Persistent storage |

## Development

All services are run via Docker. Start each component from its respective directory.

```bash
# Each service is started from its directory
docker compose up   # (from backend/, frontend/, or database/)
```

API documentation is served via Swagger (configured in the Rails backend).

## Backend

- Rails in **API mode** (no views/assets)
- API endpoints are documented in `backend/docs/API/` as YAML files
- Authentication: **JWT** — token is returned on login and must be sent as a Bearer token

### Endpoints

**Auth**
- `POST /signup` — create account (username, email, password)
- `POST /login` — authenticate and receive JWT token
- `POST /logout` — invalidate current session

**Reviews**
- `GET /reviews` — list all reviews
- `GET /reviews/:id` — get a single review
- `POST /reviews` — create a review (auth required)
- `PUT /reviews/:id` — update a review (auth required)
- `DELETE /reviews/:id` — delete a review (auth required)

**Users**
- `GET /users` — list all users
- `POST /users` — create a user
- `PUT /users/:id` — update a user
- `DELETE /users/:id` — delete a user

**Shops** — *not yet implemented; see Future Plans below*

## Frontend

- Built with **Feature-Sliced Design (FSD)** — but intentionally limited to three layers only: `app`, `pages`, `shared`
- No `features`, `entities`, or `widgets` layers — keep it small
- Storybook is configured for component development
- Start with plain HTML (no custom design system yet)

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

Five tables in total:

**users**: id (Rails PK), email, encrypted_password, username

**shops**: id, name

**burgers**: id, shops_and_burgers_id (FK)

**reviews**: id, rating, comment, users_id (FK), burgers_id (FK)

**shops_and_burgers** *(join table)*: id, shop_id (FK), burger_id (FK)

### Relationships

```
users    1 ──0..* reviews
burgers  1 ──0..* reviews
shops   *──────* burgers  (via shops_and_burgers)
```

- A user can post many reviews
- A burger can have many reviews
- Shops and burgers share a many-to-many relationship through `shops_and_burgers`

## Testing Policy

**Backend uses TDD (Test-Driven Development) as the default approach.**

When adding new features or fixing bugs in `backend/`:

1. 🔴 Write a failing test first
2. 🟢 Write the minimum implementation to make it pass
3. 🔵 Refactor

**Test stack**: RSpec + FactoryBot + Faker + Shoulda Matchers + DatabaseCleaner + SimpleCov

**Coverage target**: 80% minimum (enforced via `simplecov`)

```bash
# Run all tests with coverage report
# NOTE: RAILS_ENV=test must be passed explicitly because docker-compose.yml sets RAILS_ENV=development
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec

# File-specific run
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec spec/models/user_spec.rb

# View coverage report
open backend/coverage/index.html
```

---

## Future Plans

The following are intentionally deferred and **not** part of the MVP:

- **Shops API** (`shops.yaml` is a placeholder): store listing/search endpoints will be added later
- **Burger name and details**: `burgers` currently holds only `id` and `shops_and_burgers_id`; columns for name, ingredients, and price are planned for a future iteration
