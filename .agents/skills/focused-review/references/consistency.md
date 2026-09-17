# Lens: Consistency

Every multi-step write must be atomic, every concurrent write safe, and
everything else out of the transaction. Ask of each write path: "what state
remains if this dies halfway?" and "what if two of these run at once?"

## Hunt

1. **Missing atomicity** — related writes without a transaction (parent +
   child, write + projection, delete + cleanup). Half committing is
   corruption.
2. **Read-modify-write races** — load, compute, save loses concurrent
   updates. Prefer atomic SQL (`SET count = count + 1`), a `WHERE` guard on
   expected state, or `SELECT ... FOR UPDATE` in the tx.
3. **Foreign work inside the tx** — HTTP calls, file I/O, sleeps while
   holding locks; a slow dependency becomes a database stall.
4. **Rollback not guaranteed** — pgx `Begin` without `defer tx.Rollback(ctx)`
   before `Commit`; early returns leaking an open tx.
5. **Retry without idempotency** — retried operations that double-insert or
   double-count; look for the unique constraint or upsert backing the retry.
6. **Check-then-act uniqueness** — SELECT-then-INSERT instead of a unique
   index + `ON CONFLICT`.
7. **Stale projections** — derived data (counts, stats) updated outside the
   tx that changed its sources, or recomputed from a stale snapshot.
8. **Migration safety** — irreversible backfill+constraint in one step;
   long migrations locking hot tables.

## Do Not Flag

- Single-statement writes (already atomic).
- Deliberately eventual projections with documented staleness and a rebuild
  path.
- Post-commit advisory work (notifications, cache invalidation) — that is the
  correct place for it.

## Grep Starters

```bash
grep -rn 'Begin\|BeginTx' backend-go/internal/ --include='*.go'
grep -rn 'ON CONFLICT\|FOR UPDATE' backend-go/db/queries/
```
