---
name: frontend-spa-boundaries
description: Use when changing the React SPA, frontend API client, domain hooks, pages, forms, state, or frontend tests in hamburger_evaluation.
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [react, frontend, typescript, testing]
    related_skills: [pr-hygiene]
---

# Frontend SPA Boundaries

## Overview

Use this skill for frontend changes under `frontend/`. The SPA uses React,
TypeScript, Vite, SWR, Jotai, react-hook-form, Zod, and axios casing
conversion at the HTTP boundary.

## When to Use

- Editing `frontend/src` or frontend tests
- Changing API request/response handling
- Adding domain hooks, pages, forms, or shared UI

## Rules

- Keep feature code under `src/domains/*`.
- Keep router/providers/app shell under `src/app`.
- Keep HTTP and casing conversion under `src/api`.
- Backend payload names are snake_case; frontend code is camelCase.
- Auth uses localStorage, Jotai state, and Authorization Bearer token injection.

## Commands

Run from `frontend/`:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

## Common Pitfalls

1. Duplicating API casing conversion in feature code.
2. Treating backend snake_case as frontend state shape.
3. Adding shared UI inside a domain when multiple domains need it.
4. Reading `frontend/.env*`.

## Verification Checklist

- [ ] Type check passes for TypeScript changes.
- [ ] Lint passes for frontend changes.
- [ ] Vitest covers changed behavior.
- [ ] Production build passes for route/build changes.
