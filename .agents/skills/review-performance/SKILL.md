---
name: review-performance
description: Focused review for performance problems — N+1 queries, unbounded result sets, missing indexes, needless sequential awaits, and hot-path waste. Use when reviewing queries, list endpoints, or UI data fetching.
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, performance, database, frontend]
    related_skills: [review-transactions, review-fail-loud, backend-go-boundaries]
---

# Review: Performance

## Principle

Performance review is about *shape*, not micro-optimization: how many round
trips, how much data, how often. Flag work that grows with data size on a hot
path; ignore constant-factor tuning the profiler hasn't asked for.

## What to Hunt

Backend:

1. **N+1 queries** — a query inside a loop over query results. Fix with a
   JOIN, a batch query (`WHERE id = ANY($1)`), or a precomputed map.
2. **Unbounded result sets** — list queries with no `LIMIT`/pagination; an
   endpoint that returns the whole table grows until it falls over.
3. **Missing indexes** — new `WHERE` / `ORDER BY` / `JOIN` columns without an
   index in the migration; foreign keys are the usual omission.
4. **Over-fetching** — `SELECT *` when three columns are used; loading rows
   to count them instead of `COUNT(*)`; fetching relations the response
   never serializes.
5. **Per-request setup in the hot path** — compiling regexes, reading config,
   or opening connections inside a handler instead of at construction.
6. **O(n²) on request data** — nested loops doing membership checks over
   slices; use a map.

Frontend:

7. **Sequential independent awaits** — `await a(); await b();` where neither
   depends on the other; use `Promise.all`.
8. **Fetch storms** — SWR keys that change every render (inline objects),
   revalidation broader than the mutation (invalidate one list, not all),
   fetching in a loop over items.
9. **Render waste that scales** — recomputing derived lists on every render
   of a large collection without memoization. Flag only when the collection
   is user-scaled, not for three items.

## Legitimate Exceptions (do not flag)

- Cold paths: admin screens, one-shot scripts, migrations, tests.
- Small bounded data where the simple version is clearer — say why it's fine
  if the bound is not obvious.
- Anything a profiler would need to confirm — mark it Suggestion, not
  Critical, and say what to measure.

## Grep Starters

```bash
grep -rn 'for .*range' backend-go/internal/adapter/repository/ --include='*.go'
grep -rn 'SELECT \*' backend-go/db/queries/
grep -rnE 'await .+\n\s*await' frontend/src/ -U
```

## Output

Findings as Critical / Warning / Suggestion with `filepath:line`, how the cost
grows (per row? per request? per render?), and the concrete fix. If the diff
is performance-clean, say so in one line. Style and correctness issues go to
their own review passes.
