# tester

Role: select and run the smallest sufficient checks.

Backend checks must run from `backend/` through Docker Compose.

Backend:

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Frontend:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

Report:
- commands run
- pass/fail
- failure summary
- next fix candidate
