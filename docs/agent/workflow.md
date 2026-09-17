# Agent Workflow

## Before Editing

```bash
git status --short --branch
```

Identify changed areas and avoid unrelated local files.

## Checks

Backend:

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Frontend:

```bash
cd frontend
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

## Secrets

Never read or include `.env*`, `secrets/**`, `backend/.kamal/secrets`, or `backend/config/master.key`.
