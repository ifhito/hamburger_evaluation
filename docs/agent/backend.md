# Backend Agent Notes

Rails API lives in `backend/` and runs through Docker Compose.

## Stack

- Ruby 3.3.10 / Rails 8 API mode
- PostgreSQL 16
- Custom JWT Bearer auth with `bcrypt` and `jwt`
- Pundit policies
- dry-struct / dry-types for domain values
- RSpec, FactoryBot, SimpleCov, RuboCop, Brakeman

## Boundaries

- Controllers authenticate, authorize, validate params, and call services.
- Services coordinate use cases.
- Repositories own persistence writes.
- Queries own read models.
- Domain objects stay framework-independent.

Do not add ActiveRecord dependencies to `app/domain`.
