# CLAUDE.md

このファイルは、このリポジトリで作業する Claude Code などのコーディングエージェント向けのガイダンスを提供します。

## プロジェクト概要

BurgerStack は、ハンバーガーのレビューと評価を行う Web アプリケーションです。

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

- Go 1.27 (標準 `net/http` のルーティングを使う。Web フレームワークも ORM も使わない)
- PostgreSQL 16 (pgx)
- sqlc (SQL からの型安全なコード生成)
- JWT 認証
- MCP(リモートのサーバー。公式の `github.com/modelcontextprotocol/go-sdk`。Go 1.25 以上が必要)

### アーキテクチャ

クリーンアーキテクチャ (handler → usecase → domain) を採用しています。依存は内側にのみ向き、認可の判断は handler ではなく usecase / domain に置きます。

永続化は読み取りと書き込みで分け、repository は domain からだけ使う。usecase は読み取りに `*Query`(usecase が宣言。メソッド名は `Get*` / `List*` のみ)を使い、書き込みは domain の書き込みオブジェクト(集約ごとの `domain.Shops` / `Reviews` / `Users` / `SignupVerifications` / `MailDeliveries`)を通す。usecase は repository を宣言も保持も呼び出しもしない。repository の interface(`*Repository`。`Create*` / `Update*` / `Discard*` と、書き込みの前に行う排他ロックの `Lock*` だけの書き込み専用)は domain が宣言し、それを呼べるのは domain のコードだけである(Service という型に限らない)。Query に書き込みを、Repository に読み取りを置かない。adapter は読み取りを `internal/adapter/query`、書き込みを `internal/adapter/repository` が実装し、sqlc の行 → domain の写像のうち、query と repository の両方が使うものは `internal/adapter/rowmap` に 1 か所にまとめる(片側だけが使う写像はその側に置く)。書き込みの内部で必要な読み取り(バーガー名で探して、なければ作る処理の検索など)は repository の実装の内部に閉じる。レビューの保存とバーガーの統計の再計算の依頼の登録、ユーザーの退会と再計算の依頼の登録のように、両方成功したときだけ確定し、途中で失敗したら両方取り消したい手順は、usecase が `UnitOfWork` の中で組み立てる。`UnitOfWork`(作業のひとまとまり)は、「ここからここまでの書き込みと読み取りを、まとめて 1 つのトランザクションにする」範囲を、usecase が指定するための仕組みで、`Do(ctx, fn)` で使う。`Do` は、トランザクションを開始して `fn` を実行し、`fn` がエラーなら全体を取り消し(rollback)、成功なら確定する(commit)。`fn` には `Tx`(同じトランザクションに結び付いた書き込みと読み取りの組)が渡る。書き込みは domain の書き込みオブジェクト(`Tx` の `Reviews` / `Users` / `BurgerStats`)、読み取りは usecase が宣言する `BurgerStatsQuery`(`Tx.Stats`。まだ確定していない書き込みも見える)である。実装は `internal/adapter/uow`(pgx のトランザクション。repository と query の両方を組み合わせるのは、このパッケージだけ)。統計の再計算は、書き込みの応答のあとに、バックグラウンドのワーカーが行う(「統計の再計算(非同期)」)。書き込みは、同じ `UnitOfWork` の中で、再計算の依頼を登録するだけである(usecase の `BurgerStatsRecalculator`(「再計算する役」)の `RequestRecalculation`)。計算そのもの(バーガーの行をロックする → 統計の元になるレビューを読む → domain の `CalculateBurgerStat` で計算する → 保存する、の順)も同じ `BurgerStatsRecalculator` の `Recalculate` で、現在時刻は usecase が宣言する `Clock` から受け取る。adapter は domain の計算を呼ばない。バーガーの行のロック(`Lock*` メソッド。`FOR NO KEY UPDATE` で、他の処理が同じ行を同時に更新できないよう、いったん占有する)は、同じバーガーの統計を同時に計算し直す処理(複数のインスタンスのワーカーなど)が互いの追加分を取りこぼすのを防ぐ。レビューの書き込みは待たせない。1 つのトランザクションで複数のバーガーを扱う(退会での依頼の登録など)ときは、バーガー ID の昇順に行う(別々の処理が逆の順序でロックして、互いを待ち合って止まる「デッドロック」を避けるため)。repository の中で複数の文をまとめる `withTx` は、`UnitOfWork` の中ではセーブポイント(トランザクションの途中に打つ、部分的な巻き戻し用の目印)になる。組み立て(`cmd/api/main.go`)は「repository → 書き込みオブジェクト → usecase」の順に行い、`UnitOfWork` と `BurgerStatsRecalculator` を、レビューとユーザーの usecase に渡し、統計のワーカー(`StatsWorker`)も同じ部品で組み立てて起動する。1 つの集約だけを更新する書き込みは、Service を作らず、その集約の書き込みオブジェクトに置く(自分の集約の repository だけを持つ薄い層)。`*Service` は、複数の集約を跨ぐ更新のうち、途中で読み取りを挟まない書き込みだけの手順に使う(2 種類以上の repository を持つときだけ許す)。読み取りを挟む手順(レビューの書き込み → 統計の元になるレビューの読み取り → 統計の保存)は、repository だけを持つ Service では表現できないので、上の `UnitOfWork` の中で usecase が組み立てる。現時点では Service は 1 つもない。業務の判断はエンティティ・値オブジェクトに置き、公開メソッドは 1 つの型あたり 8 個までとする(ルールの全文は `backend-go/internal/domain/doc.go`。構造検査 `TestPersistenceInterfaceNaming` が、usecase・handler・query が repository に依存しないこと、書き込みオブジェクトが自分の repository だけを持つこと、単一の集約の Service を作らないこと、メソッド数の上限を強制する)。

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
- **エラーの文言の言語**: リクエストの `Accept-Language` で、日本語(`ja`)か英語(`en`)を選ぶ(q 値・`ja-JP`・`*` に従う。どちらでもない・ヘッダーなしは英語 = いままでの文言の契約)。言語を選ぶのは handler(`langOf`)だけで、文言は「キー + 引数」で表し、言語ごとの書式のカタログ(業務の規則の文言は `domain/messages.go` の `domain.Message`、handler が返す文言は `adapter/handler/messages.go` の `apiMessage`。型を分けている。英語・日本語の両方が必須で、値の並びも同じ。構造テスト(`internal/testutil/msgcheck`)が強制する)で文字列にする。本文の作り方は、`writeError`・`writeErrorList`・`writeErrorListWith`・`writeValidation`(言語に従う。`Vary: Accept-Language` も付く)と、`writeInternalError`(5xx。英語の固定)だけで、文字列を直接書いて本文を作ることは、構造テストが禁じる(言語の切り替えをすり抜けるため。MCP のツールの失敗も `t.failMessage`)。**日本語になるのは、利用者に見える 4xx すべて**: 検証の失敗の 422、見つからない・権限・認証・リクエストの形・写真・クエリの誤り、Google のサインインの案内(交換の応答)、OAuth の許可の画面の 422(要求が不正な理由の診断の文は英語のまま、日本語の説明に添える)、`/mcp` の 401・403 と、MCP のツールの失敗(結果が大きすぎるときの案内だけは、ツールの説明と同じく日本語で固定)。**対応が済んでも英語のまま**: OAuth の RFC のプロトコルの識別子(`error` の値・`error_description`・`WWW-Authenticate`)、5xx の `internal server error`・`database unavailable`、成功の応答(`Confirmation email sent`)、写真の配信(`GET /photos/…`)の 404(本文が text/plain で、API のエラーではない)。
- signup は**メール確認つき**で、確認メールの送信設定(`SMTP_HOST`・`SMTP_PORT`・`MAIL_FROM`・`APP_BASE_URL`)が欠けていると起動時に落ちる。開発では compose の Mailpit が受け取る(`docker compose up` で足りる。Web UI は http://localhost:8025)。詳細は「signup の確認メール」。

### 統計の再計算(非同期)

バーガーの統計(件数・平均・加重スコア)は、書き込みの応答のあとに、バックグラウンドのワーカーが計算し直す。書き込み(レビューの投稿・編集・削除、退会)は、統計を計算せず、**再計算の依頼**(`burger_stats_recalc_requests`。バーガーごとに 1 行。`000010_create_burger_stats_recalc_requests`)を、書き込みと同じトランザクションで登録するだけである。応答は統計の計算を待たず、同じバーガーへの書き込みが続いても、依頼は 1 行にまとまって、再計算は 1 回で済む。

- **結果整合**: 統計は、書き込みの数秒あと(ワーカーの間隔 1 秒 + 計算)に表示へ反映される。書き込み直後の応答や一覧では、古い値のことがある。frontend は、この遅れを許して表示する(値を推測して書き換えない)。
- **ワーカー**: 1 つの goroutine(`infra.StatsWorkerLoop`)が、起動の直後と、以降 `STATS_WORKER_INTERVAL` ごとに、`usecase.StatsWorker` の `RunOnce`(1 サイクル。テストはこれを直接呼ぶ)を実行する。`RunOnce` は、時期の来た依頼を最大 `STATS_WORKER_BATCH` 件取り出し、バーガーごとに別の `UnitOfWork` で「バーガーの行をロック → 統計を計算して保存 → 依頼を消す」を行う。プロセスが止まっていた間に溜まった依頼も、起動直後の 1 サイクルで処理される(依頼は DB にある)。
- **比較つき削除**: 依頼には `version`(専用の sequence `burger_stats_recalc_requests_version_seq` から取る番号)があり、登録のたびに進む。再計算を終えた依頼を消すのは、取り出したときの `version` と一致するときだけで、再計算の最中に入った新しい書き込みの依頼を消さない(残った依頼は、次のサイクルで最新の状態になる)。`version` は、行を消して作り直しても戻らない(戻ると、古い再計算が、新しい依頼を同じ番号だと思って消す)。
- **失敗**: 1 つのバーガーの失敗は、ほかのバーガーを止めない。失敗は、依頼に記録し(`attempts`・`next_attempt_at`・`last_error`(500 文字まで))、待ち時間を 2 秒から倍にして(上限 5 分)再試行する。`STATS_WORKER_MAX_ATTEMPTS` 回で打ち切り(error のログ `burger stats recalculation gave up`)、行は残るが取り出されない。そのバーガーに新しい書き込みがあれば、最初からやり直す。
- **複数のインスタンス**: 同じバーガーの再計算は、バーガーの行のロックで直列になる。複数のインスタンスのワーカーが同じ依頼を取り出しても、片方が待つだけで、統計は壊れない。
- **停止の順**: HTTP サーバー → 統計のワーカー(処理中のバッチを終えるまで待ち、`shutdownTimeout` を超えたら取り消す。取り消された依頼は残る) → メール送信 → DB プール。

**環境変数**(不正な値(数値でない・0 以下)は、起動を失敗させず、警告のログを出して既定の値になる)

| 変数 | 既定 | 内容 |
|---|---|---|
| `STATS_WORKER_INTERVAL` | `1s` | ワーカーが依頼を取りに行く間隔(Go の duration) |
| `STATS_WORKER_BATCH` | `20` | 1 サイクルで取り出す依頼の上限の件数 |
| `STATS_WORKER_MAX_ATTEMPTS` | `8` | 1 つの依頼を、失敗しながら再試行する上限の回数 |

### signup の確認メール

`POST /signup` は、入力を検証したあと、**登録の有無にかかわらず同じ応答(202)**を返す。応答の違いから、第三者が「この email は登録済みか」を判別できないようにするためである。

- **未登録の email**: 確認待ちを `signup_verifications` に保存し(同じ email の確認待ちは最新の入力で置き換える)、確認リンクつきのメールを送る。リンクは `<APP_BASE_URL>/signup/confirm?token=<平文のトークン>`。トークンは 32 バイトの暗号乱数(base64url)で、DB には SHA-256 だけを保存する。有効期間は 24 時間(`domain.SignupTokenTTL`。業務のルールで、環境変数では変えない)で、単回使用。パスワードは bcrypt で保存し、平文は保存しない。
- **登録済みの email**: 状態を変えず、「すでに登録済み」の通知メールを送る(確認リンク・トークンは含めない。ログイン画面へのリンクだけ)。
- どの分岐でも bcrypt のハッシュ計算を行ってから分岐し、メール送信は非同期(有界のキュー+少数の worker)なので、応答時間から登録の有無を推測されない。送信の失敗・遅延・キューの満杯・記録の失敗は signup の応答に影響しない(失敗はログに出す)。同じ email への確認メールは 60 秒に 1 通までで、間隔内の再 signup は 202 を返すが、確認待ちを変えず、メールも送らない。
- 確認(`POST /signup/confirm`)は、usecase が UnitOfWork(ここからここまでの書き込みと読み取りを、まとめて 1 つのトランザクションにする範囲を指定する仕組み。途中でエラーになれば全体を取り消す)の中で、「確認待ちをロック → 確認待ちを読む → users を作成 → 確認待ちを削除」の順に行う。途中で失敗したら全体を取り消すので、ユーザーだけが作られる、確認待ちだけが消える、が起きない(ユーザーの作成に失敗したら、確認待ちは残る)。先頭のロックで、同じトークンでの並行する確認は 1 件ずつに直列になり、1 件だけが成功する。期限切れの確認待ちは、signup のたびに上限つきで日和見的に削除する。
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

### OAuth の認可サーバー

AI アプリ(MCP のクライアントなど)が、利用者のログインと許可だけでこのアプリにつなぐための、OAuth 2.1 の認可サーバー(`internal/adapter/oauthserver`。認可ライブラリは `github.com/ory/fosite`)。`OAUTH_ISSUER` を設定したときだけ有効で、設定しなければ、下の窓口は登録されない(404)。リモートの MCP(`/mcp`。下の「リモートの MCP サーバー」)は、ここが発行したトークンで認証する。許可を尋ねる画面と、許可したアプリの一覧・取り消しは、frontend にある。

**流れ**(認可コード + PKCE)

1. アプリが、利用者のブラウザで `GET /oauth/authorize?...`(`client_id`・`redirect_uri`・`scope`・`state`・`code_challenge`(S256)・`resource`)を開く。API は要求を検証し、問題がなければ、許可を尋ねる画面(frontend。`OAUTH_CONSENT_URL`)へ、同じ値のまま 303 で渡す。アプリへ結果を戻せない不正(未登録のアプリ・登録と違う戻り先)は、リダイレクトせずにエラーで返し、戻せる不正(PKCE がない・知らない範囲・宛先の誤り)は、`error` を付けてアプリへ戻す。
2. 許可を尋ねる画面(frontend の `/oauth/authorize`)は、利用者のログイン(JWT)を確かめ(未ログインならログイン画面へ送り、ログイン後に戻す)、`GET /oauth/authorize/request?<同じ値>` で、アプリの名前・求められた範囲と説明・尋ねる必要があるか(`consent_required`)を受け取る。すでに許可済みの範囲に収まれば、尋ねずにそのまま許可を送る。利用者が選ぶと、`POST /oauth/authorize/decision`(`{"query":"<認可の URL の ? のあと>","approve":true|false}`)を送る。API は要求を検証し直し、許可なら許可の記録を残して認可コードを発行し、`redirect_to`(アプリへの戻り先。`code`・`state`・`iss`(発行者。RFC 9207)付き。拒否なら `error=access_denied` 付き)を返す。画面はそこへブラウザを移す。cookie のセッションは使わず、画面の API は、いつものログインの JWT(Bearer)で守る(OAuth のアクセストークンは受け付けない)。
3. アプリが `POST /oauth/token` で、認可コードと PKCE の `code_verifier` を、アクセストークン(と更新トークン)に交換する。切れたら、更新トークンで取り直す。
4. 保護する側(`/mcp` など)は、`usecase.OAuthAccessTokens.Authenticate` で、`Authorization: Bearer` のトークンを確かめる(宛先・範囲・持ち主のユーザーの有効性)。

**窓口**

- `GET /.well-known/oauth-authorization-server` — 認可サーバーの情報(RFC 8414)。対応するのは、`code` と `refresh_token`、PKCE は `S256` だけ、アプリの認証は `none`(秘密の鍵を持たない公開クライアントだけ)、範囲は `hamburger:read` / `hamburger:write`。
- `GET /oauth/authorize` — 認可の入口(上記 1)。
- `POST /oauth/token` — トークンの発行(認可コードの交換・更新)。
- `POST /oauth/revoke` — 取り消し(RFC 7009)。更新トークンを取り消すと、その認可から発行されたトークンがすべて使えなくなる。
- `GET /oauth/authorize/request`・`POST /oauth/authorize/decision` — 許可の画面が使う API(要ログイン。上記 2)。要求がアプリへ結果を戻せない形で不正なときは 422 `{"error":"…"}`。
- `GET /oauth/grants`・`DELETE /oauth/grants/{id}` — 利用者本人が許可したアプリの一覧(`id`・`client_id`・`client_name`・範囲と説明・`created_at`・`updated_at`。最近使ったものから順)と、取り消し(204)。**一覧は、既存の一覧(`GET /shops`・`GET /reviews`)と同じ契約でページ送りする**: `page` / `per_page`(整数でなければ 422、範囲外は補正。1 ページの件数は backend が決める。既定 20 件・上限 100 件。既定値・上限・範囲外の丸めは、業務の規則として `domain/page.go`(`DefaultPerPage`・`MaxPerPage`・`NormalizePage`)にあり、ショップ・レビュー・接続済みアプリの一覧が、すべて同じ規則に従う。`page` / `per_page` の文字列を整数に読む処理は handler の `pageParams`、DB の limit/offset への変換は usecase の `clampPage`、続きを知るために 1 件多く取り出して切り詰める処理は adapter の `trimPage` が持つ)、続きがあるかはレスポンスヘッダー `X-Has-More`(`true` / `false`)で返す。取り消すと、そのアプリのトークンはすぐ使えなくなる。別の利用者の許可・存在しない許可・正規の形でない id は、区別できない同一の 404。プロフィール画面の「接続済みのアプリ」が使う。

**ルール**(判断は `internal/domain/oauth*.go` だけが持つ)

- 範囲: 読み取り(`hamburger:read`)と書き込み(`hamburger:write`)。指定がなければ読み取りだけ。要求が、すでに許可した範囲を超えたときだけ、許可を尋ね直す(`domain.OAuthConsentRequired`)。許可の記録(`oauth_grants`)は、利用者とアプリの組ごとに 1 行で、範囲は広がる方向にだけ更新する。
- 有効期間: 認可コード 2 分、アクセストークン 15 分、更新トークン 30 日(業務のルールで、環境変数では変えない)。
- トークンは不透明な文字列で、DB(`oauth_token_sessions`)には署名だけを保存し、文字列そのものは保存しない。取り消しは、記録を消す・無効にするので、すぐ効く。
- 認可コードは 1 回だけ使える。再利用(並行した 2 回の使用を含む)を検知したら、その認可から発行されたトークンをすべて使えなくする。更新トークンは使うたびに入れ替え、入れ替え済みのものの再利用を検知したら、同じくその系列を全部使えなくする。判定と更新は 1 つの SQL 文で行う。 **認可コードを使用済みにしてから、トークンを保存するまで(更新トークンの入れ替えから、新しいトークンを保存するまでも同じ)は、1 つの DB トランザクション**にまとめる(認可ライブラリの `Transactional` の口。`usecase.OAuthTokenSessionStore`、実装は `adapter/uow`)。まとめないと、再利用を検知した別の要求が系列を取り消したあとに、先の交換が、まだ保存していなかったトークンを保存して、取り消したはずのトークンが有効なまま残る。まとめれば、認可コードの行のロックで、あとから来た要求は、先の交換が確定するまで待たされ、そのあとで取り消すので、保存されたトークンも取り消される。系列の取り消し(アクセストークンの削除と更新トークンの無効化)も、1 つの SQL 文で行い、失敗しても片方だけが反映されることはない。
- 認可コードを発行するとき(`IssueAuthorizationCode`)は、許可の記録が、その利用者・そのアプリのものか、要求された範囲を許可済みかを、保存先から確かめる(`domain.OAuthGrant.Permits`)。別の許可の記録の id や、許可していない範囲を渡されても、認可コードは発行せず、確かめた範囲だけを付与する。
- 持ち主が退会(論理削除)した利用者には、認可コードの交換でも更新でも、新しいトークンを発行しない(`invalid_grant`)。退会(`DELETE /users/:id`)は、同じトランザクションで、その利用者のすべての許可(と、発行済みのトークンの記録)を取り消す。アクセストークンの検証(`usecase.OAuthAccessTokens.Authenticate`)の判定の順は「宛先 → 持ち主が有効か → 範囲」で、退会済みの持ち主のトークンは、範囲に関係なく、常に無効(401 相当)になる。
- 宛先(`resource`): トークンは、`OAUTH_RESOURCE_URL` の宛先だけに発行する(指定がなければその宛先、違えば `invalid_target`)。保護する側は、宛先が自分のトークンだけを受け付ける。
- 戻り先: https、またはループバックの http だけを登録できる。照合は完全一致で、`127.0.0.1` と `[::1]` だけポート番号の違いを許す(`localhost` はポートまで完全一致)。独自スキーム(`myapp://`)は登録できない。
- PKCE: `S256` だけ必須(`domain.ValidateOAuthPKCE`)。`state` も必須(ライブラリの既定)。確認用の文字列(`code_verifier`)が違う交換を 1 回でも行うと、その認可コードは、正しい値でも使えなくなる(推測を繰り返させない。ライブラリの挙動)。
- アプリの登録: 秘密の鍵を持たない公開クライアントだけ。**固定で登録**(`OAUTH_STATIC_CLIENTS`)するか、アプリが自分の説明を https の URL で公開する方式(CIMD。`client_id` がその URL)。CIMD の文書は、サーバーが取りに行くので、内部のサーバーへ向けさせる攻撃(SSRF)への対策を必ず守る: https だけ・標準のポートだけ・ホスト名だけ(IP アドレスの直接指定は不可)・接続の直前に、名前解決の結果が公開のアドレスかを確認(ループバック・プライベート・リンクローカルを断る)・リダイレクトを追わない・プロキシを使わない・待ち時間 5 秒・本文 64 KiB まで・種類は `application/json` だけ(`HTTPMetadataFetcher`)。文書の `client_id` が URL と違えば断り、許す使い方・範囲・宛先は、文書に何を書いても広がらない。取得した内容は 5 分覚える。

**環境変数**(有効にしたのに足りない・不正なときは起動時に落ちる。値はログ・エラーに出さない)

| 変数 | 必須 | 内容 |
|---|---|---|
| `OAUTH_ISSUER` | 任意(設定すると有効) | この API の公開 URL(例: `http://localhost:8080`)。認可サーバーの情報の `issuer` と、戻り先の `iss` になる |
| `OAUTH_TOKEN_SECRET` | 有効なとき必須 | トークンの署名に使う秘密の鍵。**32 文字以上・秘密。ログ・コード・PR に書かない** |
| `OAUTH_RESOURCE_URL` | 任意 | トークンの宛先。既定は `<OAUTH_ISSUER>/mcp` |
| `OAUTH_CONSENT_URL` | 任意 | 許可を尋ねる画面の URL。既定は `<APP_BASE_URL>/oauth/authorize` |
| `OAUTH_STATIC_CLIENTS` | 任意 | 固定で登録するアプリ。JSON の配列 `[{"id":"…","name":"…","redirect_uris":["…"]}]` |
| `MCP_ALLOWED_ORIGINS` | 任意 | リモートの MCP(`/mcp`)が受け付ける Origin(ブラウザが付ける要求元)。カンマ区切りの `scheme://host[:port]`。既定は `OAUTH_ISSUER` の Origin だけ(設定すると置き換える)。ワイルドカードと `null` は起動時に拒否する。Origin のない要求(ブラウザ以外)は、この設定に関係なく通る |

**主なクライアントとの相性**(公式ドキュメントで確認した内容。Claude Code は、`/mcp` に、トークンつきでつなぐところまで実機で確認した(下の「リモートの MCP サーバー」)。ブラウザで許可する流れ(未ログイン → ログイン → 許可の画面 → トークン交換 → `/mcp`)は、自作のクライアントと実際のブラウザで確認した。Claude Code 自身の OAuth の実行は、非対話(`-p`)ではできず、対話画面での確認は、まだ)

- Cursor: 固定のクライアント ID を設定する方式(動的登録・CIMD は使わない)。戻り先は `http://localhost:8787/callback` と `https://www.cursor.com/agents/mcp/oauth/callback` → `OAUTH_STATIC_CLIENTS` で足りる。
- Claude Code: 既定は動的登録(DCR)で、認可サーバーが CIMD に対応していれば CIMD も使う。戻り先は `http://localhost:<ランダムなポート>/callback`。`--client-id` と `--callback-port` で固定すれば、固定で登録したアプリ(戻り先は `http://localhost:<そのポート>/callback`)でつなげる。
- Claude(claude.ai・デスクトップのカスタムコネクタ): 動的登録を試み、詳細設定でクライアント ID を指定できる。戻り先は `https://claude.ai/api/mcp/auth_callback`(固定で登録できる)。
- **動的登録(DCR。`POST /oauth/register`)は未対応**。どのクライアントにも、固定の登録か CIMD の道があること、誰でも登録できる窓口は表を増やし続ける悪用の余地があること、が理由。実機で必要と分かったら、別の PR で足す。

**制限**: アプリの説明を取りに行く回数の制限(レート制限)は、まだない(SSRF の対策と、取得結果のキャッシュだけ)。

### Google のアカウントでのサインイン

メール + パスワードのほかに、Google のアカウントで、サインイン・新規登録・結び付けができる(`internal/adapter/googleauth`。認可コード + PKCE(S256)。ライブラリは `github.com/coreos/go-oidc/v3` と `golang.org/x/oauth2`)。`GOOGLE_CLIENT_ID` を設定したときだけ有効で、設定しなければ、`/auth/google/*` と `/me/identities*` は登録されず(404)、`GET /meta` の `login_providers` は空になる。Google Cloud での準備と手元での確かめ方は `docs/google-login-setup.md`。

- 流れ: ① 画面の「Google でサインイン」が `GET /auth/google/start`(query の `return_to`)へ移動 → ② API が、手続きの秘密(state・nonce・PKCE の検証値・戻り先)を、暗号化した短命の cookie に封じて、Google の認可の画面へ 302 → ③ Google が `GET /auth/google/callback` へ戻す。API が、**state に対応する手続きの cookie を取り出して消し**、認可コードを交換し、ID トークン(署名・発行者・宛先・有効期限・nonce・`email_verified`)を検証する → ④ **結果(成功も失敗も)は、短命(60 秒)・1 回限りの「画面へ渡すコード」に入れて**、frontend の `/auth/google/complete?code=…` へ 303。**同時に、コードを使える相手を確かめる「結び付けの値」を、このブラウザの cookie に設定する** → ⑤ 画面が `POST /auth/google/exchange`(`{"code": "…"}`。cookie は同じオリジンなので、自動で付く)で交換して、結果を受け取る。**ログインの証(JWT)は URL に載せず、交換の応答でだけ返す。**
- **cookie は 2 種類で、どちらも手続き(コード)ごとに別の名前**: ① 手続きの cookie `google_login_flow_<state のハッシュ>`(値は AES-256-GCM で封じ、**cookie の名前も認証に含める**ので、別の名前へ移し替えても開けない。**Path は戻り先の path そのもの**(例: `/api/auth/google/callback`)で、戻りの要求にだけ送られる。開始の要求には送られない)、② 結び付けの値の cookie `google_login_handoff_<コードのハッシュ>`(**Path は `…/auth/google/exchange`**。有効期間はコードと同じ 60 秒。交換で消す)。どちらも HttpOnly・SameSite=Lax(戻り先が https なら Secure)。手続きごとに別の cookie なので、同じブラウザの複数のタブで並行しても、互いを上書きせず、使った手続きの cookie だけを(同じ名前・Path で)消せる。件数・大きさの上限や、Path の導出・同名 cookie の統合は要らない。**`GOOGLE_REDIRECT_URL` の path は `/auth/google/callback` で終わらなければならない**(交換の cookie の Path を、ここから導く。起動時に断る)。
- **交換は、手続きを終えたブラウザの cookie にある「結び付けの値」と一緒でなければできない**(ログイン CSRF の防止): コードは URL に載って渡るので、コードだけを別のブラウザへ持ち込んでも(攻撃者が自分の Google で進めた `/auth/google/complete?code=C` を、被害者に踏ませても)、交換できず、被害者が攻撃者のアカウントでログインした状態にはならない。結び付けの値は、保存の形(SHA-256。`login_handoffs.binder_hash`)で照合する(`domain.LoginHandoff.BoundTo`)。合わない・ない交換は、無効なコードと同じ 400 で、**コードを消費しない**(別のブラウザの試みで、本来のブラウザのコードが使えなくならない)。
- **手続きの cookie がない(または合わない)コールバックでは、DB に何も書かず**、コードなしで結果の画面へ 303(手続きを始めていない要求で、`login_handoffs` に行を増やさせない。`usecase.ErrGoogleFlowMissing`)。**失敗の応答(409・400)にも、検証済みの `return_to` を含める**(AI アプリの許可の画面から来た利用者が、失敗のあと、元の要求へ戻れる。画面は、これをサインインの画面へ渡すだけ)。
- **交換(`Redeem`)は、後続の処理が成功してから、コードを消す**: 1 つのトランザクションの中で「コードをロック(`FOR UPDATE`)→ 内容を読む → 利用者の取得・トークンの発行 → コードを削除」。途中で失敗したら取り消すので、DB の一時的なエラーやトークンの発行の失敗で 500 になっても、コードは期限まで有効で、画面が同じコードで再試行できる。同じコードの並行する交換は、ロックで直列になり、成功するのは 1 回だけ。**トランザクションの中の読み取りは、プールからもう 1 つ接続を取らず、トランザクションの接続(`Tx.UserReads`)を使う**(取ると、同じコードを待つ処理が接続を使い切り、全体が止まる)。
- 識別は、メールでなく、Google の `sub`(`user_identities.provider_user_id`)。結び付いていればサインイン。結び付いておらず、同じメール(大文字小文字を区別しない)の利用者がいなければ、パスワードなしで新規登録(ユーザー名は `domain.UsernameFromProfile`。表示名だけが材料で、**メールの一部は使わない**(`user-<乱数>`)。書式の文字(Cf。双方向の上書き・ゼロ幅)は取り除く。あとで変更できる)。**同じ Google アカウントの初回のサインインが並行して、ユーザーの作成が負けたときは、sub で結び付きを引き直して、先に作られた利用者としてサインインさせる**(「パスワードでサインインしてください」と誤って案内しない)。**同じメールの利用者がいるときは、自動では結び付けない**(409 と案内。パスワードでサインインして、プロフィールから結び付ける)。**メールの一意性は、DB が、大文字小文字を区別せずに保証する**(`users` の `lower(email)` の一意の索引 `users_email_lower_key`。退会済みも対象。並行する登録の競合で、破られない。一意違反は `domain.ErrEmailTaken` になり、Google の新規登録は案内のエラー、メール確認での登録は「確認できない」になる)。
- 結び付け(ログイン済み): **`POST /me/identities/google/link`(認証つき。body の `return_to` は省略できる)が、その要求を出したブラウザで手続きを始め、Google の認可の画面の URL(`redirect_url`)を返す**。結び付ける利用者は、要求の認証から決まり、手続きの cookie(応答の Set-Cookie で、そのブラウザにだけ設定される)の中にだけある。**開始の URL やコードで、別のブラウザに利用者を伝える経路はない**(別のブラウザで開かせて、被害者の Google を攻撃者のアカウントに結び付ける攻撃を防ぐため。cookie を持たないブラウザでは、戻ってきても state が合わず失敗する)。画面は、`redirect_url` へ移動する。`GET /me/identities`(一覧。`can_unlink`)。`DELETE /me/identities/google`(204。**パスワードなしのアカウントは、サインインする方法がなくなるので 422**。判断は domain の `CanUnlinkIdentity`)。
- 戻り先(`return_to`)は、アプリの中のパスだけ(`domain.SanitizeReturnTo`。外部の URL・`//host`・`\`・制御文字は、既定の画面になる)。S39 の許可の画面へ戻る流れ(`/oauth/authorize?...`)も、この経路で戻る。
- パスワードなしのアカウント: `users.password_digest` は NULL。**明示したときだけ作る**(`CreateUserParams.Passwordless`。digest が空なのに明示していない作成は、`domain.ErrInvalidPasswordDigest` で拒否する。空の digest を黙って NULL にしない)。**退会すると、結び付きも削除する**(退会のトランザクションで、OAuth の許可の取り消しと同じく)。パスワードでのサインインは、知らないメールと同じ失敗(文言・ステータス・hash の比較 1 回分)になる。
- テーブル: `user_identities`(`UNIQUE(provider, provider_user_id)`・`UNIQUE(user_id, provider)`。Google のトークンは保存しない)、`login_handoffs`(画面へ渡すコードの中身。`code_hash`・`binder_hash` は SHA-256。使うと消える)。`users.password_digest` を NULL 可にしたので、**適用済みの開発用 DB は作り直す**(`migrate drop -f` → `migrate up` → `seed`)。
- テスト: 本物の Google にはつながず、`internal/testutil/fakeoidc`(OpenID Connect の提供元の代役)を使う。**本番のコードから import しない。**

**環境変数**(有効にしたのに足りない・不正なときは起動時に落ちる。値はログ・エラーに出さない)

| 変数 | 必須 | 内容 |
|---|---|---|
| `GOOGLE_CLIENT_ID` | 任意(設定すると有効) | Google Cloud Console で作った OAuth クライアントの ID |
| `GOOGLE_CLIENT_SECRET` | 有効なとき必須 | そのクライアントの秘密の鍵。**秘密。ログ・コード・PR・チャットに書かない。`.env` は Git に入れない** |
| `GOOGLE_REDIRECT_URL` | 有効なとき必須 | Google が認可のあとに利用者を戻す URL(この API の `/auth/google/callback` の公開 URL)。Google Cloud Console の「承認済みのリダイレクト URI」と完全に一致させる(例: `http://localhost:8080/auth/google/callback`)。**https、または開発用のループバックの http だけ**(起動時に断る。`APP_BASE_URL` も、有効なときは同じ制約)。画面と同じサイト(同じホスト)にして、手続きの cookie が届くようにする |
| `GOOGLE_OIDC_ISSUER` | 任意 | OpenID Connect の提供元。既定は `https://accounts.google.com`。テスト・隔離した確認で、代役に向けるためだけにある。https か、ループバック(`localhost`・`127.0.0.1`・`[::1]`)の http だけ許す。**本番では設定しない** |

### リモートの MCP サーバー(`/mcp`)

AI アプリ(Claude Code など)が、このアプリのショップ・レビューを調べ、許可されたときだけ、利用者の名前でレビューの投稿・編集・削除とショップの申請をするための、MCP(Model Context Protocol。AI が外部のツールを使う標準の決まり)のサーバーである。backend-go の API と**同じプロセス**に入っていて(`internal/adapter/handler/mcp*.go`)、OAuth の認可サーバー(上の節)を有効にしたとき(`OAUTH_ISSUER`)だけ登録される。SDK は公式の `github.com/modelcontextprotocol/go-sdk`(Go 1.25 以上が必要)。

- **認証**: `POST /mcp` の `Authorization: Bearer <アクセストークン>`。トークンの確認は `usecase.OAuthAccessTokens.Authenticate`(宛先 → 持ち主が有効か → 範囲の順。判断は usecase と domain にあり、handler は結果を HTTP に写すだけ)。トークンは、宛先が `OAUTH_RESOURCE_URL`(既定 `<OAUTH_ISSUER>/mcp`)のものだけを受け付け、**受け取ったトークンを、別のサーバーや API に渡さない**(ツールは usecase を直接呼ぶ)。トークンを URL の query に入れない。
  - トークンがない・無効(存在しない・期限切れ・取り消し済み・宛先違い・持ち主が退会済み) → `401` と `WWW-Authenticate: Bearer resource_metadata="…"`(トークンがあって無効なときは `error="invalid_token"` も付く)。
  - 範囲が足りない → `403` と `WWW-Authenticate: Bearer error="insufficient_scope", scope="hamburger:write", …`(クライアントは、範囲を広げる許可を求め直せる)。
  - 保護されたリソースの情報(RFC 9728)は、トークンなしで `GET /.well-known/oauth-protected-resource`(と、リソースの path を足した `/.well-known/oauth-protected-resource/mcp`)。宛先・認可サーバーの場所・使える範囲を返す。
- **Origin の検証**(DNS の付け替え攻撃への対策): MCP の仕様は、Streamable HTTP のサーバーに、すべての接続で `Origin` を検証すること、不正なら `403` を返すことを求めている(2025-06-18 は MUST。最新の 2026-07-28 は「Origin が存在して不正なら 403」まで明記)。`POST /mcp` は、**認証より前**に検証する(トークンの確認にも、本文の読み取りにも進ませない)。`Origin` がなければ通す(Claude Code などブラウザ以外のクライアント)。あれば、`MCP_ALLOWED_ORIGINS` の一覧と、scheme・host・port の**完全一致**で比べる(`domain.NormalizeOrigin`。scheme と host の大文字小文字は区別せず、既定のポートは省いて比べる。部分一致はしない)。一覧にない・`null`・空・複数・path や末尾のスラッシュを持つ不正な形は、理由を返さず、固定の本文(`{"error":"Forbidden"}`)で `403`。CORS のヘッダーは返さない(別の Origin のブラウザから直接使うクライアントには、対応しない。事前確認(preflight)は承認されないので、ブラウザは本要求を送らない)。
- **範囲はツールごと**: 読み取りのツール(`get_meta`・`list_shops`・`get_shop`・`list_reviews`・`get_review`・`get_user`)は `hamburger:read`、書き込みのツール(`create_review`・`update_review`・`delete_review`・`submit_shop`)は `hamburger:write`。対応表は `mcpToolScopes` の 1 か所で、入口(本文から読み取った範囲の確認。範囲を広げる許可を求め直せる 403 を返すため)と、ツールを実行する直前の確認(`guarded`。SDK が本文を別の読み方で解釈しても、書き込みが通らないようにする二重の防御)の両方が使う。ツールを足すときは、この表に足す(足し忘れると、テストが落ちる)。初期化・ツールの一覧は、範囲を要求しない。
- **ツールの実体**: 既存の usecase を呼ぶだけ。権限(投稿者本人だけが編集・削除、審査待ちのショップの見え方)は usecase と domain にあり、ここに複製しない。返す JSON は、REST の API と同じ形(`newReviewResponse` などを共有)。エラーの文言も REST と同じで、知らないエラーは、詳細をログにだけ残し、利用者には `internal server error` だけを返す。
- **プロンプトインジェクションへの注意**: レビューの本文・店名・自己紹介は、他の利用者が書いた文字列である。ツールの説明と、接続時の説明(`instructions`)で、内容として扱い、その中の命令には従わないよう伝えている。書き込みのツールの説明には、実際にデータを変えること、実行前に利用者へ確認することを書いている。防げる保証はない(AI の判断による)ので、書き込みは、必要なときだけ許可する。
- **結果の大きさ**: 1 回のツールの結果は 64 KiB まで。超える一覧は、途中で切らずに、`per_page` を小さくするよう伝えるエラーにする。
- **動かし方**: 状態を持たない(セッションを作らない)・応答は JSON。要求ごとに、認証した利用者のためのサーバーを組み立てる。SDK は、`localhost` で受けた要求の `Host` が `localhost` でないと `403` にする(DNS の付け替えの攻撃への対策)。同じ機械の逆プロキシの後ろに置くときは、この既定が邪魔になりうる(その場合は `DisableLocalhostProtection` を検討する。本番の構成は未決)。
- **つなぎ方**(ローカル。Claude Code の例):
  1. API を、OAuth を有効にして起動する(`export OAUTH_ISSUER=http://localhost:8080 OAUTH_TOKEN_SECRET=$(openssl rand -hex 32)`)。Claude Code を固定のアプリとして登録する(`OAUTH_STATIC_CLIENTS='[{"id":"claude-code","name":"Claude Code","redirect_uris":["http://localhost:8788/callback"]}]'`)。
  2. `claude mcp add --transport http hamburger http://localhost:8080/mcp --client-id claude-code --callback-port 8788`
  3. Claude Code の `/mcp` から認可する。ブラウザで、ログインして、許可を選ぶ(許可の画面は frontend の `/oauth/authorize`)。
- **本番の公開**: 公開の HTTPS の URL・ドメインは未決(この story の外)。`OAUTH_ISSUER` / `OAUTH_RESOURCE_URL` を、その公開の URL に合わせる。

### エンドポイント

**ヘルスチェック**
- `GET /up` — ヘルスチェック (DB への ping)

**Google でのサインイン**(`GOOGLE_CLIENT_ID` を設定したときだけ。詳細は「Google のアカウントでのサインイン」)
- `GET /auth/google/start` — Google の認可の画面へ 302(query: `return_to`。サインイン・新規登録の手続き専用)
- `GET /auth/google/callback` — Google からの戻り。結果を入れた 1 回限りのコードを付けて、frontend の `/auth/google/complete` へ 303
- `POST /auth/google/exchange` — コードを交換して結果を返す。手続きを終えたブラウザの cookie(結び付けの値)が要る(サインインの成功 200 + `token`・`return_to`、結び付けの成功 200 + `linked`、重複 409・失敗 400(どちらも `errors` + `return_to`)、無効なコード・cookie なし 400)
- `POST /me/identities/google/link` — 結び付けの手続きを、要求を出したブラウザで始め(cookie を設定)、Google の認可の画面の URL(`redirect_url`)を返す (要認証)
- `GET /me/identities` — 結び付き(Google など)の一覧。`can_unlink` (要認証)
- `DELETE /me/identities/google` — 結び付きの解除 204 (要認証。解除するとサインインする方法がなくなるなら 422)

**OAuth の認可サーバー**(`OAUTH_ISSUER` を設定したときだけ。詳細は「OAuth の認可サーバー」)
- `GET /.well-known/oauth-authorization-server` — 認可サーバーの情報(RFC 8414)
- `GET /oauth/authorize` — 認可の入口。検証して、許可を尋ねる画面へ 303 で渡す
- `POST /oauth/token` — 認可コード(PKCE つき)・更新トークンを、トークンに交換する
- `POST /oauth/revoke` — トークンの取り消し(RFC 7009)
- `GET /oauth/authorize/request` / `POST /oauth/authorize/decision` — 許可の画面が使う API(要認証)
- `GET /oauth/grants` / `DELETE /oauth/grants/:id` — 許可したアプリの一覧と取り消し(要認証)

**リモートの MCP サーバー**(`OAUTH_ISSUER` を設定したときだけ。詳細は「リモートの MCP サーバー」)
- `POST /mcp` — MCP(Streamable HTTP。`Authorization: Bearer <アクセストークン>` が必須。範囲はツールごと)
- `GET /.well-known/oauth-protected-resource`(と `…/mcp`) — 保護されたリソースの情報(RFC 9728)

**認証**
- `POST /signup` — アカウントの作成を申し込み、確認メールを送る。**登録済みの email でも未登録の email でも、同じ 202 `{"message":"Confirmation email sent"}` を返す**(アカウント列挙の防止)。アカウントは、確認メールのリンクを開いて `POST /signup/confirm` を呼んで初めて作られる。検証は登録の有無に依存しないものだけで、違反は 422(username、email、password。email は形式(`net/mail` で解析でき、表示名などを含まないアドレスだけであること)を検証し、不正なら 422 `Email is invalid`。password は 8〜72 バイトで、半角英字・数字・記号をそれぞれ 1 文字以上含む。`PUT /users/:id` のパスワード変更にも同じ規則を適用する。password_confirmation は任意で、送った場合は password と不一致なら 422。規則の判定は backend の domain だけが持ち、frontend は説明文の表示と、サーバーの 422 メッセージの表示だけを行う)。「登録済み」を示すエラーは返さない
- `POST /signup/confirm` — 確認メールのリンクの平文トークン(`{"token":"…"}`)でアカウントを作成する。成功すると従来の signup と同じ 201 `{id, username, email, admin, can_moderate, token}`(`can_moderate` は login・`GET /me` と共通)を返し、そのままログイン状態にできる。期限切れ・存在しない・改ざん・使用済みのトークン(と、確認までの間に同じ email のユーザーが作られていた場合)は、区別できない同一の 400 `{"error":"Confirmation token is invalid or has expired"}`
- `POST /login` — 認証して JWT トークンを受け取る (email とパスワードは signup と同じ規則を `domain.ValidateCredentials` で判定し、満たさなければ照合の前に 422。規則を満たしたうえで誤っていれば 401 `Invalid email or password`)
- `GET /me` — Bearer トークンから解決した現在のユーザー(`id`・`username`・`email`・`admin`・`can_moderate`)。無効・期限切れのトークンは 401。frontend は、トークンの有効性を自分で判断せず、起動時にこの応答でログイン状態を復元する。`can_moderate`(moderation ができるか。domain の `User.CanModerate`)は、`POST /login`・`POST /signup` の応答にも含まれ、frontend は `admin` から権限を導かず、管理画面の出し分けをこの値で行う (要認証)
- `GET /meta` — frontend が描画・送信前の処理に使う、backend のルールの値(`{"rating": {"min": 1, "max": 5}, "photo": {"max_edge": 1600, "max_bytes": 5242880}, "text": {"review_comment_max_chars": 2000, "burger_name_max_chars": 100, "shop_name_max_chars": 100, "username_max_chars": 50, "bio_max_chars": 500, "moderation_note_max_chars": 500}, "password": {"min_bytes": 8, "max_bytes": 72}, "login_providers": []}`)。認証不要で、`Cache-Control: public, max-age=3600`。ルールを持つのは backend だけ(rating の範囲は domain の `MinRating` / `MaxRating`、文字数の上限は domain の `Max*Chars`、パスワードの長さは `MinPasswordBytes` / `MaxPasswordBytes`、写真の上限は `domain/photo.go` の `MaxPhotoEdge` と `MaxPhotoBytes`)で、frontend は定数を持たず、評価の選択肢・★の描画・絞り込み・写真の縮小・文字数のカウンター・パスワードの説明文にこの値を使う。文字数はコードポイント数(日本語・絵文字も 1 文字)、パスワードはバイト数(日本語は 1 文字が 3 バイト)。domain に `Max*` / `Min*` の公開の定数を足したときは、`GET /meta` に足すか、出さない理由を `handler/meta_limits_test.go` の一覧に書く(書かないとテストが失敗する)。frontend の文字数のカウンター(`CharCounter`)は表示だけで、上限を超えても入力も送信も止めない(判定は backend の 422)。`login_providers` は、パスワードのほかに使えるサインイン方法で、規則ではなく設定(環境変数)で決まる(Google が有効なら `["google"]`、無効なら空の配列。frontend は、これに含まれる方法のボタンだけを出す。MCP の `get_meta` には含めない)
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
- レビューに付ける写真(`POST /reviews`・`PUT /reviews/:id` の `photo` パート)は、**JPEG・PNG・WebP** だけを受け付ける。形式は、ファイルの中身で判別する(申告された Content-Type は見ない)。保存は、JPEG は JPEG、PNG は PNG、WebP は JPEG で、長辺を 1,600 px 以下に縮小して再エンコードする(拡大はしない)。向きは、保存する画素が正立するように直す
- **HEIC / HEIF は受け付けない**(422)。iPhone の既定の形式だが、対応しないことにした。理由: サーバーで変換するには、デコーダ(WASM の libheif。LGPL)の依存が要り、メモリを大きく使う(1,600 万画素で約 490 MiB)うえ、iPhone 15 以降の標準の写真(約 2,447 万画素)は、コンテナの上限(1 GiB)に収まらない。PC の Chrome は HEIC を読めないので、救うにはブラウザ側にもデコーダが要る。**iPhone の Safari は、選択欄(`accept`)が JPEG・PNG・WebP だけのとき、写真を JPEG に変換して渡す**ので、iPhone からの投稿は通る。frontend の `accept` に HEIC / HEIF を加えないこと(加えると、iPhone が HEIC のまま渡す)。検出は、ファイルの先頭の `ftyp` ボックスの主なブランド(`heic`・`heix`・`mif1`・`heif` など)だけを見る(`photo.looksLikeHEIF`)
- 上限(すべて backend で判定する): ファイルは 5 MiB、寸法は 1 辺 10,000 px かつ 2,400 万画素。寸法は、デコードの前に、ヘッダーから読んで確かめる(ファイルの 5 MiB と保存する長辺 1,600 px は、`GET /meta` で frontend にも伝えるので `domain/photo.go` が持つ。寸法の上限は、メモリの保護のための実装上の値なので `internal/photo` が持つ)。frontend は、送る前に、長辺が `GET /meta` の `photo.max_edge` を超える(またはファイルが `photo.max_bytes` を超える)写真を縮小する
- 422 のメッセージは原因ごとに分かれる: `Photo must be a JPEG, PNG, or WebP image`(対応しない形式・壊れている)、`Photo must be a JPEG, PNG, or WebP image (HEIC/HEIF is not supported)`(HEIC / HEIF。対応しない理由が分かる)、`Photo dimensions are too large (max 10000px per side and 24 megapixels)`(寸法)、`Photo is too large (max 5MB)`(ファイルの大きさ)
- `GET /photos/*` — ディスクに保存されたレビュー写真を配信 (認証不要。末尾が `/` のディレクトリ path は一覧せず 404、末尾 `/` なしは 301 で `/` 付きへ転送されてから 404)。`PHOTO_STORAGE` が `disk` (既定) のときだけ登録され、`s3` では登録されない (写真の URL は bucket の公開ドメインを指す)

**ユーザー**
- `GET /users/:id` — ユーザーを 1 人取得 (認証は任意。存在しない・退会済み・UUID の正規形でない id は同一の 404。自己紹介文(応答のキーは `bio`。書かれていなければ空文字)は、誰が閲覧しても含む。email・admin は、本人が閲覧したときだけ含む。`can_edit`: 閲覧者がこのプロフィールを編集・削除できるか(domain の `Manages`。本人だけ `true`)を常に含む)
- `PUT /users/:id` — ユーザーの更新 (要認証。本人のみ。usecase で判定。email を変更するときは、signup と同じ形式の検証を行う。自己紹介文(`bio`)は、送ったときだけ更新される(送らなければ変わらず、空文字を送ると消える)。上限は 500 文字で、超えると 422 `Bio is too long (maximum is 500 characters)`。応答にも `bio` を含む。新規登録では自己紹介文を設定できない(`POST /signup` の要求に含めても無視される))
- `DELETE /users/:id` — ユーザーの削除 (要認証。本人のみ。usecase で判定。論理削除と、統計の再計算の依頼、AI アプリへの許可の取り消しを、1 つのトランザクションで行う)

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
├── domains/      # auth、oauth、reviews、shops、users
├── api/          # API クライアント / HTTP 境界
├── states/       # グローバル state
├── lib/          # 共通ユーティリティ (date、i18n、rating、photoResize)
└── components/   # 共通 UI コンポーネント
```

### API の接続先

- ベースパスは既定で `/api` (同一オリジン)。環境変数 `VITE_API_BASE_URL` で変更できる。
- 開発時は Vite の proxy が `/api` を Go API へ転送する。転送先の既定は `http://host.docker.internal:8080` で、`VITE_API_PROXY_TARGET` で変更できる。レビュー写真の `/photos` も同じ転送先へ proxy される(本番の nginx にも `/photos/` がある)。

### 写真の送信

- 写真を選ぶ input の `accept` は、**JPEG・PNG・WebP だけ**にする。HEIC / HEIF を加えない: iPhone の Safari は、`accept` が JPEG・PNG・WebP だけのとき、写真を JPEG に変換して渡す(加えると、HEIC のまま渡り、backend は HEIC / HEIF を受け付けない)。
- 送る前に、`useCreateReview` / `useUpdateReview` が `shrinkPhoto` を通す。長辺が上限(`GET /meta` の `photo.maxEdge`)を超える、またはファイルが `photo.maxBytes` を超えるときだけ、canvas で縮小した JPEG にする(向きは `createImageBitmap` の `imageOrientation: "from-image"` で画素に反映する)。**上限の値は frontend に書かない**(backend の値を使う)。
- 縮小できないとき(ブラウザが画像を読み込めないなど)は、失敗にせず、元のファイルをそのまま送る。backend が受け付けるか、理由つきのメッセージ(422 の 4 種類。HEIC / HEIF は、対応しない理由つき)を返し、それを画面にそのまま出す。

### 許可を尋ねる画面と、接続済みのアプリ(`domains/oauth`)

- `/oauth/authorize`(要ログイン): AI アプリが、ログインと許可だけでつなぐための画面。backend の `GET /oauth/authorize` から、同じクエリのまま 303 で渡される。未ログインなら、`ProtectedRoute` がログイン画面へ送り、ログイン後に、この画面へ(クエリごと)戻す。戻り先は、ルーターの遷移の state(`from`)に入れる。URL のパラメーターには入れない(外部のリンクから任意の戻り先を指定されるのを防ぐ。`returnPathFrom` は、このアプリの中のパスだけを返す)。
- **画面の状態は、それを作った認可の要求(URL の query。`search`)に結び付ける**(`consentFlow.ts`)。同じ画面のまま query だけが変わったとき(履歴を戻る・進む)に、前のアプリの内容が残ったまま、新しいアプリへの許可を送ってしまうのを防ぐため: いまの URL のために作られた状態だけを見せ(それ以外は「確認中」で、ボタンも出ない)、許可・拒否として送るのは、いま画面に内容を見せている要求だけにする。URL が変わったら、前の取得は取り消し(`AbortController`)、遅れて返った応答は画面に届かない。
- 画面は、`GET /oauth/authorize/request` の結果(アプリの名前・範囲と説明・`consentRequired`)を表示し、選択を `POST /oauth/authorize/decision` に送って、返ってきた `redirectTo`(アプリへの戻り先)へブラウザを移す。`consentRequired` が false(すでに許可済みの範囲に収まる)なら、尋ねずに許可を送る。**要求の検証・範囲の説明・尋ねる必要があるかの判断は、すべて backend が行い、frontend は表示と送信だけ**を行う(範囲の名前や説明を frontend に持たない)。アプリへの戻り先は、http(s) のときだけ開く(`isNavigable`。ページの中でコードが動くのを防ぐ確認)。
- プロフィール(本人のときだけ。`canEdit`)に「Connected apps」を出す(`ConnectedApps`)。`GET /oauth/grants` の一覧(ページ送り。`X-Has-More` があるときに「Load more」で続きを取る。既存の一覧と同じ `useInfinitePages`。キャッシュのキーには利用者の id を含める)と、`DELETE /oauth/grants/{id}` の取り消し(確認のあと。読み込み済みの全ページを取り直す)。OAuth の認可サーバーが無効な環境(API が 404)では、何も出さない。

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
- いまの画面を再現したデザインは `design/files/hamburger-evaluation.penpot`(画面・部品・色と文字のスタイル)。画面を変えたら、`design/scripts/` で作り直す(手順は `design/README.md` の「いまの画面から作り直す」)。
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

同じ検査(`gofmt`・`go build`・`go vet`・DB つきの `go test -race`・sqlc の生成物の差分)は、`backend-go/` を変えた PR で CI(`.github/workflows/backend-go.yml`)も実行する。CI が赤いときは、手元で同じコマンドを再現して直す。

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
- PR を ready にする前に、`/code-review` を実行し(未検証の指摘は再現を確かめて対応)、
  結果の要約を PR のコメントに残す。オーケストレーターを通さない実装でも同じ。実行できなかった
  ときは、理由だけを書いて ready にせず、利用者に手動での実行を頼む。`git worktree` で作業して
  いるときは、対象の絶対パスを引数に明示する(明示しないと、主ディレクトリの変更を見る)。
