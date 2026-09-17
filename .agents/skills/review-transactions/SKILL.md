---
name: review-transactions
description: Focused review for transaction and data-consistency problems — missing atomicity, read-modify-write races, and work that doesn't belong inside a transaction. Use when reviewing writes, migrations, or multi-step persistence.
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, transactions, consistency, database]
    related_skills: [review-fail-loud, review-performance, backend-go-boundaries]
---

# Review: Transactions and Consistency

## Principle

Every multi-step write must be atomic, every concurrent write must be safe,
and everything else must stay out of the transaction. Ask of each write path:
"what state remains if this dies halfway?" and "what happens if two of these
run at once?".

## What to Hunt

1. **Missing atomicity** — two or more related writes without a transaction
   (create parent + child, write + projection update, delete + cleanup).
   Half of it committing on failure is data corruption.
2. **Read-modify-write races** — load, compute in memory, save. Two
   concurrent requests lose one update. Prefer an atomic SQL update
   (`SET count = count + 1`), a `WHERE` guard on the expected state, or
   `SELECT ... FOR UPDATE` inside the transaction.
3. **Foreign work inside the transaction** — HTTP calls, file I/O, sleeps, or
   heavy computation while holding the tx: locks are held for the duration
   and a slow dependency becomes a database stall.
4. **Rollback not guaranteed** — Go/pgx: a `Begin` without
   `defer tx.Rollback(ctx)` before the `Commit`; early returns that leak an
   open transaction or connection.
5. **Retry without idempotency** — a retried operation that double-inserts or
   double-counts. Look for unique constraints or upserts backing any retry.
6. **Check-then-act uniqueness** — SELECT-then-INSERT instead of a unique
   index + `ON CONFLICT`. The check and the act are not atomic.
7. **Non-atomic projections** — derived data (counts, averages, stats)
   updated outside the transaction that changed the source rows, or
   recomputed from a snapshot that can go stale between read and write.
8. **Migration safety** — backfill and constraint added in one irreversible
   step; long-running migration locking a hot table.

## Legitimate Exceptions (do not flag)

- Single-statement writes (already atomic).
- Deliberately eventual projections, when staleness is documented and a
  rebuild path exists.
- Advisory work after commit (notifications, cache invalidation) — that is
  the *correct* place for it.

## Grep Starters

```bash
grep -rn 'Begin\|BeginTx' backend-go/internal/ --include='*.go'
grep -rn 'ON CONFLICT\|FOR UPDATE' backend-go/db/queries/
```

## Output

Findings as Critical / Warning / Suggestion with `filepath:line`, the
interleaving or crash point that breaks consistency, and the concrete fix
(tx boundary, atomic statement, constraint). If write paths are sound, say so
in one line.
