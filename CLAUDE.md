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

永続化は読み取りと書き込みで分け、repository は domain からだけ使う。usecase は読み取りに `*Query`(usecase が宣言。メソッド名は `Get*` / `List*` のみ)を使い、書き込みは domain の書き込みオブジェクト(集約ごとの `domain.Shops` / `Reviews` / `Users` / `SignupVerifications`)を通す。usecase は repository を宣言も保持も呼び出しもしない。repository の interface(`*Repository`。`Create*` / `Update*` / `Discard*` のみの書き込み専用)は domain が宣言し、それを呼べるのは domain のコードだけである(Service という型に限らない)。Query に書き込みを、Repository に読み取りを置かない。adapter は読み取りを `internal/adapter/query`、書き込みを `internal/adapter/repository` が実装し、sqlc の行 → domain の写像のうち、query と repository の両方が使うものは `internal/adapter/rowmap` に 1 か所にまとめる(片側だけが使う写像はその側に置く)。書き込みの内部で必要な読み取り(トランザクション内のロック取得など)は repository の実装の内部に閉じる。組み立て(`cmd/api/main.go`)は「repository → 書き込みオブジェクト → usecase」の順に行う。1 つの集約だけを更新する書き込みは、Service を作らず、その集約の書き込みオブジェクトに置く(自分の集約の repository だけを持つ薄い層)。`*Service` は、複数の集約を跨ぐ更新だけに使う(2 種類以上の repository を持つときだけ許す。現時点では 1 つもない)。業務の判断はエンティティ・値オブジェクトに置き、公開メソッドは 1 つの型あたり 8 個までとする(ルールの全文は `backend-go/internal/domain/doc.go`。構造検査 `TestPersistenceInterfaceNaming` が、usecase・handler・query が repository に依存しないこと、書き込みオブジェクトが自分の repository だけを持つこと、単一の集約の Service を作らないこと、メソッド数の上限を強制する)。

```text
backend-go/
├── cmd/api/main.go     # composition root: 設定、DB プール、配線、サーバ
├── internal/
│   ├── domain/         # エンティティ、値オブジェクト、ドメインエラー、書き込みの契約(*Repository の interface と、それを持つ書き込みオブジェクト。複数の集約を跨ぐ更新だけ *Service)。集約ごとに 1 ファイルにまとめ、エンティティが repository を参照しないことは構造検査で守る。標準ライブラリのみ
│   ├── usecase/        # アプリケーションのユースケース + 読み取りの *Query(利用側で宣言)。repository には依存しない
│   └── adapter/
│       ├── handler/    # net/http のハンドラ、DTO、ルーティング、middleware
│       ├── query/      # usecase の *Query(読み取り専用)を sqlc で実装
│       ├── repository/ # domain の *Repository(書き込み専用)を sqlc で実装
│       │   └── sqlcgen/  # sqlc の生成コード。手で編集しない
│       ├── rowmap/     # query と repository が共有する、sqlc の行 → domain の写像
│       └── infra/      # DB プール、JWT、パスワードハッシュ、設定
├── db/
│   ├── migrations/     # SQL マイグレーション
│   └── queries/        # sqlc のクエリ(*.sql)
└── sqlc.yaml
```

### Backend 設計ルール

- `domain` は標準ライブラリのみを import する。`net/http`・`database/sql`・`pgx`・`usecase`・`adapter` は import しない。
- 読み取りの `*Query` は `usecase` 側で宣言し、`adapter/query` が実装する。書き込みの `*Repository` は `domain` が宣言し、`adapter/repository` が実装する。repository を呼べるのは `domain` のコードだけ(単一の集約は書き込みオブジェクト、複数の集約を跨ぐ更新だけ Service)で、`usecase` は repository に依存しない。
- sqlc の行構造体や `pgx` の型を `adapter/` の外に出さない。ドメインの形と DB の形は別々に設計する。
- `sqlcgen/` は手で編集しない。`db/queries/` を変更して再生成する。
- API の JSON は snake_case を使う。
- 詳細は `.agents/skills/backend-go-boundaries` と `.agents/skills/db-design` を参照する。

### Backend の前提

- **:8080** で待ち受け、ヘルスチェックは `GET /up`。
- 専用の Postgres を使う (ホストのポートは 5433)。
- ユーザーの ID は **UUID**(小文字・ハイフン区切りの正規形。DB の `gen_random_uuid()` が v4 で生成する)。URL・API・JWT(`user_id` claim)・frontend では、この文字列をそのまま扱う。正規形でない ID(大文字・ハイフンなし・整数)は、パスの `{id}` では存在しないものと同じ 404、`user_id` クエリでは 422 にする(形式の判定は `domain.IsUUID` だけが持ち、frontend は判定しない)。shop・burger・review の ID は、当面は連番の bigint(S29・S30 で UUID にする)。
- 認証は **JWT**。ログイン時にトークンを返し、以降は `Authorization: Bearer <token>` で送る。`user_id` が数値の旧形式のトークンは無効(401)。`JWT_SECRET` が未設定だと起動時にエラーで落ちる(fail-loud)ため、`docker compose up` の前に export する。
- signup は**メール確認つき**で、確認メールの送信設定(`SMTP_HOST`・`SMTP_PORT`・`MAIL_FROM`・`APP_BASE_URL`)が欠けていると起動時に落ちる。開発では compose の Mailpit が受け取る(`docker compose up` で足りる。Web UI は http://localhost:8025)。詳細は「signup の確認メール」。

### signup の確認メール

`POST /signup` は、入力を検証したあと、**登録の有無にかかわらず同じ応答(202)**を返す。応答の違いから、第三者が「この email は登録済みか」を判別できないようにするためである。

- **未登録の email**: 確認待ちを `signup_verifications` に保存し(同じ email の確認待ちは最新の入力で置き換える)、確認リンクつきのメールを送る。リンクは `<APP_BASE_URL>/signup/confirm?token=<平文のトークン>`。トークンは 32 バイトの暗号乱数(base64url)で、DB には SHA-256 だけを保存する。有効期間は 24 時間(`domain.SignupTokenTTL`。業務のルールで、環境変数では変えない)で、単回使用。パスワードは bcrypt で保存し、平文は保存しない。
- **登録済みの email**: 状態を変えず、「すでに登録済み」の通知メールを送る(確認リンク・トークンは含めない。ログイン画面へのリンクだけ)。
- どの分岐でも bcrypt のハッシュ計算を行ってから分岐し、メール送信は非同期(有界のキュー+少数の worker)なので、応答時間から登録の有無を推測されない。送信の失敗・遅延・キューの満杯・記録の失敗は signup の応答に影響しない(失敗はログに出す)。同じ email への確認メールは 60 秒に 1 通までで、間隔内の再 signup は 202 を返すが、確認待ちを変えず、メールも送らない。
- 確認(`POST /signup/confirm`)は、1 つの transaction で「確認待ちをロック → users を作成 → 確認待ちを削除」を行う。同じトークンでの並行する確認は、1 件だけが成功する。期限切れの確認待ちは、signup のたびに上限つきで日和見的に削除する。
- メール本文はプレーンテキスト・英語で、利用者が入力した値(username など)を入れない。
- **送信の記録と冪等**: 送るメールは `mail_deliveries` に記録する(送信の履歴と冪等キーだけ。本文・確認トークン・パスワードは保存しない)。worker が、送る前に冪等キーで `pending` の行を作り(`INSERT … ON CONFLICT DO NOTHING`。作れたときだけ送る)、結果を `sent` / `failed`(試行の回数、失敗の種類 `temporary` / `permanent`、200 文字に切った理由)で更新する。冪等キーは、確認メールなら「確認待ちの id + 送信の世代(再 signup で置き換えるたびに増える)」、登録済みへの通知なら「email(小文字)+ 60 秒の窓」で、同じ要求は何度来てもメールが 1 通だけ出る。窓は固定なので、窓をまたぐ 2 つの要求は続けて 2 通出ることがある(どの 60 秒の間でも最大 2 通)。60 秒の制限は DB の判定で、複数のインスタンスで共有される。再送(リトライ)はしない。記録できなかったときは、冪等を守るために送らない。
- **腐敗防止層**: usecase は、ドメインの意図(`usecase.Mailer` の `SendSignupConfirmation` / `SendAlreadyRegistered`。宛先・リンクの URL・有効期間・冪等キー)だけを渡す。件名・本文・書式は `internal/adapter/infra` が組み立て、SMTP のプロトコル・MIME・応答コードも `infra` が受け止めて、domain の言葉(一時的な失敗・恒久的な失敗)に翻訳して記録する。usecase と domain が `net/smtp` などを import・参照しないことは、構造検査で固定している。

**環境変数**(必須が欠けている・不正なときは起動時に落ちる。値はログ・エラーに出さない)

| 変数 | 必須 | 内容 |
|---|---|---|
| `SMTP_HOST` / `SMTP_PORT` | 必須 | 送信に使う SMTP サーバー。`docker compose` の開発環境は Mailpit(`mailpit:1025`) |
| `SMTP_SECURITY` | 任意 | `starttls`(既定。587)/ `tls`(暗黙の TLS。465)/ `none`(開発用。認証情報を設定したまま `none` にすると起動時に落ちる) |
| `SMTP_USER` / `SMTP_PASSWORD` | 任意 | 認証(2 つ揃えて設定。暗号化した接続でしか送らない)。**秘密。ログ・コード・PR に書かない** |
| `MAIL_FROM` | 必須 | 送信元(本番は検証済みのドメインのアドレス) |
| `APP_BASE_URL` | 必須 | 確認リンクの生成元(frontend の URL。例: `http://localhost:5173`) |

**開発**: `docker compose up` で Mailpit が起動する。確認メールは http://localhost:8025 で読める(SMTP は compose のネットワーク内の `mailpit:1025` で、ホストには公開しない)。

**本番(Resend)**: 実装は汎用の SMTP で、プロバイダー固有のコードはない。Resend は次の設定でそのまま使える。API キーの値はここに書かない(環境変数・シークレットで渡す)。

```text
SMTP_HOST=smtp.resend.com
SMTP_PORT=465            # 暗黙の TLS。587 なら SMTP_SECURITY=starttls
SMTP_SECURITY=tls
SMTP_USER=resend
SMTP_PASSWORD=<Resend で発行した API キー>
MAIL_FROM=noreply@<Resend で検証した送信ドメイン>
APP_BASE_URL=https://<frontend の URL>
```

本番稼働の前に、**Resend のダッシュボードで送信ドメインを検証(SPF / DKIM)し、API キーを発行する**(運用の作業)。Amazon SES や Mailgun など別のプロバイダーも、ホスト・ポート・認証の設定だけで切り替えられる。

**既知の残課題・残リスク**
- `PUT /users/:id` で email を変更するときは、確認メールを挟まず、使用済みの email に 422 `Email has already been taken` を返し続ける。**認証済みのユーザーからは、email の登録有無を判別できる**(メール変更の確認は後続の story)。
- **pre-hijacking**: 攻撃者が被害者の email で先に signup し、被害者が身に覚えのない確認メールのリンクを開くと、攻撃者のパスワードのアカウントができる。緩和として、確認メールに「心当たりがなければ無視」と明記し、同じ email への signup は最新の入力で置き換え、有効期間は 24 時間、間隔内の再 signup では確認待ちを変えない。根本対策(リンク先でパスワードを設定する方式)は、UI の変更が大きいため採用していない。
- メールの大量送信の悪用は、同じ宛先・同じ種類を 60 秒の窓ごとに 1 通に絞って緩和している(確認メールは確認待ちの間隔、通知メールは `mail_deliveries` の冪等キー。どちらも DB の判定で、複数のインスタンスで共有される。窓は固定なので、窓をまたぐと続けて 2 通出ることがある)。IP 単位の制限はない。`alice+1@…` のような別名は別の宛先として数えるので、同じ受信箱への送信は抑えられない。
- 応答時間: メール送信は非同期で、bcrypt は全分岐で行う。DB 操作の差(ミリ秒)は残る。
- 退会(soft delete)したユーザーの email は、`users.email` の一意制約が残るため、再登録できない(signup は 202 になるが、確認で 400 になる)。
- email の大文字小文字: 確認待ちは大文字小文字を区別せず一意だが、`users.email` は入力どおり保存され、登録済みの判定(`GetActiveUserByEmail`)と login は完全一致である(従来どおり)。

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
- `POST /signup` — アカウントの作成を申し込み、確認メールを送る。**登録済みの email でも未登録の email でも、同じ 202 `{"message":"Confirmation email sent"}` を返す**(アカウント列挙の防止)。アカウントは、確認メールのリンクを開いて `POST /signup/confirm` を呼んで初めて作られる。検証は登録の有無に依存しないものだけで、違反は 422(username、email、password。email は形式(`net/mail` で解析でき、表示名などを含まないアドレスだけであること)を検証し、不正なら 422 `Email is invalid`。password は 8〜72 バイトで、半角英字・数字・記号をそれぞれ 1 文字以上含む。`PUT /users/:id` のパスワード変更にも同じ規則を適用する。password_confirmation は任意で、送った場合は password と不一致なら 422。規則の判定は backend の domain だけが持ち、frontend は説明文の表示と、サーバーの 422 メッセージの表示だけを行う)。「登録済み」を示すエラーは返さない
- `POST /signup/confirm` — 確認メールのリンクの平文トークン(`{"token":"…"}`)でアカウントを作成する。成功すると従来の signup と同じ 201 `{id, username, email, admin, token}` を返し、そのままログイン状態にできる。期限切れ・存在しない・改ざん・使用済みのトークン(と、確認までの間に同じ email のユーザーが作られていた場合)は、区別できない同一の 400 `{"error":"Confirmation token is invalid or has expired"}`
- `POST /login` — 認証して JWT トークンを受け取る (email とパスワードは signup と同じ規則を `domain.ValidateCredentials` で判定し、満たさなければ照合の前に 422。規則を満たしたうえで誤っていれば 401 `Invalid email or password`)
- `POST /logout` — 確認メッセージを返すだけ。JWT は stateless なのでサーバー側での無効化はなく、token の破棄はクライアントが行う (要認証)

**ショップ**
- `GET /shops` — ショップ一覧 (`page` / `per_page` が整数でなければ 422。空・省略は既定値、範囲外の整数は補正される)
- `GET /shops/:id` — ショップ 1 件の取得
- `POST /shops` — ショップの申請 (要認証)

**レビュー**
- `GET /reviews` — レビュー一覧 (省略可能な `user_id` クエリ(ユーザーの UUID。正規形でなければ 422 `User id must be a valid UUID`)で、そのユーザーの公開レビューだけに絞り込める。`page` / `per_page` の扱いは `GET /shops` と同じ)
- `GET /reviews/:id` — レビュー 1 件の取得
- `POST /reviews` — レビューの投稿 (要認証)
- `PUT /reviews/:id` — レビューの更新 (要認証)
- `DELETE /reviews/:id` — レビューの削除 (要認証)

**写真**
- `GET /photos/*` — ディスクに保存されたレビュー写真を配信 (認証不要。末尾が `/` のディレクトリ path は一覧せず 404、末尾 `/` なしは 301 で `/` 付きへ転送されてから 404)。`PHOTO_STORAGE` が `disk` (既定) のときだけ登録され、`s3` では登録されない (写真の URL は bucket の公開ドメインを指す)

**ユーザー**
- `GET /users/:id` — ユーザーを 1 人取得 (認証は任意。存在しない・退会済み・UUID の正規形でない id は同一の 404。本人が閲覧したときだけ email・admin を含む)
- `PUT /users/:id` — ユーザーの更新 (要認証。本人のみ。usecase で判定。email を変更するときは、signup と同じ形式の検証を行う)
- `DELETE /users/:id` — ユーザーの削除 (要認証。本人のみ。usecase で判定)

**管理者** (要認証。管理者のみ許可する判定は usecase で行う)
- `GET /admin/shops` — モデレーション用のショップ一覧
- `PUT /admin/shops/:id` — ショップの更新
- `POST /admin/shops/:id/approve` — 申請されたショップの承認
- `POST /admin/shops/:id/reject` — 申請されたショップの却下

### データベーススキーマ

`backend-go/db/migrations/` のマイグレーションで定義された 8 つのテーブル:

- **users** — id (uuid), email, username, password_digest, admin フラグ, 論理削除 (discarded_at)
- **shops** — name, モデレーション状態 (pending / active / rejected), moderation_note, 申請者への FK
- **burgers** — 中間テーブル経由でショップに紐づくバーガー
- **shops_burgers** *(中間テーブル)* — shop_id (FK), burger_id (FK)
- **reviews** — rating, comment, user への FK, burger への FK, photo_key (写真の保存キー。任意), 論理削除 (discarded_at)
- **burger_stats** — バーガーごとの、レビュー由来の集計値
- **signup_verifications** — メール確認を待っている signup(uuid の主キー。email は入力どおり保存し、大文字小文字を区別せず一意。username、bcrypt 済みの password_digest、確認トークンの SHA-256(token_hash)、expires_at、last_sent_at、generation(確認メールを出した回数。冪等キーに使う))。users とは独立で、外部キーを持たない
- **mail_deliveries** — メール送信の記録(uuid の主キー。kind、recipient、idempotency_key(一意)、status(pending / sent / failed)、failure_kind、attempts、last_error、sent_at)。本文・確認トークン・パスワードは保存しない

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
