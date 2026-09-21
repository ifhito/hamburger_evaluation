---
name: backend-go-boundaries
description: hamburger_evaluation の backend-go/ 配下の Go API(ハンドラ、ユースケース、ドメイン、リポジトリ、sqlc クエリ、Go テスト)を変更するときに使う。
version: 1.0.0
author: BurgerStack Agents
license: MIT
metadata:
  hermes:
    tags: [go, backend, clean-architecture, sqlc, testing]
    related_skills: [backend-go-change-validation, db-design, pr-hygiene]
---

# Backend Go Boundaries

## 概要

`backend-go/` 配下の変更にはこのスキルを使う。この Go API は `frontend/` の
React SPA にサービスを提供し、クリーンアーキテクチャに従う。スタック: Go 1.27 の
標準 `net/http` ルーティング + `sqlc` + PostgreSQL 16。Web フレームワークも ORM も使わない。

Go の版(`go.mod` の `go` 行と Dockerfile の `golang:` タグ)は、その時点の最新の安定版にする。
「必要な最低の版」を、そのまま採用する版にしない(最初に Go 1.22 のまま置いて、後から 1.27 へ
引き上げる手間が出た)。最新の版は https://go.dev/dl/?mode=json の `stable` で確かめ、
新しい版が出たら、依存の要求に合わせて引き上げる PR を出す。

## レイアウトと依存関係のルール

```text
backend-go/
├── cmd/api/main.go        # composition root: config, DB pool, wiring, server
├── internal/
│   ├── domain/            # entities, value objects, domain errors, *Repository (write ports) + *Service that calls them
│   ├── usecase/           # application use cases + *Query (read ports); never depends on a repository
│   └── adapter/
│       ├── handler/       # net/http handlers, DTOs, routing, middleware
│       ├── query/         # implements usecase *Query (reads) via sqlc
│       ├── repository/    # implements domain *Repository (writes) via sqlc
│       │   └── sqlcgen/   # sqlc-generated code — NEVER edit by hand
│       ├── rowmap/        # sqlc row -> domain mapping shared by query and repository
│       └── infra/         # DB pool, JWT, password hashing, config
├── db/
│   ├── migrations/        # SQL migrations
│   └── queries/           # sqlc query sources (*.sql)
├── sqlc.yaml
└── go.mod
```

依存は内側にのみ向く: `handler → usecase → domain`。**repository は domain からだけ使う**
(usecase は repository に依存しない。詳細は下の usecase の規約)。

## ドメインのルールは domain だけが判断する

ドメインのルール(何が有効か、誰に何が許されるか、状態がどう遷移するか)の**唯一の判断者は
`domain`** である。frontend・handler・adapter は、その判断を再実装しない。

- API は**判断の結果**を返す: 検証の結果は 422 とメッセージ、権限の結果は `can_edit` などの
  値、次のページの有無などの導出も backend が返す。frontend に判断させない。
- frontend が同じ規則を持たないと成立しない API を作らない(frontend に規則を複製させると、
  片方だけ直して食い違う)。判断が必要な情報は、レスポンスに含める。
- 規則の数値を、利用者への説明文に書く必要がある場合は、その説明文が backend の定数の写しで
  あることをコメントに書く(判定そのものは複製しない)。

## ドメインと DB の分離

**ドメイン設計と DB 設計は別の活動であり、互いを鏡写しにしてはならない。**
スキーマはデータ整合性とクエリの形に奉仕し、ドメインは振る舞いと不変条件に
奉仕する。両者のマッピングは `adapter`(`query` / `repository` / `rowmap`)が担う。

- ドメイン型はテーブル行の形をコピーしない。カラムからではなく、振る舞い
  (値オブジェクト、状態遷移)から設計する。
- sqlc の行構造体や `pgtype`/`sql` 型を `adapter/` の外に出さない。
- スキーマ変更が機械的にドメイン変更を強制してはならず、その逆も同様 —
  両者の形が乖離するのは想定内であり、健全なこと。
- スキーマ作業は `db-design` スキルに従う。

- `domain` は stdlib のみを import する。`net/http` も `database/sql` も
  `pgx` も、`usecase`/`adapter` からの import も禁止。domain は、書き込みの契約である
  `*Repository` の interface と、それを呼ぶ集約ごとの書き込みオブジェクト(`Shops` / `Reviews` /
  `Users`。自分の集約の `*Repository` だけを持つ)を持つ。
- `usecase` は `domain`、stdlib、および**副作用のない純粋な内部ライブラリ**
  (例: 画像のデコード/リサイズを行う `internal/photo`。DB・HTTP・ファイル I/O に
  依存しないもの)だけを import する。`adapter/*` や `net/http`・`database/sql`・`pgx` は
  import しない。**読み取りと書き込みで、依存の形を分ける**:
  - `*Query`(例: `ShopQuery`): usecase が宣言する**読み取り専用**の interface
    (利用側で宣言する Go の慣習)。メソッド名は `Get*` / `List*`。
  - `*Repository`(例: `ShopRepository`): **domain が宣言する書き込み専用**の interface。
    メソッド名は `Create*` / `Update*` / `Discard*` と、書き込みの前段の排他ロック `Lock*`(統計の再計算の
    前に、バーガーの行をロックして、並行する書き込みの取りこぼしを防ぐ。値を返さず、行も変えない)。
    書き込みが更新後の行(`RETURNING`)を返すのはよいが、読み取りのメソッドを置いてはならない。
  - **repository を呼べるのは domain のコードだけ**(`*Service` に限らない)。usecase と
    handler は repository を宣言も保持も呼び出しもせず、読み取りは `*Query`、書き込みは
    domain の書き込みオブジェクト(`domain.Shops` など)を通す。組み立て(`cmd/api/main.go`)は
    「repository → 書き込みオブジェクト → usecase」の順に行う。
  - **単一の集約だけを更新する書き込みに `*Service` を使わない**(集約の書き込みオブジェクトが担う)。
    domain の `*Service` は、複数の集約を跨ぐ更新のうち、**間に読み取りを挟まない手順**だけに使う。読み取りを挟む
    手順(例: 統計の再計算 = バーガーの行のロック → 統計の元データの読み取り → 統計の保存)は、repository だけを持つ `*Service` では
    表現できない。トランザクションを持つ usecase が、`UnitOfWork.Do`(ここからここまでの読み書きを 1 つの
    トランザクションにまとめる仕組み。途中で失敗すれば全体を取り消す)の中で、各集約の書き込みオブジェクトと
    `*Query` を組み合わせて組み立てる。書き込みオブジェクトは、自分の
    集約の repository だけを持ち、公開メソッドは 8 個まで、読み取り・認可・外部 I/O は持たない
    (業務の判断はエンティティ・値オブジェクトへ)。
  - 書き込みの内部で必要な読み取り(例: 同一トランザクション内のロック取得)は、
    adapter の repository の実装の内部に閉じる。
- `handler` はリクエストのデコード/バリデーション、ユースケース呼び出し、
  レスポンスのエンコード、ドメインエラーから HTTP ステータスへのマッピングを行う。
  SQL もビジネスルールも書かない。
- adapter の `query` と `repository`(と `rowmap`)は sqlc/pgx に触れる唯一の層。クエリは
  `db/queries/*.sql` に置き、`sqlc generate` で再生成して結果をコミットする。

## API 契約のルール

- JSON は snake_case。ケーシング変換はフロントエンドの HTTP 境界で行う。
  `frontend/src/domains/*/api/types.ts` 配下の TypeScript 型が
  レスポンス形状の source of truth — フィールド単位で同期を保つこと。
- 認証はカスタム JWT Bearer 方式(`Authorization: Bearer <token>`)。
- エラーは `{"error": "..."}`(単一)または `{"errors": [...]}`(バリデーション)で、
  慣例的なステータスコード(401/403/404/422)を使う。
- 認可ルール(例: レビューは作者のみ編集可、ショップのモデレーションは管理者のみ)は
  `domain`/`usecase` に置き、ハンドラには置かない。frontend が出し分けに使う権限は、
  backend が値(`can_edit` など)として返す(frontend に権限の条件を持たせない)。

## コード内の文章は日本語で書く

新規・変更するコードの**人間向けの文章は日本語**で書く。

- **日本語にするもの**: コメント(`//`、`/* */`、`--`、`#`)、Go の doc コメント、
  テスト名(`t.Run("…")` の文字列)。
- Go の doc コメントは慣習どおり**識別子名で始める**(godoc/linter 互換):
  `// ShopRepository はショップの書き込みの契約(domain が宣言)。` のように「識別子名 + は/を」で書く。
- TODO は、追跡用の issue 番号だけ `TODO(#123): 統計の再計算を呼ぶ` の形で 1 つ添えてよい(story・受け入れ条件の番号は書かない)。
- 技術用語(fail-loud、tx、ctx、race、N+1 など)は無理に訳さず原語のままでよい。
- **英語のままにするもの**:
  - 識別子(関数名・型名・カラム名など)
  - コンパイラ/ツールへの指示: `//go:build`、`//go:embed`、`//nolint`、`// Code generated`、
    **sqlc の `-- name: Xxx :one` 注釈**(壊すと `sqlc generate` が壊れる)
  - API の外部契約の文字列: エラーレスポンスのメッセージ(`"Name can't be blank"` など)と JSON キー
  - ログメッセージ(運用時の検索性を優先して英語)
- 既存の英語コメントは、その行を触るときに日本語へ直す(まとめての一括翻訳は別 PR で行う)。

### 読む人に伝わる書き方(テスト名・コメント)

テスト名・コメントは、**この story を知らない人が読んで分かる日本語**で書く。

1. story・受け入れ条件・issue の番号(`AC1`・`R4`・`S24`・`Story #61`・`#123`)を書かない。番号は issue と PR にだけ置く。
2. テスト名は「どんな状況で、何をしたら、どうなるか」を具体的に書き、名前を読むだけで、何を確かめているかと期待する結果が分かるようにする。
   悪い例: `AC1 本人が bio を更新すると、PUT の応答と GET に反映される`。
   良い例: `本人が自己紹介文を更新すると、更新の応答にも、あとから取得したプロフィールにも反映される`。
3. 日本語の文の中に、意味の伝わりにくい英語(略語・内部用語・識別子の断片)を混ぜない。画面・API の項目は日本語の呼び名を使い、コード上の名前は必要なときだけ括弧で添える(`自己紹介文(bio)`)。`PUT`・`GET`・`422`・`JSON`・`commit` のような一般的な技術用語はそのままでよい。
4. コメントは、コードを読めば分かること(何をしているか)を繰り返さず、理由・前提・注意点を、初めて読む人に分かるように書く。この story だけの経緯(「S28 で足した」「レビューで指摘された」)は書かない。

`stop-sensors.py` が、追加した行の `AC<数字>`・`Story #<数字>`・`issue #<数字>` などを検出する。

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
- `usecase`: 手書きのフェイク(`*Query` のフェイクと、domain の書き込みオブジェクトに渡す `*Repository` の
  フェイク。テストファイル内の小さな構造体 — モックフレームワークは使わない)によるユニットテスト。
- `handler`: フェイクのユースケースを使い、ルーターに対して `net/http/httptest` でテスト。
- `adapter/query`・`adapter/repository`: `docker compose` で実際の PostgreSQL に対する
  統合テスト — `testing.Short()` でスキップ可能にする。

## よくある落とし穴

1. `db/queries/` を変更する代わりに `sqlcgen/` 配下のファイルを手で編集してしまう。
2. `pgx`/`sql` 型や sqlc の行構造体をリポジトリ層より上にリークさせる —
   リポジトリ境界でドメイン型へマッピングすること。
3. 「if 一つだけだから」とビジネスルールがハンドラに流れ込む。
4. レスポンスのフィールド名がフロントエンドの API 型から乖離する(SPA が壊れる)。
5. ルーターや DI フレームワークを導入する — stdlib の採用は偶然ではなく意思決定。
6. コメントやテスト名を英語で書く(上記の例外を除き日本語で書く)。story・受け入れ条件の番号や、意味の伝わりにくい英語を混ぜて、story を知らない人に読めない書き方にする。
7. usecase が repository を宣言・保持・呼び出す(`.repo.` の呼び出し、`*Repository` の型・
   フィールド、`domain.*Repository` の参照)。書き込みは domain の書き込みオブジェクトを通す。
8. `*Repository` に `Get*` / `List*` を足す、`*Query` に `Create*` / `Update*` /
   `Discard*` / `Lock*` を足す(読み取りと書き込みを同じインターフェースに混ぜる)。
9. ドメインのルールの判断を、handler や frontend に置く・複製する(検証・権限・導出)。
   判断は domain に置き、API は結果を返す。

## 検証チェックリスト

- [ ] `domain` と `usecase` に外向きの import(adapter/infra/pgx/net-http)がない。
- [ ] usecase が repository を宣言・保持・呼び出していない(`.repo.` の呼び出し、`*Repository` の
      型・フィールド、`domain.*Repository` の参照がない)。domain の `*Repository` は
      `Create*` / `Update*` / `Discard*` / `Lock*`(排他ロックだけ)だけ、usecase の `*Query` は `Get*` / `List*` だけ。
      `usecase` が import する内部ライブラリは、副作用のない純粋なものに限られる。
- [ ] ドメインのルールの判断が domain にあり、API が結果(422 のメッセージ、`can_*` などの値)を
      返している。frontend に同じ判断が必要になっていない。
- [ ] `db/queries/` を変更した場合、sqlc の出力を再生成しコミットした。
- [ ] 変更したエンドポイントについて、レスポンス JSON をフロントエンドの API 型と突き合わせた。
- [ ] 追加・変更したコメントとテスト名が日本語になっている(例外は「コード内の文章は日本語で書く」を参照)。
- [ ] テスト名・コメントに story・受け入れ条件・issue の番号がなく、story を知らない人にも「状況 → 結果」「理由」が伝わる(「読む人に伝わる書き方」を参照)。
- [ ] [[backend-go-change-validation]] のチェックが通る。
