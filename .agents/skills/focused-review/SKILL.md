---
name: focused-review
description: Generic focused-review lenses — fail-loud, consistency, concurrency, performance, security, test quality. Use when reviewing a diff for one of these concerns, or to pick the right lenses for what a diff touches.
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, reliability, consistency, performance, security, testing]
    related_skills: [review-fix, pr-self-review, backend-go-boundaries]
---

# Focused Review

## Overview

A catalog of generic review lenses. Each lens fixes attention on one class of
defect; a focused pass finds what a do-everything review skims past. Pick the
lenses that match what the diff touches, read only those references, and
review with that checklist.

## Picking Lenses

| Lens | Read when the diff touches | Reference |
|------|---------------------------|-----------|
| Fail loud | error handling, external calls, parsing, async work | `references/fail-loud.md` |
| Consistency | writes, transactions, migrations, derived data | `references/consistency.md` |
| Concurrency | goroutines, channels, shared state, caches | `references/concurrency.md` |
| Performance | queries, list endpoints, loops over data, UI fetching | `references/performance.md` |
| Security | auth, input handling, SQL, file paths, secrets | `references/security.md` |
| Test quality | new or changed tests, or behavior changes without tests | `references/test-quality.md` |

Default to the 2–3 lenses the diff most obviously touches. Run every lens only
when explicitly asked for an exhaustive pass.

## Shared Rules (apply to every lens)

Output:

- Findings as Critical / Warning / Suggestion, each with `filepath:line`,
  the concrete failure (inputs/state → wrong outcome), and the fix.
- If a lens finds nothing, say so in one line. Never pad.
- Issues outside the chosen lenses go to a normal review pass — note them in
  one line, do not expand.

Suppressing false positives:

- Each reference has a "do not flag" list; respect it.
- Uncertain findings are Suggestions with what to measure or test, never
  Criticals on speculation.
- Do not report style nits that gofmt, go vet, or ESLint would catch.

## Repo Grounding

Concrete patterns in the references assume this repo's stack: Go
(`backend-go/`, net/http + sqlc + pgx) and React/TypeScript (`frontend/`,
SWR + axios). The principles are stack-agnostic; update the patterns if the
stack changes.
