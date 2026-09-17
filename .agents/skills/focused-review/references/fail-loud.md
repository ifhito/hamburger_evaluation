# Lens: Fail Loud

An operation that cannot do its job must say so — to the caller, the user, or
the logs with enough context to act. A silent wrong answer is worse than a
loud crash.

## Hunt (by severity)

1. **Swallowed errors** — Go: `_ = doThing()`, `val, _ := parse(x)`,
   `if err != nil { log.Println(err) }` then continuing on the success path,
   blank `recover()`. TS: empty `catch {}`, `.catch(() => {})`,
   `catch (e) { console.log(e) }` with execution continuing.
2. **Masking defaults** — failure converted to a plausible value: `0`, `""`,
   `[]`, `nil` returned on error; `?? fallback` hiding a failed fetch; parse
   failure treated as "no data".
3. **Degraded 200s** — handler responds success with partial/empty data after
   a dependency failed.
4. **Lost context** — Go: `errors.New` where `fmt.Errorf("...: %w", err)`
   belongs; rethrow that discards the original error.
5. **Fire-and-forget async** — un-awaited promises; goroutines whose error
   nobody reads; background work with no failure path.
6. **Catch-all too low** — broad recovery at a low layer so callers never
   learn something failed.

## Do Not Flag

- Documented best-effort work (cache warm, metrics, logging).
- Cleanup-path errors (`defer f.Close()` on a read-only file).
- A default that is specified behavior, stated in a comment or test.

## Grep Starters

```bash
grep -rn ', _ :=\|_ = ' backend-go/internal/ --include='*.go'
grep -rnE 'catch\s*(\(\w*\))?\s*\{\s*\}' frontend/src/
grep -rn 'recover()' backend-go/
```
