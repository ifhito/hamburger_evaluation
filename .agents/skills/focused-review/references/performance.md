# Lens: Performance

Review the *shape* of the work, not micro-optimizations: how many round trips,
how much data, how often. Flag work that grows with data size on a hot path.

## Hunt

Backend:

1. **N+1 queries** — a query inside a loop over query results. Fix with a
   JOIN, a batch query (`WHERE id = ANY($1)`), or a precomputed map.
2. **Unbounded result sets** — list queries with no `LIMIT`/pagination.
3. **Missing indexes** — new `WHERE`/`ORDER BY`/`JOIN` columns without an
   index in the migration; foreign keys are the usual omission.
4. **Over-fetching** — `SELECT *` for three used columns; loading rows to
   count them instead of `COUNT(*)`.
5. **Per-request setup** — compiling regexes, reading config, opening
   connections inside handlers instead of at construction.
6. **O(n²) on request data** — nested membership scans over slices; use a map.

Frontend:

7. **Sequential independent awaits** — `await a(); await b();` with no
   dependency; use `Promise.all`.
8. **Fetch storms** — SWR keys rebuilt every render (inline objects);
   revalidation broader than the mutation; fetching inside a loop over items.
9. **Render waste that scales** — derived lists recomputed each render of a
   user-scaled collection without memoization.

## Do Not Flag

- Cold paths: admin screens, one-shot scripts, migrations, tests.
- Small bounded data where the simple version is clearer — say why it's fine
  if the bound isn't obvious.
- Anything needing a profiler to confirm — Suggestion + what to measure.

## Grep Starters

```bash
grep -rn 'for .*range' backend-go/internal/adapter/repository/ --include='*.go'
grep -rn 'SELECT \*' backend-go/db/queries/
```
