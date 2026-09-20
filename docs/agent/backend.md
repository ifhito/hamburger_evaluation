# Backend エージェント向けメモ

Go API は `backend-go/` にあり、検証は `go-checks.sh`、DB・マイグレーション・sqlc は Docker Compose 経由で実行する。

## 技術スタック

- Go 1.22+(標準 `net/http` のルーティング。Web フレームワークも ORM も使わない)
- PostgreSQL 16(pgx)と sqlc
- `golang-jwt/jwt` と `golang.org/x/crypto`(bcrypt)による独自 JWT Bearer 認証
- 検証は `gofmt` / `go vet` / `go build` / `go test`

## 境界

- handler はリクエストのデコード・バリデーション、usecase の呼び出し、レスポンスの組み立てを行う。SQL もビジネスルールも書かない。
- usecase はユースケースを調整し、認可を判断する。読み取りは usecase が宣言する `*Query` を使い、書き込みは domain の書き込みオブジェクトを通す。**usecase は repository を宣言も保持も呼び出しもしない。**
- domain は書き込みの契約(`*Repository` の interface)と、それを呼ぶ集約ごとの書き込みオブジェクト(`Shops` / `Reviews` / `Users`)を持つ。repository を呼べるのは domain のコードだけで、単一の集約だけを更新する書き込みに `*Service` は使わない(`*Service` は複数の集約を跨ぐ更新だけ)。
- adapter の `query` と `repository`(と `rowmap`)は sqlc / pgx に触れる唯一の層で、`query` は usecase の `*Query` を、`repository` は domain の `*Repository` を実装する。
- domain はフレームワークにも DB にも依存させない。ドメインのルールの判断は domain だけが持ち、API は結果(422 のメッセージ、`can_*` などの値)を返す。

`internal/domain` に `net/http`・`database/sql`・`pgx`・`usecase`・`adapter` への依存を追加しないこと。
