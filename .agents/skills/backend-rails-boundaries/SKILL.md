---
name: backend-rails-boundaries
description: Use when changing Rails API controllers, services, repositories, queries, policies, domain objects, or backend specs in hamburger_evaluation.
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [rails, backend, ddd, testing]
    related_skills: [pr-hygiene]
---

# Backend Rails Boundaries

## Overview

Use this skill for backend changes under `backend/`. The project uses a
lightweight DDD split around controllers, services, repositories, queries,
policies, and framework-independent domain code.

## When to Use

- Editing `backend/app/controllers`, `app/services`, `app/repositories`, `app/queries`, `app/domain`, or `app/policies`
- Adding or changing backend specs
- Reviewing backend architecture boundaries

## Rules

- Domain objects must not depend on ActiveRecord.
- Controllers authenticate, authorize, validate params, and call services.
- Services coordinate use cases and should not become persistence dumps.
- Repositories own persistence writes.
- Queries own read models.
- Pundit policies own authorization decisions.
- API payloads remain snake_case at the backend boundary.

## Commands

Run from `backend/`:

```bash
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
```

Targeted RSpec may pass examples but fail SimpleCov coverage. Final judgment
uses the full backend suite.

## Common Pitfalls

1. Adding `.find`, `.where`, `.save`, `.update!`, or `.discard` directly in controllers/jobs.
2. Moving framework concerns into `app/domain`.
3. Forgetting policy specs when changing authorization.
4. Reading `.env*`, `backend/.kamal/secrets`, or `backend/config/master.key`.

## Verification Checklist

- [ ] Changed code follows the controller/service/repository/query boundary.
- [ ] Authorization changes have Pundit coverage.
- [ ] Full backend RSpec passes when backend behavior changed.
- [ ] RuboCop and Brakeman pass for backend changes.
