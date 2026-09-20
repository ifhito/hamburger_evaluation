# Backend エージェント向けメモ

Go API は `backend-go/` にあり、検証は `go-checks.sh`、DB・マイグレーション・sqlc は Docker Compose 経由で実行する。

## 技術スタック

- Go 1.22+(標準 `net/http` のルーティング。Web フレームワークも ORM も使わない)
- PostgreSQL 16(pgx)と sqlc
- `golang-jwt/jwt` と `golang.org/x/crypto`(bcrypt)による独自 JWT Bearer 認証
- 検証は `gofmt` / `go vet` / `go build` / `go test`

## 境界

- handler はリクエストのデコード・バリデーション、usecase の呼び出し、レスポンスの組み立てを行う。SQL もビジネスルールも書かない。
- usecase はユースケースを調整し、認可を判断する。永続化のインターフェースは usecase 側で宣言する(読み取りは `*Query`、書き込みは `*Repository`)。
- adapter/repository は sqlc / pgx に触れる唯一の層で、usecase のインターフェースを実装する。
- domain はフレームワークにも DB にも依存させない。

`internal/domain` に `net/http`・`database/sql`・`pgx`・`usecase`・`adapter` への依存を追加しないこと。
