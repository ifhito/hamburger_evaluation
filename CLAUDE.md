# CLAUDE.md

このファイルは、このリポジトリで作業する Claude Code などのコーディングエージェント向けのガイダンスを提供します。

## プロジェクト概要

Hamburger Evaluation は、ハンバーガーのレビューと評価を行う Web アプリケーションです。

ユーザーは次のことができます:

- サインアップ、ログイン、ログアウト;
- ショップ一覧とショップ詳細の閲覧;
- バーガーレビューの投稿・編集・削除;
- プロフィールの更新・削除;
- レビューから算出されるバーガー統計の閲覧。

リポジトリは Go API の backend と React SPA の frontend に分かれています。

```text
hamburger_evaluation/
├── backend-go/ # Go API (net/http + sqlc + PostgreSQL 16)
├── frontend/   # React 19 + TypeScript + Vite
├── memory/     # プロジェクトメモ
├── plan/       # 計画ドキュメント
└── plans/      # エージェントが生成した計画
```

## コミュニケーション

ユーザーの主な使用言語は日本語です。ユーザーから別途指定がない限り、要約・状況報告・確認のための質問は日本語で行うことを優先してください。

## 重要な作業ルール

- 振る舞いを変更する前に、既存のコード・テスト・ドキュメントを確認する。
- シークレット、トークン、認証情報、JWT のシークレット値を記録しない。
- 明示的に依頼されない限り、無関係なファイルや未追跡ファイルを commit しない。
- このリポジトリには未追跡の `SETUP.md` や `plans/*.md` が存在することがあり、無関係な commit にはデフォルトで含めない。
- commit する前に、status と diff を注意深く確認する。

## Backend (`backend-go/`)

### 技術スタック

- Go 1.22+ (標準 `net/http` のルーティング。Web フレームワークも ORM も使わない)
- PostgreSQL 16 (pgx)
- sqlc (SQL からの型安全なコード生成)
- JWT 認証

### アーキテクチャ

クリーンアーキテクチャ (handler → usecase → domain) を採用しています。依存は内側にのみ向き、認可の判断は handler ではなく usecase / domain に置きます。

```text
backend-go/
├── cmd/api/main.go     # composition root: 設定、DB プール、配線、サーバ
├── internal/
│   ├── domain/         # エンティティ、値オブジェクト、ドメインエラー(標準ライブラリのみ)
│   ├── usecase/        # アプリケーションのユースケース + 永続化のインターフェース(利用側で宣言)
│   └── adapter/
│       ├── handler/    # net/http のハンドラ、DTO、ルーティング、middleware
│       ├── repository/ # usecase のインターフェースを sqlc で実装
│       │   └── sqlcgen/  # sqlc の生成コード。手で編集しない
│       └── infra/      # DB プール、JWT、パスワードハッシュ、設定
├── db/
│   ├── migrations/     # SQL マイグレーション
│   └── queries/        # sqlc のクエリ(*.sql)
└── sqlc.yaml
```

### Backend 設計ルール

- `domain` は標準ライブラリのみを import する。`net/http`・`database/sql`・`pgx`・`usecase`・`adapter` は import しない。
- 永続化のインターフェースは `usecase` 側で宣言し、`adapter/repository` が実装する。
- sqlc の行構造体や `pgx` の型を `adapter/` の外に出さない。ドメインの形と DB の形は別々に設計する。
- `sqlcgen/` は手で編集しない。`db/queries/` を変更して再生成する。
- API の JSON は snake_case を使う。
- 詳細は `.agents/skills/backend-go-boundaries` と `.agents/skills/db-design` を参照する。

### Backend の前提

- **:8080** で待ち受け、ヘルスチェックは `GET /up`。
- 専用の Postgres を使う (ホストのポートは 5433)。
- 認証は **JWT**。ログイン時にトークンを返し、以降は `Authorization: Bearer <token>` で送る。`JWT_SECRET` が未設定だと起動時にエラーで落ちる(fail-loud)ため、`docker compose up` の前に export する。

### Backend コマンド

```bash
# 起動 (:8080 で待ち受け。ヘルスチェックは GET /up)
cd backend-go
docker compose up --build
```

```bash
# 検証 (gofmt / go vet / go build / go test)。リポジトリのルートから実行
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
```

```bash
# マイグレーション。既定では開発用 DB が対象。MIGRATE_DATABASE_URL で上書きできる
cd backend-go
docker compose run --rm migrate up
docker compose run --rm migrate down -all
```

```bash
# 冪等な開発用フィクスチャを投入 (admin + alice/bob/charlie、ショップ、バーガー、
# レビュー、burger_stats)。`migrate up` の後に実行。DATABASE_URL で上書きできる
cd backend-go
docker compose run --rm seed
```

```bash
# sqlc の再生成。internal/adapter/repository/sqlcgen に差分が出てはならない
cd backend-go
docker compose run --rm sqlc generate
```

```bash
# DB の受け入れテスト (compose の db サービスが起動していること。
# TEST_DATABASE_URL がないとテストは黙ってスキップされる)
cd backend-go
TEST_DATABASE_URL='postgres://postgres:password@localhost:5433/postgres?sslmode=disable' go test ./db/...
```

### エンドポイント

**ヘルスチェック**
- `GET /up` — ヘルスチェック (DB への ping)

**認証**
- `POST /signup` — アカウントを作成する (username、email、password。email は形式(`net/mail` で解析でき、表示名などを含まないアドレスだけであること)を検証し、不正なら 422 `Email is invalid`。password は 8〜72 バイトで、半角英字・数字・記号をそれぞれ 1 文字以上含む。`PUT /users/:id` のパスワード変更にも同じ規則を適用するが、login は強度を検証しない。password_confirmation は任意で、送った場合は password と不一致なら 422。規則の判定は backend の domain だけが持ち、frontend は説明文の表示と、サーバーの 422 メッセージの表示だけを行う)
- `POST /login` — 認証して JWT トークンを受け取る
- `POST /logout` — 確認メッセージを返すだけ。JWT は stateless なのでサーバー側での無効化はなく、token の破棄はクライアントが行う (要認証)

**ショップ**
- `GET /shops` — ショップ一覧 (`page` / `per_page` が整数でなければ 422。空・省略は既定値、範囲外の整数は補正される)
- `GET /shops/:id` — ショップ 1 件の取得
- `POST /shops` — ショップの申請 (要認証)

**レビュー**
- `GET /reviews` — レビュー一覧 (省略可能な `user_id` クエリで、そのユーザーの公開レビューだけに絞り込める。`page` / `per_page` の扱いは `GET /shops` と同じ)
- `GET /reviews/:id` — レビュー 1 件の取得
- `POST /reviews` — レビューの投稿 (要認証)
- `PUT /reviews/:id` — レビューの更新 (要認証)
- `DELETE /reviews/:id` — レビューの削除 (要認証)

**写真**
- `GET /photos/*` — ディスクに保存されたレビュー写真を配信 (認証不要。末尾が `/` のディレクトリ path は一覧せず 404、末尾 `/` なしは 301 で `/` 付きへ転送されてから 404)。`PHOTO_STORAGE` が `disk` (既定) のときだけ登録され、`s3` では登録されない (写真の URL は bucket の公開ドメインを指す)

**ユーザー**
- `GET /users/:id` — ユーザーを 1 人取得 (認証は任意。存在しない・退会済み・整数でない id は同一の 404。本人が閲覧したときだけ email・admin を含む)
- `PUT /users/:id` — ユーザーの更新 (要認証。本人のみ。usecase で判定。email を変更するときは、signup と同じ形式の検証を行う)
- `DELETE /users/:id` — ユーザーの削除 (要認証。本人のみ。usecase で判定)

**管理者** (要認証。管理者のみ許可する判定は usecase で行う)
- `GET /admin/shops` — モデレーション用のショップ一覧
- `PUT /admin/shops/:id` — ショップの更新
- `POST /admin/shops/:id/approve` — 申請されたショップの承認
- `POST /admin/shops/:id/reject` — 申請されたショップの却下

### データベーススキーマ

`backend-go/db/migrations/` のマイグレーションで定義された 6 つのテーブル:

- **users** — id, email, username, password_digest, admin フラグ, 論理削除 (discarded_at)
- **shops** — name, モデレーション状態 (pending / active / rejected), moderation_note, 申請者への FK
- **burgers** — 中間テーブル経由でショップに紐づくバーガー
- **shops_burgers** *(中間テーブル)* — shop_id (FK), burger_id (FK)
- **reviews** — rating, comment, user への FK, burger への FK, photo_key (写真の保存キー。任意), 論理削除 (discarded_at)
- **burger_stats** — バーガーごとの、レビュー由来の集計値

```text
users    1 ──0..* reviews
burgers  1 ──0..* reviews
shops   *──────* burgers  (shops_burgers 経由)
```

## Frontend

ドメインのルール(検証・権限・計算)の判断は backend の domain だけが持つ。frontend は入力・説明・表示・サーバーのエラーの表示だけを行い、ルールを複製しない(規則違反は、サーバーの 422 メッセージを表示する)。

### 技術スタック

- React 19
- TypeScript
- Vite
- React Router
- SWR
- Jotai
- react-hook-form
- axios
- ESLint
- Vitest
- pnpm

### Frontend の構成

```text
frontend/src/
├── app/          # router、provider、アプリシェル
├── domains/      # auth、reviews、shops、users
├── api/          # API クライアント / HTTP 境界
├── states/       # グローバル state
├── lib/          # 共通ユーティリティ (date、i18n、rating)
└── components/   # 共通 UI コンポーネント
```

### API の接続先

- ベースパスは既定で `/api` (同一オリジン)。環境変数 `VITE_API_BASE_URL` で変更できる。
- 開発時は Vite の proxy が `/api` を Go API へ転送する。転送先の既定は `http://host.docker.internal:8080` で、`VITE_API_PROXY_TARGET` で変更できる。レビュー写真の `/photos` も同じ転送先へ proxy される(本番の nginx にも `/photos/` がある)。

### Frontend コマンド

以下は `frontend/` から実行します。

```bash
# frontend を起動
cd frontend
docker compose up --build

# Lint
cd frontend
pnpm run lint

# 型チェック
cd frontend
pnpm run type-check

# テスト
cd frontend
pnpm run test

# ビルド
cd frontend
pnpm run build
```

## API と認証

- Backend の API は snake_case を使う。
- Frontend のコードは camelCase を使う。
- casing の変換は HTTP 境界 (`frontend/src/api/client/buildApiClient.ts`) の責務とする。
- 認証は独自実装の JWT Bearer token を使う。
- 認証が必要なリクエストは `Authorization: Bearer <token>` を送信すること。

## 品質ゲート

Backend の変更では、通常は次を実行する:

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
cd backend-go && docker compose run --rm sqlc generate   # db/queries/ を変更したとき
```

Frontend の変更では、通常は次を実行する:

```bash
cd frontend
pnpm run lint
pnpm run type-check
pnpm run test
```

## Git と PR のワークフロー

- 変更の前後に `git status --short --branch` を確認する。
- commit は依頼された範囲に絞る。
- 無関係な未追跡ファイルを含めない。
- commit の前に `git diff --check` または `git diff --cached --check` を実行する。
- PR ブランチを push した後、`gh` が使える場合は `gh pr view` で PR を確認する。
