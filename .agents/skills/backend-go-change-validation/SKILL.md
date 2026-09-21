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
ツールチェーン)。ホストの Go が `go.mod` の版(1.27)より古いときは、
ビルドできないので、`golang:1.27` の Docker イメージで同じコマンドを動かす。
Docker Compose のデータベースが必要なのはリポジトリの統合テストだけ。

終了前のセンサー(`.claude/hooks/stop-sensors.py`)は、これを自動で行う: ホストの Go が
`go.mod` の版より古いときは、`golang:<その版>` の使い捨て Docker で、gofmt・`go vet`・`go build`・
`go test` を 1 回の起動で動かす(モジュールのキャッシュは名前つき volume `he-sensor-gocache`)。
DB は使わないので、DB のテストは skip される。完全な検査は CI の Backend Go が行う。Go の検査は、
`.go`・`.sql`・`go.mod`・`go.sum`・`sqlc.yaml` が変わったときだけ起動する(`.env.example` や文書では起動しない)。
センサー自体のテストは `python3 -m unittest discover -s .claude/hooks`。

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
