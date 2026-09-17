---
name: backend-go-boundaries
description: Use when changing the Go API under backend-go/ — handlers, usecases, domain, repositories, sqlc queries, or Go tests in hamburger_evaluation.
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [go, backend, clean-architecture, sqlc, testing]
    related_skills: [backend-go-change-validation, pr-hygiene]
---

# Backend Go Boundaries

## Overview

Use this skill for changes under `backend-go/`. The Go API serves the React
SPA in `frontend/` and follows clean architecture. Stack: Go 1.22+ standard
`net/http` routing + `sqlc` + PostgreSQL 16. No web framework, no ORM.

## Layout and Dependency Rule

```text
backend-go/
├── cmd/api/main.go        # composition root: config, DB pool, wiring, server
├── internal/
│   ├── domain/            # entities, value objects, domain errors
│   ├── usecase/           # application use cases + repository INTERFACES
│   └── adapter/
│       ├── handler/       # net/http handlers, DTOs, routing, middleware
│       ├── repository/    # implements usecase interfaces via sqlc
│       │   └── sqlcgen/   # sqlc-generated code — NEVER edit by hand
│       └── infra/         # DB pool, JWT, password hashing, config
├── db/
│   ├── migrations/        # SQL migrations
│   └── queries/           # sqlc query sources (*.sql)
├── sqlc.yaml
└── go.mod
```

Dependencies point inward only: `handler → usecase → domain`.

- `domain` imports stdlib only. No `net/http`, no `database/sql`, no `pgx`,
  no imports from `usecase`/`adapter`.
- `usecase` imports `domain` and stdlib only. Repository interfaces are
  declared in `usecase` (consumer side, Go convention), implemented in
  `adapter/repository`.
- `handler` decodes/validates requests, calls a usecase, encodes responses,
  and maps domain errors to HTTP status. No SQL, no business rules.
- `adapter/repository` is the only layer that touches sqlc/pgx. Queries live
  in `db/queries/*.sql`; regenerate with `sqlc generate`, commit the result.

## API Contract Rules

- JSON is snake_case; the frontend converts casing at its HTTP boundary.
  The TypeScript types under `frontend/src/domains/*/api/types.ts` are the
  source of truth for response shapes — keep them in sync field-for-field.
- Auth is a custom JWT Bearer scheme (`Authorization: Bearer <token>`).
- Errors use `{"error": "..."}` (single) or `{"errors": [...]}` (validation)
  with conventional status codes (401/403/404/422).
- Authorization rules live in `domain`/`usecase` (e.g. review editable only by
  its author, shop moderation admin-only), not in handlers.

## Runtime Resource Guardrails

Memory/CPU problems are configuration debt; these are defaults, not
optimizations:

- `http.Server` always sets `ReadHeaderTimeout`, `ReadTimeout`,
  `WriteTimeout`, and `IdleTimeout`. Never bare `http.ListenAndServe` —
  slow clients pile up goroutines forever without timeouts.
- Request bodies are capped with `http.MaxBytesReader` (default 1 MiB)
  before decoding.
- Every DB/outbound call takes the request `ctx`; long operations get an
  explicit `context.WithTimeout`.
- One `pgxpool` created in `main`, with explicit `MaxConns` sized against
  Postgres `max_connections` — never a pool or connection per request.
- Graceful shutdown via `server.Shutdown(ctx)` on SIGTERM, then close the
  pool, so deploys don't drop in-flight requests or leak connections.
- List endpoints paginate by default (`LIMIT` + offset/cursor); "return
  everything" is a decision, not a default.
- Containers declare memory limits, and the process respects them
  (`GOMEMLIMIT`, and `GOMAXPROCS` matching the CPU quota).

## Testing

- Table-driven tests throughout.
- `usecase`: unit tests with hand-written fake repositories (small structs in
  the test file — no mock framework).
- `handler`: `net/http/httptest` against the router with a fake usecase.
- `adapter/repository`: integration tests against real PostgreSQL via
  `docker compose` — skip with `testing.Short()`.

## Common Pitfalls

1. Editing files under `sqlcgen/` by hand instead of changing `db/queries/`.
2. Leaking `pgx`/`sql` types or sqlc row structs above the repository layer —
   map them to domain types at the repository boundary.
3. Business rules drifting into handlers because "it's just one if".
4. Response field names diverging from the frontend API types (breaks the SPA).
5. Introducing a router/DI framework — stdlib is a decision, not an accident.

## Verification Checklist

- [ ] `domain` and `usecase` have no outward imports (adapter/infra/pgx/net-http).
- [ ] sqlc output regenerated and committed if `db/queries/` changed.
- [ ] Response JSON verified against the frontend API types for changed endpoints.
- [ ] Checks in [[backend-go-change-validation]] pass.
