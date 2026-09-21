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
├── design/     # デザインツール Penpot のローカル環境と、書き出したデザイン(.penpot)
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

永続化は読み取りと書き込みで分け、repository は domain からだけ使う。usecase は読み取りに `*Query`(usecase が宣言。メソッド名は `Get*` / `List*` のみ)を使い、書き込みは domain の書き込みオブジェクト(集約ごとの `domain.Shops` / `Reviews` / `Users` / `SignupVerifications` / `MailDeliveries`)を通す。usecase は repository を宣言も保持も呼び出しもしない。repository の interface(`*Repository`。`Create*` / `Update*` / `Discard*` と、書き込みの前に行う排他ロックの `Lock*` だけの書き込み専用)は domain が宣言し、それを呼べるのは domain のコードだけである(Service という型に限らない)。Query に書き込みを、Repository に読み取りを置かない。adapter は読み取りを `internal/adapter/query`、書き込みを `internal/adapter/repository` が実装し、sqlc の行 → domain の写像のうち、query と repository の両方が使うものは `internal/adapter/rowmap` に 1 か所にまとめる(片側だけが使う写像はその側に置く)。書き込みの内部で必要な読み取り(バーガー名で探して、なければ作る処理の検索など)は repository の実装の内部に閉じる。レビューの保存とバーガーの統計の再計算、ユーザーの退会と統計の再計算のように、両方成功したときだけ確定し、途中で失敗したら両方取り消したい手順は、usecase が `UnitOfWork` の中で組み立てる。`UnitOfWork`(作業のひとまとまり)は、「ここからここまでの書き込みと読み取りを、まとめて 1 つのトランザクションにする」範囲を、usecase が指定するための仕組みで、`Do(ctx, fn)` で使う。`Do` は、トランザクションを開始して `fn` を実行し、`fn` がエラーなら全体を取り消し(rollback)、成功なら確定する(commit)。`fn` には `Tx`(同じトランザクションに結び付いた書き込みと読み取りの組)が渡る。書き込みは domain の書き込みオブジェクト(`Tx` の `Reviews` / `Users` / `BurgerStats`)、読み取りは usecase が宣言する `BurgerStatsQuery`(`Tx.Stats`。まだ確定していない書き込みも見える)である。実装は `internal/adapter/uow`(pgx のトランザクション。repository と query の両方を組み合わせるのは、このパッケージだけ)。統計の再計算は、usecase の `BurgerStatsRecalculator`(「再計算する役」。バーガーの行をロックする → 統計の元になるレビューを読む → domain の `CalculateBurgerStat` で計算する → 保存する、の順。現在時刻は usecase が宣言する `Clock` から受け取る)が行い、adapter は domain の計算を呼ばない。バーガーの行のロック(`Lock*` メソッド。他の処理が同じ行を同時に更新できないよう、いったん占有する)は、同じバーガーの統計を同時に計算し直す処理が互いの追加分を取りこぼすのを防ぐ。1 つのトランザクションで複数のバーガーを再計算するときは、バーガー ID の昇順に行う(別々の処理が逆の順序でロックして、互いを待ち合って止まる「デッドロック」を避けるため)。repository の中で複数の文をまとめる `withTx` は、`UnitOfWork` の中ではセーブポイント(トランザクションの途中に打つ、部分的な巻き戻し用の目印)になる。組み立て(`cmd/api/main.go`)は「repository → 書き込みオブジェクト → usecase」の順に行い、`UnitOfWork` と `BurgerStatsRecalculator` を、レビューとユーザーの usecase に渡す。1 つの集約だけを更新する書き込みは、Service を作らず、その集約の書き込みオブジェクトに置く(自分の集約の repository だけを持つ薄い層)。`*Service` は、複数の集約を跨ぐ更新のうち、途中で読み取りを挟まない書き込みだけの手順に使う(2 種類以上の repository を持つときだけ許す)。読み取りを挟む手順(レビューの書き込み → 統計の元になるレビューの読み取り → 統計の保存)は、repository だけを持つ Service では表現できないので、上の `UnitOfWork` の中で usecase が組み立てる。現時点では Service は 1 つもない。業務の判断はエンティティ・値オブジェクトに置き、公開メソッドは 1 つの型あたり 8 個までとする(ルールの全文は `backend-go/internal/domain/doc.go`。構造検査 `TestPersistenceInterfaceNaming` が、usecase・handler・query が repository に依存しないこと、書き込みオブジェクトが自分の repository だけを持つこと、単一の集約の Service を作らないこと、メソッド数の上限を強制する)。

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
- すべてのテーブルの ID(users・shops・burgers・reviews)は **UUID**(小文字・ハイフン区切りの正規形。DB の `gen_random_uuid()` が v4 で生成する)。URL・API・JWT(`user_id` claim)・frontend では、この文字列をそのまま扱う。正規形でない ID(大文字・ハイフンなし・整数)は、パスの `{id}`(`/users/{id}`・`/shops/{id}`・`/admin/shops/{id}`・`/reviews/{id}`)では存在しないものと同じ 404、クエリ(`user_id`・`shop_id`)と `POST /reviews` の本文(`shop_id`・`burger_id`。空は「指定なし」で、`shop_id` がなければ 404、`burger_id` がなければ `burger_name` の経路)では 422(`User id must be a valid UUID` など)にする(形式の判定は `domain.IsUUID` だけが持ち、frontend は判定しない)。一覧の並びは、id ではなく `created_at` や `name` と、同値の決着のための `id` で決める(UUID の id は作成順ではない)。レビューの一覧は `created_at` の降順、同時刻は `id` の降順で、同時刻のレビューが多数あっても、ページをまたいで重複・欠落がない。統計の再計算に使うレビューの順序も、`created_at` の昇順、同時刻は `id` の昇順である。
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
- `POST /signup/confirm` — 確認メールのリンクの平文トークン(`{"token":"…"}`)でアカウントを作成する。成功すると従来の signup と同じ 201 `{id, username, email, admin, can_moderate, token}`(`can_moderate` は login・`GET /me` と共通)を返し、そのままログイン状態にできる。期限切れ・存在しない・改ざん・使用済みのトークン(と、確認までの間に同じ email のユーザーが作られていた場合)は、区別できない同一の 400 `{"error":"Confirmation token is invalid or has expired"}`
- `POST /login` — 認証して JWT トークンを受け取る (email とパスワードは signup と同じ規則を `domain.ValidateCredentials` で判定し、満たさなければ照合の前に 422。規則を満たしたうえで誤っていれば 401 `Invalid email or password`)
- `GET /me` — Bearer トークンから解決した現在のユーザー(`id`・`username`・`email`・`admin`・`can_moderate`)。無効・期限切れのトークンは 401。frontend は、トークンの有効性を自分で判断せず、起動時にこの応答でログイン状態を復元する。`can_moderate`(moderation ができるか。domain の `User.CanModerate`)は、`POST /login`・`POST /signup` の応答にも含まれ、frontend は `admin` から権限を導かず、管理画面の出し分けをこの値で行う (要認証)
- `GET /meta` — frontend が描画・送信前の処理に使う、backend のルールの値(`{"rating": {"min": 1, "max": 5}, "photo": {"max_edge": 1600, "max_bytes": 5242880}}`)。認証不要で、`Cache-Control: public, max-age=3600`。ルールを持つのは backend だけ(rating の範囲は domain の `MinRating` / `MaxRating`、写真の上限は `photo.MaxEdge` と handler の `maxPhotoBytes`)で、frontend は定数を持たず、評価の選択肢・★の描画・絞り込み・写真の縮小にこの値を使う
- `POST /logout` — 確認メッセージを返すだけ。JWT は stateless なのでサーバー側での無効化はなく、token の破棄はクライアントが行う (要認証)

**ショップ**
- `GET /shops` — ショップ一覧 (`page` / `per_page` が整数でなければ 422。空・省略は既定値、範囲外の整数は補正される。次のページがあるかを、レスポンスヘッダー `X-Has-More: true|false` で返す。本文は従来どおりの配列で、1 ページの件数は backend が決め、frontend は件数から最終ページを推測しない)
- `GET /shops/:id` — ショップ 1 件の取得 (`can_review`: 閲覧者がこのショップにレビューを書けるか。domain の `CanBeReviewedBy` の結果で、匿名は `false`。frontend は「レビューを書く」ボタンをこの値で出し分ける)
- `POST /shops` — ショップの申請 (要認証)

**レビュー**
- `GET /reviews` — レビュー一覧 (省略可能な `user_id` クエリ(ユーザーの UUID。正規形でなければ 422 `User id must be a valid UUID`)で、そのユーザーの公開レビューだけに絞り込める。`page` / `per_page` と `X-Has-More` の扱いは `GET /shops` と同じ。各レビューに `can_edit` を含む)
- `GET /reviews/:id` — レビュー 1 件の取得 (`can_edit`: 閲覧者がそのレビューを編集・削除できるか。domain の `CanBeModifiedBy` の結果で、作者だけ `true`(admin も他人は `false`)、匿名は `false`。`POST` / `PUT` の応答にも含み、shop 詳細に埋め込まれるレビューには含まない。frontend は所有者を比較せず、この値で編集・削除ボタンを出し分ける)
- `POST /reviews` — レビューの投稿 (要認証)
- `PUT /reviews/:id` — レビューの更新 (要認証)
- `DELETE /reviews/:id` — レビューの削除 (要認証)

**写真**
- レビューに付ける写真(`POST /reviews`・`PUT /reviews/:id` の `photo` パート)は、**JPEG・PNG・WebP・HEIC**(HEIF に入った HEVC の静止画。iPhone の既定の形式)を受け付ける。形式は、ファイルの中身で判別する(申告された Content-Type は見ない)。保存は、JPEG は JPEG、PNG は PNG、WebP と HEIC は JPEG で、長辺を 1,600 px 以下に縮小して再エンコードする(拡大はしない)。向きは、保存する画素が正立するように直す(HEIC は、デコーダが回転の情報を適用するので、二重には回転しない)
- 上限(すべて backend で判定する): ファイルは 5 MiB、寸法は 1 辺 10,000 px かつ 2,400 万画素(**HEIC は 1,600 万画素**。HEIC のデコードは 1 画素あたり約 31 バイトのメモリを使い、1,600 万画素で約 490 MiB、2,400 万画素では約 760 MiB になって、コンテナの上限(1 GiB)に収まらないため)。寸法は、デコードの前に、ヘッダーから読んで確かめる。HEIC のデコードは同時に 1 件で、その間は JPEG などのデコードも止まる(メモリの実測にもとづく)。順番待ちとデコードそのものに、それぞれ 8 秒の時間制限がある
- HEIC のデコードの順番が 8 秒以内に回ってこないほど混み合っているときは、写真の問題ではないので、422 ではなく **503**(`Retry-After: 5`。`{"error":"photo processing is busy, please try again"}`)を返す(デコードそのものが 8 秒を超えるものは、処理しきれない写真として 422)
- 422 のメッセージは原因ごとに 3 種類: `Photo must be a JPEG, PNG, WebP, or HEIC image`(対応しない形式・壊れている・AVIF などの別の形式)、`Photo dimensions are too large (max 10000px per side and 24 megapixels, 16 megapixels for HEIC)`(寸法)、`Photo is too large (max 5MB)`(ファイルの大きさ)
- HEIC のデコーダは、libheif を WASM(隔離された実行系)にコンパイルして純 Go で動かすライブラリ(`github.com/gen2brain/heic`)で、cgo を使わない。既定では「OS に libheif があればそれを使い、なければ WASM」と切り替わるので、**常に WASM だけを使うよう、build タグ `nodynamic` を付ける**(Dockerfile の `ENV GOFLAGS=-tags=nodynamic` が、イメージの中の go build・go test に効く。ホストで直接ビルド・テストするときは `GOFLAGS=-tags=nodynamic go test ./...`)。デコーダの WASM は libheif・libde265(どちらも LGPL-3.0)を含む。このサービスはバイナリを配布しないので、LGPL の義務(配布時のライセンス表示・ソースの提示)は生じないが、バイナリやイメージを配布するときは、`LICENSE.libheif`・`LICENSE.libde265`(モジュールの `lib/` にある)を同梱する
- `GET /photos/*` — ディスクに保存されたレビュー写真を配信 (認証不要。末尾が `/` のディレクトリ path は一覧せず 404、末尾 `/` なしは 301 で `/` 付きへ転送されてから 404)。`PHOTO_STORAGE` が `disk` (既定) のときだけ登録され、`s3` では登録されない (写真の URL は bucket の公開ドメインを指す)

**ユーザー**
- `GET /users/:id` — ユーザーを 1 人取得 (認証は任意。存在しない・退会済み・UUID の正規形でない id は同一の 404。自己紹介文(応答のキーは `bio`。書かれていなければ空文字)は、誰が閲覧しても含む。email・admin は、本人が閲覧したときだけ含む。`can_edit`: 閲覧者がこのプロフィールを編集・削除できるか(domain の `Manages`。本人だけ `true`)を常に含む)
- `PUT /users/:id` — ユーザーの更新 (要認証。本人のみ。usecase で判定。email を変更するときは、signup と同じ形式の検証を行う。自己紹介文(`bio`)は、送ったときだけ更新される(送らなければ変わらず、空文字を送ると消える)。上限は 500 文字で、超えると 422 `Bio is too long (maximum is 500 characters)`。応答にも `bio` を含む。新規登録では自己紹介文を設定できない(`POST /signup` の要求に含めても無視される))
- `DELETE /users/:id` — ユーザーの削除 (要認証。本人のみ。usecase で判定)

**管理者** (要認証。管理者のみ許可する判定は usecase で行う)
- `GET /admin/shops` — モデレーション用のショップ一覧 (各ショップに `can_approve` / `can_reject`: 承認・却下の操作を画面が提示してよいか。domain の `Shop.CanBeApproved` / `CanBeRejected` が status から判断する。`PUT`・`approve`・`reject` の応答にも含まれる。frontend は status を比較してボタンを出さない)
- `PUT /admin/shops/:id` — ショップの更新
- `POST /admin/shops/:id/approve` — 申請されたショップの承認
- `POST /admin/shops/:id/reject` — 申請されたショップの却下

### 入力の上限

テキスト入力には文字数の上限がある。超えると 422 で、`{"errors": ["Comment is too long (maximum is 2000 characters)"]}` のように、他の違反と一緒に列挙される(検証は永続化の前で、失敗したら何も書かれない。JSON と multipart の両方の経路で同じ)。判定は domain だけが持ち(上限の定数は、ルールを持つ側の `internal/domain/` のファイルに、検証の関数と並べて置く: `review.go` のコメント・バーガー名、`shop.go` のショップ名・却下メモ、`username.go`、`email.go`、`bio.go` の自己紹介文)、frontend は判定を持たず、サーバーのメッセージを表示する。

| 項目 | 上限(文字) |
|---|---|
| レビューのコメント(`POST /reviews`、`PUT /reviews/:id`) | 2,000 |
| バーガー名(`burger_name` の経路) | 100 |
| ショップ名(`POST /shops`、`PUT /admin/shops/:id`) | 100 |
| ユーザー名(`POST /signup`、`PUT /users/:id`) | 50 |
| メールアドレス(`POST /signup`、`PUT /users/:id`) | 254 |
| 自己紹介文(`PUT /users/:id` の `bio`。送ったときだけ判定) | 500 |
| 管理者の却下メモ(`POST /admin/shops/:id/reject` の `moderation_note`) | 500 |

- 文字数は Unicode の**コードポイント数**で数える(バイト数でも書記素クラスタでもない。日本語は 1 文字、通常の絵文字も 1 文字。結合文字は 1 コードポイントごとに数える)。PostgreSQL の `char_length` と同じ数え方である
- `PUT` の部分更新は、送られた項目だけを検証する
- DB にも `CHECK (char_length(...) <= N)` がある(多層防御。最初のマイグレーションの `CREATE TABLE` に、名前つきの制約として入っている)。値は domain の定数と同じで、食い違いは `db/migrations_test.go` が検出する。上限を変えるときは、定数と、該当する `CREATE TABLE` の `CHECK` の 2 か所を直す(実運用に入ったあとは、新しいマイグレーションで直す)
- リクエスト body のバイト数の上限は Content-Type で決まる: 既定は 1 MiB。`POST /reviews` と `PUT /reviews/:id` の `multipart/form-data`(写真つき)だけが 6 MiB(写真は別に 5 MiB)。JSON は、レビューの書き込みでも 1 MiB を超えると 413。multipart のテキスト項目は 1 項目 64 KiB(外側のガード。超えると 400)

### データベーススキーマ

`backend-go/db/migrations/` のマイグレーションで定義された 8 つのテーブル:

- **users** — id (uuid), email, username, bio (自己紹介文。書かれていなければ空文字), password_digest, admin フラグ, 論理削除 (discarded_at)
- **shops** — id (uuid), name, モデレーション状態 (pending / active / rejected), moderation_note, 申請者への FK
- **burgers** — id (uuid), 中間テーブル経由でショップに紐づくバーガー
- **shops_burgers** *(中間テーブル)* — shop_id (FK, uuid), burger_id (FK, uuid)
- **reviews** — id (uuid), rating, comment, user への FK, burger への FK, photo_key (写真の保存キー。任意), 論理削除 (discarded_at)
- **burger_stats** — burger_id (uuid), バーガーごとの、レビュー由来の集計値
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
├── lib/          # 共通ユーティリティ (date、i18n、rating、photoResize)
└── components/   # 共通 UI コンポーネント
```

### API の接続先

- ベースパスは既定で `/api` (同一オリジン)。環境変数 `VITE_API_BASE_URL` で変更できる。
- 開発時は Vite の proxy が `/api` を Go API へ転送する。転送先の既定は `http://host.docker.internal:8080` で、`VITE_API_PROXY_TARGET` で変更できる。レビュー写真の `/photos` も同じ転送先へ proxy される(本番の nginx にも `/photos/` がある)。

### 写真の送信

- 写真を選ぶ input の `accept` は、HEIC / HEIF も含める(`lib/photoResize.ts` の `PHOTO_ACCEPT`)。選びやすくするためのヒントで、受け付けるかどうかは backend が決める。
- 送る前に、`useCreateReview` / `useUpdateReview` が `shrinkPhoto` を通す。長辺が上限(`GET /meta` の `photo.maxEdge`)を超える、またはファイルが `photo.maxBytes` を超えるときだけ、canvas で縮小した JPEG にする(向きは `createImageBitmap` の `imageOrientation: "from-image"` で画素に反映する)。**上限の値は frontend に書かない**(backend の値を使う)。
- 縮小できないとき(HEIC を読み込めないブラウザなど)は、失敗にせず、元のファイルをそのまま送る。backend が JPEG に変換するか、理由つきのメッセージ(422 の 3 種類、混み合いの 503)を返し、それを画面にそのまま出す。

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

## デザイン (`design/`)

デザインツール Penpot(オープンソース)を、自分の PC で動かすための環境です。アプリ(`backend-go/`・`frontend/`)とは独立していて、アプリの起動・テストには影響しません。使い方の詳細は `design/README.md` を参照してください。

```bash
# 初回だけ: design/.env を作り、鍵を追記する(design/.env はコミットしない。中身を読まない・出力しない)
cp design/.env.example design/.env
printf 'PENPOT_SECRET_KEY=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" >> design/.env

# 起動(http://localhost:9001)/ 停止 / データごと削除
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env up -d
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env down
docker compose -p hamburger-penpot -f design/docker-compose.yml --env-file design/.env down -v
```

- 書き出したデザイン(`.penpot`)は `design/files/` に置いて git で保存する(バイナリなので差分は読めない)。
- ポートは 9001(既存の 8080・5173・5433 と重ならない)。Penpot は複数のコンテナで数 GiB のメモリを使うので、使わないときは `down` する。
- `design/.env` は秘密(Penpot の鍵)を含む。エージェントは読まない(`.claude/settings.json` の deny 対象)。

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
