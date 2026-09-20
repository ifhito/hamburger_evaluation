# Backend エージェント向けメモ

Rails API は `backend/` にあり、Docker Compose 経由で実行する。

## 技術スタック

- Ruby 3.3.10 / Rails 8 API mode
- PostgreSQL 16
- `bcrypt` と `jwt` による独自 JWT Bearer 認証
- Pundit policy
- ドメインの値に dry-struct / dry-types を使用
- RSpec, FactoryBot, SimpleCov, RuboCop, Brakeman

## 境界

- Controller は認証・認可・パラメータ検証を行い、service を呼び出す。
- Service はユースケースを調整する。
- Repository は永続化の書き込みを担う。
- Query は read model を担う。
- Domain オブジェクトはフレームワークに依存させない。

`app/domain` に ActiveRecord への依存を追加しないこと。
