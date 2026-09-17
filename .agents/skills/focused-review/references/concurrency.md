# Lens: Concurrency

In-process parallelism: shared state, goroutine lifecycles, cancellation.
(Database-level races belong to the consistency lens.)

## Hunt

1. **Unsynchronized shared state** — maps/slices/structs written from
   multiple goroutines without a mutex; lazy init without `sync.Once`;
   check-then-set on shared fields.
2. **Goroutine leaks** — goroutines with no exit path: blocked forever on a
   channel nobody reads, or looping without a `ctx.Done()` check.
3. **Missing context propagation** — `context.Background()` deep in call
   chains instead of the request ctx; DB/HTTP calls that outlive a canceled
   request.
4. **Unbounded fan-out** — a goroutine per item over user-scaled input with
   no worker pool or semaphore.
5. **WaitGroup/channel misuse** — `Add` after `Wait` races; send on a channel
   after close; forgetting `close` where a `range` reads.
6. **Frontend races** — state updates after unmount; concurrent mutations to
   the same SWR key without serialization; stale closures capturing old
   state in async callbacks.

## Do Not Flag

- Single-goroutine code paths — don't invent hypothetical parallelism.
- Values that are write-once-before-share (configs built in main, then read).
- `go test -race` noise candidates without a plausible interleaving — mark
  Suggestion and say to run the race detector.

## Grep Starters

```bash
grep -rn 'go func' backend-go/internal/ --include='*.go'
grep -rn 'context.Background()' backend-go/internal/ --include='*.go'
```

`go test -race ./...` is the authoritative check; suggest it whenever this
lens finds anything plausible.
