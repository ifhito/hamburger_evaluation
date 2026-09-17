# Lens: Resources

Memory and CPU incidents are unbounded-growth problems: every buffer, cache,
pool, goroutine, and loop needs a bound and a release path. Ask of each
resource: "what grows it?" and "what shrinks it?" — no answer to the second
question is the finding.

## Hunt

1. **Unclosed resources** — `resp.Body`, pgx `rows`, files, `time.NewTicker`
   without `Stop`; `defer` inside a loop postponing release until function
   exit; missing `Close` on early-return paths.
2. **Unbounded in-memory growth** — package-level maps/slices appended per
   request; caches without eviction or TTL; `io.ReadAll` on request/response
   bodies without a size cap (`http.MaxBytesReader`); loading whole tables
   into memory to filter in Go.
3. **Goroutine pileup** — servers without read/write/idle timeouts; outbound
   calls without ctx timeouts; each slow client parks goroutines and memory
   until OOM.
4. **Hot spinning** — `for {}` polling without sleep/backoff; retry loops
   without backoff or attempt caps; tickers faster than the work they check.
5. **Pool misconfiguration** — a connection/client created per request
   instead of shared; `MaxConns` unset (fd exhaustion under load) or larger
   than Postgres `max_connections`.
6. **Hot-path allocation churn** — string concatenation in loops (use
   `strings.Builder`); building large intermediate slices where streaming or
   preallocation (`make(_, 0, n)`) fits.
7. **Frontend leaks** — `setInterval`/subscriptions/listeners not cleaned up
   in `useEffect` teardown; unbounded lists rendered without pagination or
   virtualization; growing state arrays never truncated.

## Do Not Flag

- Short-lived paths (CLI, tests, migrations) where the process exit is the
  release path.
- Bounded-by-construction data (config lists, enums).
- GC or allocation micro-tuning without measurement — Suggestion + "profile
  with pprof", never Critical on speculation.

## Grep Starters

```bash
grep -rn 'io.ReadAll\|NewTicker\|go func' backend-go/internal/ --include='*.go'
grep -rn 'setInterval\|addEventListener' frontend/src/
```
