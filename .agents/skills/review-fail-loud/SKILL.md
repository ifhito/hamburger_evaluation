---
name: review-fail-loud
description: Focused review for silent failures — swallowed errors, masking defaults, and ignored results. Use when reviewing error handling or when the user asks "does this fail loud?".
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, errors, reliability]
    related_skills: [review-transactions, review-performance, pr-self-review]
---

# Review: Fail Loud

## Principle

An operation that cannot do its job must say so — to the caller, the user, or
the logs with enough context to act. A silent wrong answer is worse than a
loud crash: it corrupts data and trust before anyone notices.

## What to Hunt

Ordered by severity:

1. **Swallowed errors** — error caught (or returned) and then ignored.
   - Go: `_ = doThing()`, `val, _ := parse(x)`, `if err != nil { log.Println(err) }`
     followed by continuing as if it succeeded; blank `recover()`.
   - TS: empty `catch {}`, `.catch(() => {})`, `catch (e) { console.log(e) }`
     with execution continuing on the success path.
2. **Masking defaults** — failure converted to a plausible value: returning
   `0`, `""`, `[]`, or `nil` on error; `?? fallback` hiding a failed fetch;
   parse failure silently treated as "no data".
3. **Degraded success responses** — handler returns 200 with partial/empty
   data after a dependency failed, instead of an error status.
4. **Lost error context** — rewrapping that drops the cause (Go: `errors.New`
   instead of `fmt.Errorf("...: %w", err)`); catch-and-rethrow that discards
   the original error.
5. **Fire-and-forget async** — un-awaited promises, goroutines whose error
   channel nobody reads, background work with no failure path.
6. **Broad exception handling** — catching everything at a low layer so upper
   layers never learn something went wrong.

## Legitimate Exceptions (do not flag)

- Explicitly documented best-effort work (cache warm, metrics, logging).
- Cleanup-path errors (`defer f.Close()` on a read-only file).
- A default that is the *specified* behavior, stated in a comment or test.

## Grep Starters

```bash
grep -rn ', _ :=\|_ = ' backend-go/internal/ --include='*.go'
grep -rnE 'catch\s*(\(\w*\))?\s*\{\s*\}' frontend/src/
grep -rn 'recover()' backend-go/
```

## Output

Findings as Critical / Warning / Suggestion with `filepath:line`, the failure
that would be silent, and the loud alternative. If error handling is sound,
say so in one line. Correctness bugs outside error handling go to a normal
review pass, not this one.
