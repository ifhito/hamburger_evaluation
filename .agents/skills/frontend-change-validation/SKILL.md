---
name: frontend-change-validation
description: When frontend React behavior changes, validate types, lint, and tests.
allowed-tools: [Read, Grep, Glob, Bash(pnpm run:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [react, frontend, validation]
    related_skills: [pr-self-review]
---

# Frontend Change Validation

## Overview

Use this skill when frontend behavior, API calls, state, routing, or build
configuration changes. The job is to validate TypeScript, lint, unit tests, and
build when needed.

## When to Use

- After changing React pages, hooks, API clients, forms, or state.
- After changing route or Vite/TypeScript configuration.
- Before reporting frontend work as finished.

## Job

1. Read `references/frontend-change-validation.md`.
2. Run the following script:

```bash
.agents/skills/frontend-change-validation/scripts/frontend-checks.sh
```

3. If routing/build/API boundary changed, also run `pnpm run build` from `frontend/`.

## Output

Return pass/fail per command. If failed, include the failing file/test and the
shortest actionable error.

## Common Pitfalls

1. Do not duplicate snake_case/camelCase conversion in feature code.
2. Do not read `frontend/.env*`.
3. Do not skip type-check for TypeScript changes.

## Verification Checklist

- [ ] Type check passed.
- [ ] ESLint passed.
- [ ] Vitest passed.
- [ ] Build ran when route/build/API boundary changed.
