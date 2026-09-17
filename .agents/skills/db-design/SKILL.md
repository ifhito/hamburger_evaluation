---
name: db-design
description: Use when designing or changing the PostgreSQL schema — tables, constraints, indexes, migrations under backend-go/db. Enforces the domain/DB separation rule: schema and domain are designed separately and mapped in the repository layer.
allowed-tools: [Read, Grep, Glob, Bash(docker compose run:*), Bash(sqlc:*), Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [database, postgresql, schema, migrations, design]
    related_skills: [backend-go-boundaries, focused-review]
---

# DB Design

## The Separation Rule (read this first)

**DB design and domain design are different activities with different
masters, and must not mirror each other.**

- The **schema** optimizes for data integrity and query shape: normalization,
  constraints, indexes, storage-efficient representations (e.g. status as
  smallint + CHECK).
- The **domain** optimizes for behavior and invariants: value objects, state
  machines, rules (e.g. `ShopStatus` with transition methods).
- The **repository layer owns the mapping** between the two. sqlc row structs
  never leave `adapter/`; domain types never gain fields "because the table
  has them"; a schema change must not mechanically force a domain change,
  nor vice versa.

Deriving one side from the other is the Rails habit this repo is
deliberately leaving behind. When the two shapes drift apart, that is the
design working, not a problem to fix.

## Schema Principles

1. **Constraints live in the schema** — NOT NULL, CHECK, UNIQUE, FK.
   Application validation is UX; the database is the last line of defense.
   If an invariant can be expressed as a constraint, express it there too.
2. **Every FK gets an index**, plus whatever the real `WHERE`/`ORDER BY`
   needs. No speculative indexes — each one taxes every write.
3. **Prefer boring types** — bigint ids, timestamptz, text, smallint + CHECK
   for closed enums. Reach for jsonb/arrays only with a named reason.
4. **Soft delete is a column (`discarded_at`)**, and every read path must
   decide explicitly whether it sees discarded rows.
5. **Naming**: snake_case, plural tables, `<table>_id` FKs, join tables as
   `a_b` alphabetical.

## Migration Rules

- Migrations are plain SQL in `backend-go/db/migrations`, up/down paired,
  and are the single source of truth for the schema.
- Additive first: backfill and `NOT NULL`/constraint tightening are separate
  steps from column creation — never one irreversible migration.
- Destructive changes (drop column/table) get their own migration and an
  explicit user decision.
- After any schema or query change: `sqlc generate`, commit the output,
  verify drift-zero with `git diff --exit-code`.

## Design Checklist (before the migration PR)

- [ ] Each invariant either has a constraint or a written reason why not.
- [ ] FKs indexed; query-driven indexes named after the query they serve.
- [ ] up/down both tested against an empty DB (see AC pattern in stories).
- [ ] The domain model was designed from behavior, not from these tables —
      and the mapping lives in `adapter/repository`.
- [ ] Reviewed with the `focused-review` consistency lens if data is moved
      or backfilled.
