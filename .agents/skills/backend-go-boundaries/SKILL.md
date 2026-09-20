---
name: backend-go-boundaries
description: hamburger_evaluation の backend-go/ 配下の Go API(ハンドラ、ユースケース、ドメイン、リポジトリ、sqlc クエリ、Go テスト)を変更するときに使う。
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [go, backend, clean-architecture, sqlc, testing]
    related_skills: [backend-go-change-validation, db-design, pr-hygiene]
---

# Backend Go Boundaries

## 概要

`backend-go/` 配下の変更にはこのスキルを使う。この Go API は `frontend/` の
React SPA にサービスを提供し、クリーンアーキテクチャに従う。スタック: Go 1.22+ の
標準 `net/http` ルーティング + `sqlc` + PostgreSQL 16。Web フレームワークも ORM も使わない。

## レイアウトと依存関係のルール

```text
backend-go/
├── cmd/api/main.go        # composition root: config, DB pool, wiring, server
├── internal/
│   ├── domain/            # entities, value objects, domain errors
│   ├── usecase/           # application use cases + repository INTERFACES
│   └── adapter/
│       ├── handler/       # net/http handlers, DTOs, routing, middleware
│       ├── repository/    # implements usecase interfaces via sqlc
│       │   └── sqlcgen/   # sqlc-generated code — NEVER edit by hand
│       └── infra/         # DB pool, JWT, password hashing, config
├── db/
│   ├── migrations/        # SQL migrations
│   └── queries/           # sqlc query sources (*.sql)
├── sqlc.yaml
└── go.mod
```

依存は内側にのみ向く: `handler → usecase → domain`。

## ドメインと DB の分離

**ドメイン設計と DB 設計は別の活動であり、互いを鏡写しにしてはならない。**
スキーマはデータ整合性とクエリの形に奉仕し、ドメインは振る舞いと不変条件に
奉仕する。両者のマッピングは `adapter/repository` が担う。

- ドメイン型はテーブル行の形をコピーしない。カラムからではなく、振る舞い
  (値オブジェクト、状態遷移)から設計する。
- sqlc の行構造体や `pgtype`/`sql` 型を `adapter/` の外に出さない。
- スキーマ変更が機械的にドメイン変更を強制してはならず、その逆も同様 —
  両者の形が乖離するのは想定内であり、健全なこと。
- スキーマ作業は `db-design` スキルに従う。

- `domain` は stdlib のみを import する。`net/http` も `database/sql` も
  `pgx` も、`usecase`/`adapter` からの import も禁止。
- `usecase` は `domain`、stdlib、および**副作用のない純粋な内部ライブラリ**
  (例: 画像のデコード/リサイズを行う `internal/photo`。DB・HTTP・ファイル I/O に
  依存しないもの)だけを import する。`adapter/*` や `net/http`・`database/sql`・`pgx` は
  import しない。永続化のインターフェースは `usecase` 側で宣言する
  (利用側で宣言する Go の慣習)。**読み取りと書き込みでインターフェースを分ける**:
  - `*Query`(例: `ShopQuery`): **読み取り専用**。メソッド名は `Get*` / `List*`。
  - `*Repository`(例: `ShopRepository`): **書き込み専用**。メソッド名は
    `Create*` / `Update*` / `Discard*`。書き込みが更新後の行(`RETURNING`)を返すのは
    よいが、読み取りのメソッドを置いてはならない。
  - usecase が repository を呼ぶのは**書き込みのときだけ**。読み取りは必ず Query を通す。
  - 書き込みの内部で必要な読み取り(例: 同一トランザクション内のロック取得)は、
    adapter の repository の実装の内部に閉じる。
- `handler` はリクエストのデコード/バリデーション、ユースケース呼び出し、
  レスポンスのエンコード、ドメインエラーから HTTP ステータスへのマッピングを行う。
  SQL もビジネスルールも書かない。
- `adapter/repository` は sqlc/pgx に触れる唯一の層。クエリは
  `db/queries/*.sql` に置き、`sqlc generate` で再生成して結果をコミットする。

## API 契約のルール

- JSON は snake_case。ケーシング変換はフロントエンドの HTTP 境界で行う。
  `frontend/src/domains/*/api/types.ts` 配下の TypeScript 型が
  レスポンス形状の source of truth — フィールド単位で同期を保つこと。
- 認証はカスタム JWT Bearer 方式(`Authorization: Bearer <token>`)。
- エラーは `{"error": "..."}`(単一)または `{"errors": [...]}`(バリデーション)で、
  慣例的なステータスコード(401/403/404/422)を使う。
- 認可ルール(例: レビューは作者のみ編集可、ショップのモデレーションは管理者のみ)は
  `domain`/`usecase` に置き、ハンドラには置かない。

## コード内の文章は日本語で書く

新規・変更するコードの**人間向けの文章は日本語**で書く。

- **日本語にするもの**: コメント(`//`、`/* */`、`--`、`#`)、Go の doc コメント、
  テスト名(`t.Run("…")` の文字列)。
- Go の doc コメントは慣習どおり**識別子名で始める**(godoc/linter 互換):
  `// ShopRepository はショップの永続化契約(利用側で宣言)。` のように「識別子名 + は/を」で書く。
- タグは保持し本文だけ日本語にする: `// TODO(S7): 統計の再計算を呼ぶ`。
- 技術用語(fail-loud、tx、ctx、race、N+1 など)は無理に訳さず原語のままでよい。
- **英語のままにするもの**:
  - 識別子(関数名・型名・カラム名など)
  - コンパイラ/ツールへの指示: `//go:build`、`//go:embed`、`//nolint`、`// Code generated`、
    **sqlc の `-- name: Xxx :one` 注釈**(壊すと `sqlc generate` が壊れる)
  - API の外部契約の文字列: エラーレスポンスのメッセージ(`"Name can't be blank"` など)と JSON キー
  - ログメッセージ(運用時の検索性を優先して英語)
- 既存の英語コメントは、その行を触るときに日本語へ直す(まとめての一括翻訳は別 PR で行う)。

## 実行時リソースのガードレール

メモリ/CPU の問題は設定の負債である。以下は最適化ではなくデフォルト:

- `http.Server` は必ず `ReadHeaderTimeout`、`ReadTimeout`、
  `WriteTimeout`、`IdleTimeout` を設定する。素の `http.ListenAndServe` は禁止 —
  タイムアウトがないと遅いクライアントが goroutine を際限なく積み上げる。
- リクエストボディはデコード前に `http.MaxBytesReader`(デフォルト 1 MiB)で
  上限を設ける。
- すべての DB/外部呼び出しはリクエストの `ctx` を受け取る。長時間の処理には
  明示的な `context.WithTimeout` を設定する。
- `pgxpool` は `main` で 1 つだけ作り、Postgres の `max_connections` に対して
  適切な `MaxConns` を明示する — リクエストごとのプールやコネクションは禁止。
- SIGTERM で `server.Shutdown(ctx)` によるグレースフルシャットダウンを行い、
  その後プールを閉じる。デプロイで処理中のリクエストを落としたり
  コネクションをリークさせたりしないため。
- 一覧系エンドポイントはデフォルトでページネーションする(`LIMIT` +
  offset/cursor)。「全件返す」は意思決定であって、デフォルトではない。
- コンテナはメモリ上限を宣言し、プロセスはそれを尊重する
  (`GOMEMLIMIT`、および CPU クォータに合わせた `GOMAXPROCS`)。

## テスト

- 全体をテーブル駆動テストで書く。
- `usecase`: 手書きのフェイクリポジトリ(テストファイル内の小さな構造体 —
  モックフレームワークは使わない)によるユニットテスト。
- `handler`: フェイクのユースケースを使い、ルーターに対して `net/http/httptest` でテスト。
- `adapter/repository`: `docker compose` で実際の PostgreSQL に対する
  統合テスト — `testing.Short()` でスキップ可能にする。

## よくある落とし穴

1. `db/queries/` を変更する代わりに `sqlcgen/` 配下のファイルを手で編集してしまう。
2. `pgx`/`sql` 型や sqlc の行構造体をリポジトリ層より上にリークさせる —
   リポジトリ境界でドメイン型へマッピングすること。
3. 「if 一つだけだから」とビジネスルールがハンドラに流れ込む。
4. レスポンスのフィールド名がフロントエンドの API 型から乖離する(SPA が壊れる)。
5. ルーターや DI フレームワークを導入する — stdlib の採用は偶然ではなく意思決定。
6. コメントやテスト名を英語で書く(上記の例外を除き日本語で書く)。
7. `*Repository` に `Get*` / `List*` を足す、`*Query` に `Create*` / `Update*` /
   `Discard*` を足す(読み取りと書き込みを同じインターフェースに混ぜる)。

## 検証チェックリスト

- [ ] `domain` と `usecase` に外向きの import(adapter/infra/pgx/net-http)がない。
- [ ] usecase の `*Repository` に読み取り(`Get*` / `List*`)が、`*Query` に書き込みがない。
      `usecase` が import する内部ライブラリは、副作用のない純粋なものに限られる。
- [ ] `db/queries/` を変更した場合、sqlc の出力を再生成しコミットした。
- [ ] 変更したエンドポイントについて、レスポンス JSON をフロントエンドの API 型と突き合わせた。
- [ ] 追加・変更したコメントとテスト名が日本語になっている(例外は「コード内の文章は日本語で書く」を参照)。
- [ ] [[backend-go-change-validation]] のチェックが通る。
