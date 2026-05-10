# Backend Change Validation Reference

## Boundary Review

Check the changed backend files for these repo-specific rules:

1. `app/domain` should not depend on ActiveRecord.
2. Controllers should authenticate, authorize, build params, and call services.
3. Jobs should not grow direct persistence logic.
4. Reads belong in query objects where a read boundary exists.
5. Writes belong in repositories where a persistence boundary exists.
6. Authorization behavior belongs in Pundit policies and policy specs.

## Checks

Run all backend checks through Docker Compose from `backend/`:

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

## Coverage Note

Targeted specs may pass examples but fail SimpleCov's global threshold. Use full
RSpec before declaring backend tests failed or complete.
