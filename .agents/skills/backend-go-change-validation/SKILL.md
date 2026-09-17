---
name: backend-go-change-validation
description: When Go API behavior under backend-go/ changes, validate formatting, vet, build, and tests.
allowed-tools: [Read, Grep, Glob, Bash(go:*), Bash(gofmt:*), Bash(sqlc:*), Bash(docker compose run:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [go, backend, validation]
    related_skills: [backend-go-boundaries, pr-self-review]
---

# Backend Go Change Validation

## Overview

Use this skill when Go code under `backend-go/` changes. The job is to verify
the clean-architecture boundaries from [[backend-go-boundaries]] and run the
Go checks. The Go toolchain runs on the host (a single static toolchain — no
version drift risk like Ruby); only repository integration tests need the
Docker Compose database.

## Checks

Run everything from `backend-go/`, or use the bundled script:

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

Which is equivalent to:

```bash
cd backend-go
test -z "$(gofmt -l .)"        # formatting
go vet ./...                   # static analysis
go build ./...                 # compile everything
go test ./...                  # unit + handler tests (repository tests skip without DB)
```

When `db/queries/` or `sqlc.yaml` changed, additionally:

```bash
cd backend-go
sqlc generate
git diff --exit-code -- internal/adapter/repository/sqlcgen   # no drift
```

When repository implementations changed, run integration tests with the
database up:

```bash
cd backend-go
docker compose run --rm api-go go test ./internal/adapter/repository/...
```

## Boundary Spot-Checks

Before finishing, grep for outward imports (each should return nothing):

```bash
grep -rE '"net/http"|database/sql|pgx|/adapter/|/usecase/' backend-go/internal/domain/
grep -rE '"net/http"|database/sql|pgx|/adapter/'           backend-go/internal/usecase/
```

## Reporting

Report commands run with pass/fail, any skipped checks and why, and boundary
violations found. A formatting or vet failure is a hard stop, not a note.
