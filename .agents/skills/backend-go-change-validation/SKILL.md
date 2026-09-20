---
name: backend-go-change-validation
description: backend-go/ 配下の Go API の挙動が変わったときに使う。フォーマット、vet、ビルド、テストを検証する。
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

## 概要

`backend-go/` 配下の Go コードが変わったときにこのスキルを使う。仕事は
[[backend-go-boundaries]] のクリーンアーキテクチャ境界を検証し、Go の
チェックを実行すること。Go ツールチェーンはホスト上で動く(単一の静的
ツールチェーン)。Docker Compose のデータベースが必要なのはリポジトリの
統合テストだけ。

## チェック

すべて `backend-go/` から実行するか、同梱スクリプトを使う:

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

これは以下と等価:

```bash
cd backend-go
test -z "$(gofmt -l .)"        # formatting
go vet ./...                   # static analysis
go build ./...                 # compile everything
go test ./...                  # unit + handler tests (repository tests skip without DB)
```

`db/queries/` または `sqlc.yaml` を変更した場合は、追加で:

```bash
cd backend-go
sqlc generate
git diff --exit-code -- internal/adapter/repository/sqlcgen   # no drift
```

リポジトリ実装を変更した場合は、データベースを起動して統合テストを実行する。
`TEST_DATABASE_URL` を渡さないと、統合テストは黙ってスキップされる(検証したことにならない):

```bash
cd backend-go
docker compose run --rm -e JWT_SECRET=dummy -e TEST_DATABASE_URL='postgres://postgres:password@db:5432/postgres?sslmode=disable' api-go go test ./internal/adapter/repository/...
```

## 境界の抜き打ちチェック

終える前に、外向きの import を grep する(いずれも何も返らないこと):

```bash
grep -rE '"net/http"|database/sql|pgx|/adapter/|/usecase/' backend-go/internal/domain/
grep -rE '"net/http"|database/sql|pgx|/adapter/'           backend-go/internal/usecase/
```

## 報告

実行したコマンドと pass/fail、スキップしたチェックとその理由、見つかった
境界違反を報告する。フォーマットや vet の失敗はメモではなくハードストップ。
