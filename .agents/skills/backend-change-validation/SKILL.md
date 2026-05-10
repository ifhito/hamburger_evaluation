---
name: backend-change-validation
description: When backend Rails behavior changes, validate boundaries and checks.
allowed-tools: [Read, Grep, Glob, Bash(docker compose run:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [rails, backend, validation]
    related_skills: [pr-self-review]
---

# Backend Change Validation

## Overview

Use this skill when backend Rails code changes. The job is to verify the change
against this repo's lightweight DDD boundaries and run backend checks through
Docker Compose.

## When to Use

- After changing controllers, services, repositories, queries, policies, jobs, or domain code.
- After adding backend specs or changing authorization.
- Before reporting backend work as finished.

## Job

1. Read `references/backend-change-validation.md`.
2. Run the following script:

```bash
.agents/skills/backend-change-validation/scripts/backend-checks.sh
```

3. Report command results and any boundary concerns.

## Output

Return pass/fail per command. If failed, include the shortest relevant error
and one next fix candidate.

## Common Pitfalls

1. Do not use host Ruby for backend validation.
2. Do not treat targeted RSpec coverage failure as final without full RSpec.
3. Do not add ActiveRecord dependencies to domain code.

## Verification Checklist

- [ ] Backend checks ran from `backend/` through Docker Compose.
- [ ] Domain/persistence boundaries were reviewed.
- [ ] Authorization changes include policy coverage.
