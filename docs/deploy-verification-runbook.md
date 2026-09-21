# デプロイ先 検証手順書

- 日付: 2026-09-21(実装の確認を受けて改訂)
- 対象: [検証計画](deploy-verification-plan.md) で選んだ 6 層 24 候補
- 前提となる判断: [ADR-0001〜0006](adr/)
- **検証には本物の `backend-go` と `frontend` をそのまま使う**(最小構成は作らない)

## 検証の順番と、その理由

**インフラ → API → フロントエンド** の順で進める。後ろの層が前の層の成果物を必要とするためで、
逆順にすると毎回ダミーを用意することになる。

```text
Phase 0  準備          本番用 Dockerfile + 計測スクリプト
   ↓
Phase 1  DB            DATABASE_URL が出る      → Phase 3 が使う
   ↓
Phase 2  写真          PHOTO_S3_* が出る        → Phase 3 が使う
   ↓
Phase 3  メール        SMTP_* が出る            → Phase 4 が使う
   ↓
Phase 4  API           公開 URL が出る          → Phase 5, 6 が使う
   ↓
Phase 5  非同期        API を外から叩く
   ↓
Phase 6  フロント      API へ /api を転送する
```

メールを API より前に置くのは、API の起動に `SMTP_*` が要るためである。
ただし**ポートが塞がれているかは API を立てるまで分からない**ので、Phase 3 では
送信サービス側の準備までを行い、ホストごとの疎通確認は Phase 4 で行う。

各 Phase は独立して中断・再開できる。1 Phase が 1 本の記事に対応する。

---

## 共通ルール

検証を始める前に決めておく。あとから揃えるのは不可能なので、最初に守る。

### 計測条件をそろえる

- **同じ回線・同じ端末**から測る。自宅回線とモバイル回線を混ぜない。
- **同じ時間帯**に測る。層ごとに 4 候補を連続で測り、日をまたがない。
- 1 指標につき **3 回測って中央値**を採る。1 回だけの値は記事に出さない。
- 計測日時を必ず記録する。無料枠の条件もサービス性能も変わるため、数字は「いつ時点か」とセットでないと意味がない。

### 記録の置き場所

| 種類 | 置き場所 |
|---|---|
| 生データ(CSV) | `docs/benchmarks/raw/<label>.csv`(スクリプトが自動生成) |
| 各 Phase の結果まとめ | `docs/benchmarks/phase<N>-<層名>.md` |
| 手順で詰まった点・ハマりどころ | 同じまとめファイルの「ハマった点」節 |

記事の数字は必ず生データと対応させる。読者が追試できることを優先する。

### シークレットの扱い

- 接続文字列・アクセスキー・JWT シークレットを**リポジトリに書かない**。
- ローカルは `.env.local`(gitignore 済みを確認)、各ホストは環境変数の画面に直接入れる。
- 記事に貼るスクリーンショットは、接続文字列の部分を必ず隠す。
- 検証が終わったキーは撤収時に**失効させる**。残したまま記事を書かない。

### 課金事故を防ぐ

無料枠でもカード登録を求めるサービスがある。登録したら必ず次をやる。

- GCP: 予算アラートを 1 ドルで設定する。
- Cloudflare / Render / Upstash: ダッシュボードの使用量ページをブックマークし、Phase の最後に確認する。
- 各 Phase の終わりに**撤収手順**を実行する。使い終わったリソースを放置しない。

### 計測スクリプト

`scripts/bench/` に 2 本ある。どちらも curl だけで動く。

```bash
# 定常時のレイテンシ(n 回叩いて p50 / p95)
./scripts/bench/latency.sh <url> 100 <label>

# 放置後の初回応答(wait 分待って 1 回叩く、を trials 回)
./scripts/bench/coldstart.sh <url> 30 5 <label>
```

`coldstart.sh` は 30 分 x 5 回で約 2.5 時間かかる。バックグラウンドで流す。

```bash
nohup ./scripts/bench/coldstart.sh https://example.com/healthz 30 5 cold-render > /tmp/cold-render.log 2>&1 &
```

---

## Phase 0: 準備(検証の土台を作る)

### 目的

**本物の `backend-go` を 4 候補すべてに載せられる状態**にする。
初版では「最小構成を実装する」としていたが、実装は完成しているので不要になった。

### 手順

1. **本番用 Dockerfile を作る。** 現行の `backend-go/Dockerfile` は開発用で、
   コンテナ起動時に `go build` する。このままではコールドスタートの計測が成立しない。

   ```dockerfile
   FROM golang:1.27 AS build
   WORKDIR /src
   COPY go.mod go.sum ./
   RUN go mod download
   COPY . .
   RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

   FROM gcr.io/distroless/static
   COPY --from=build /out/api /api
   ENV PORT=8080
   EXPOSE 8080
   ENTRYPOINT ["/api"]
   ```

   既存の開発用と共存させる(`Dockerfile.prod` にするか、ステージを分ける)。

2. 内部ジョブのエンドポイントを足す([ADR-0005](adr/0005-async-job-platform.md))。Phase 5 で使う。

   | エンドポイント | 用途 |
   |---|---|
   | `POST /internal/jobs/recalculate-all` | 全バーガーの再計算依頼を積む |
   | `POST /internal/jobs/cleanup-expired` | 期限切れトークンとメール履歴を削除 |

   共有シークレットのヘッダで保護する。公開 API の JWT とは別系統にする。

3. 環境変数の一覧を確定する。全ホストで同じ名前を使う。

   | 分類 | 変数 |
   |---|---|
   | 基本 | `PORT`、`APP_BASE_URL`、`DATABASE_URL`、`DB_MAX_CONNS` |
   | 認証 | `JWT_SECRET`、`JWT_TTL` |
   | 写真 | `PHOTO_STORAGE`、`PHOTO_S3_ENDPOINT`、`PHOTO_S3_BUCKET`、`PHOTO_S3_ACCESS_KEY_ID`、`PHOTO_S3_SECRET_ACCESS_KEY`、`PHOTO_PUBLIC_BASE_URL` |
   | メール | `SMTP_HOST`、`SMTP_PORT`、`SMTP_USER`、`SMTP_PASSWORD`、`SMTP_SECURITY`、`MAIL_FROM` |
   | OAuth | `OAUTH_ISSUER`、`OAUTH_TOKEN_SECRET`、`OAUTH_CONSENT_URL`、`OAUTH_STATIC_CLIENTS` |
   | ジョブ | `JOB_SECRET`、`STATS_WORKER_INTERVAL` |

4. ローカルの Docker Compose で本番用イメージを起動し、`GET /up` と `GET /shops` が返ることを確認する。

5. 計測の基準値を取る。

   ```bash
   ./scripts/bench/latency.sh http://localhost:8080/shops 20 local-baseline
   ```

6. **写真アップロードのメモリ使用量をローカルで測る。** Phase 4 の予測に使う。

   ```bash
   docker stats --no-stream   # 2,400 万画素の JPEG を投稿しながら
   ```

### 完了条件

- 本番用イメージが 30MB 以下でビルドできる
- 内部ジョブのエンドポイント 2 本が動く
- `local-baseline` が取れている
- ローカルでの写真アップロード時のピークメモリが分かっている

## Phase 1: データベース(4 候補)

### 目的

無料枠の Postgres が「休む」ことが実用上どれだけ問題かを測る。
マイグレーション 12 本(UUID 主キー・外部キー・`CHECK` 制約)が無改変で通るかも確認する。

### 候補

| # | 候補 | 取得するもの | 事前に必要なもの |
|---|---|---|---|
| 1 | Neon | プーラー経由の接続文字列 | メールアドレスのみ |
| 2 | Supabase | 接続文字列(**Session pooler**。ポート 5432) | メールアドレスのみ |
| 3 | Render Postgres | 内部・外部それぞれの接続文字列 | メールアドレスのみ(無料枠は 30 日で失効) |
| 4 | Xata | 接続文字列 | メールアドレスのみ |

### 手順(各候補で行う)

1. サインアップし、東京または最寄りのリージョンでインスタンスを作る。選べない場合はその事実を記録する。
2. **PostgreSQL のバージョンを確認する。** `gen_random_uuid()` を使うため 13 以上が必須。

   ```bash
   psql "$DATABASE_URL" -c 'select version()'
   psql "$DATABASE_URL" -c "select gen_random_uuid()"
   ```

3. 接続文字列を `.env.local` に控える。リポジトリには書かない。
4. マイグレーションを適用する。

   ```bash
   migrate -path backend-go/db/migrations -database "$DATABASE_URL" up
   ```

   **記録**: 12 本すべてが無改変で通ったか。通らなければエラー全文を残す。
5. シードを投入する。全候補で同じ件数にする。

   ```bash
   DATABASE_URL="$DATABASE_URL" go run ./cmd/seed
   ```

6. ローカルの API を `DATABASE_URL` だけ差し替えて起動し、`GET /shops` が返ることを確認する。
7. `DB_MAX_CONNS` を 5 / 10 / 20 と変えて、どこでエラーになるか記録する。

### 計測

候補 1 社分は `db-verify.sh` が通しで行う(版の確認・マイグレーション・データ投入・
レイテンシ・接続数の上限)。

```bash
source backend-go/.env.bench
./scripts/bench/db-verify.sh neon "$NEON_URL"
```

読み書きの統計は `db-ops.sh` で別に取る。アプリの HTTP 層を挟まず pgx で直接叩くので、
DB とネットワークの往復だけが出る。読み 5 種と書き 4 種について、平均・標準偏差・
p50 / p90 / p95 / p99 を記録する。

```bash
./scripts/bench/db-ops.sh neon "$NEON_URL" 200
```

書き込みで作った行は計測の最後に削除するので、候補間でデータ量がずれない。

`/shops` と `/up` の差が DB 往復のコストになる。

休止からの復帰は手動で測る。指定時間放置してから 1 クエリ投げ、返るまでを計る。

| 候補 | 放置時間 | 期待される挙動 |
|---|---|---|
| Neon | 30 分 | 自動復帰。1 秒未満 |
| Supabase | **8 日** | 停止。ダッシュボードから手動復帰 |
| Render Postgres | 30 分 | 休止しない想定。要確認 |
| Xata | 30 分 | 要確認 |

Supabase の 8 日放置は**検証全体の最初に仕掛け、他の Phase と並行して寝かせる**。
直列でやると検証が 1 週間止まる。

### 記録すること

`docs/benchmarks/phase1-database.md` に書く。

- PostgreSQL のバージョンと `gen_random_uuid()` の可否
- マイグレーション 12 本が無改変で通ったか
- `/shops` と `/up` の p50 / p95、およびその差
- 休止復帰の実測値
- `DB_MAX_CONNS` の上限
- `STATS_WORKER_INTERVAL=1s` のままで、無料枠のコンピュート時間がどれだけ減るか
- 無料枠の実条件、ハマった点

### 撤収

Phase 4 で 1 つを本採用するので、この時点では消さない。

---

## Phase 2: 写真ストレージ(4 候補)

### 目的

`PHOTO_S3_ENDPOINT` の差し替えだけで `adapter/storage/s3.go` が 4 社すべてで動くかを確認し、
**転送量**が無料枠をどれだけ食うかを測る。

**注意**: 写真は API が受けてサーバー側で処理する。署名付き URL も、バケットのアップロード用
CORS 設定も**不要**である([ADR-0004](adr/0004-photo-storage.md))。

### 候補

| # | 候補 | 取得するもの | 事前に必要なもの |
|---|---|---|---|
| 1 | Cloudflare R2 | エンドポイント、バケット、アクセスキー | カード登録 |
| 2 | Supabase Storage | S3 互換エンドポイント、キー | Phase 1 の 2 と同じアカウント |
| 3 | Tigris | エンドポイント、キー | カード登録 |
| 4 | Backblaze B2 | エンドポイント、キー | メールアドレスのみ |

### 手順(各候補で行う)

1. バケットを作り、**読み取りだけ**公開する。書き込みはアクセスキーを持つ API だけに限る。
2. アクセスキーを発行し `.env.local` に控える。
3. ローカル API の環境変数を差し替えて起動する。

   ```bash
   PHOTO_STORAGE=s3
   PHOTO_S3_ENDPOINT=...
   PHOTO_S3_BUCKET=...
   PHOTO_S3_ACCESS_KEY_ID=...
   PHOTO_S3_SECRET_ACCESS_KEY=...
   PHOTO_PUBLIC_BASE_URL=...
   ```

   **記録**: `adapter/storage/s3.go` のコードを変えずに動いたか。

4. 写真つきレビューを投稿する。

   ```bash
   curl -X POST "$API/reviews" -H "Authorization: Bearer $TOKEN" \
     -F 'rating=5' -F 'comment=テスト' -F 'photo=@sample.jpg' -w '\n%{http_code}\n'
   ```

5. 応答に含まれる写真 URL が `PHOTO_PUBLIC_BASE_URL` を指し、ブラウザから取得できることを確認する。
6. カスタムドメインを設定する(1 の R2 のみ。他は既定のドメインで可)。

### 計測

```bash
./scripts/bench/latency.sh "<公開画像 URL>" 100 storage-r2-get
```

- **転送量の消費**: 同じ画像を 1000 回取得し、ダッシュボードの使用量の増分を記録する。
  ここが各社の無料枠の寿命を決める。

### 記録すること

`docs/benchmarks/phase2-photo.md` に書く。

- エンドポイント差し替えだけで動いたか(4 社それぞれ)
- 画像配信の p50 / p95(日本から)
- 1000 回配信で消費した転送量と、無料枠の残り
- カスタムドメイン設定の手数
- ハマった点

### 撤収

不採用の 3 つはバケットを削除し、**アクセスキーを失効させる**。

---

## Phase 3: メール送信(4 候補)

### 目的

送信サービス側の準備と、ローカルからの疎通を確認する。
**ホストごとにポートが塞がれているかは Phase 4 で測る。**

### 候補

| # | 候補 | 無料枠 | 事前に必要なもの |
|---|---|---|---|
| 1 | Resend | 3,000 通/月 | メールアドレス。独自ドメイン推奨 |
| 2 | Brevo | 300 通/日 | メールアドレス |
| 3 | SendGrid | 100 通/日 | メールアドレス |
| 4 | Gmail SMTP | 500 通/日 | Google アカウント + アプリパスワード |

### 手順(各候補で行う)

1. サインアップし、SMTP の認証情報を取得する。
2. 独自ドメインを使う場合は SPF / DKIM を設定する(1〜3)。**記録**: DNS レコードの本数と反映までの時間。
3. ローカル API の環境変数を差し替えて起動する。

   ```bash
   SMTP_HOST=... SMTP_PORT=587 SMTP_USER=... SMTP_PASSWORD=... SMTP_SECURITY=starttls MAIL_FROM=...
   ```

4. サインアップを実行し、確認メールが届くことを確認する。

   ```bash
   curl -X POST "$API/signup" -H 'Content-Type: application/json' \
     -d '{"email":"...","username":"...","password":"..."}'
   ```

5. `mail_deliveries` テーブルを確認する。

   ```sql
   SELECT kind, status, failure_kind, attempts, last_error FROM mail_deliveries ORDER BY created_at DESC LIMIT 5;
   ```

6. **冪等性を確認する。** 同じメールアドレスで 2 回サインアップし、メールが 1 通しか出ないことを見る。
7. 迷惑メール判定の有無を、独自ドメイン設定の前と後で比べる。

### 記録すること

`docs/benchmarks/phase3-email.md` に書く。

- 認証情報が手に入るまでの手数
- SPF / DKIM の設定本数と反映時間
- 送信から受信までの所要時間
- 迷惑メールに入ったか(ドメイン設定の前後)
- `mail_deliveries` の記録が期待どおりか
- ハマった点

### 撤収

Phase 4 まで 1 つを残す。不採用の 3 つは API キーを失効させる。

---

## Phase 4: API(4 候補)

### 目的

**メモリと CPU をいつもらえるか**を測る。検証全体の本丸。

### 前提

- Phase 0 の本番用 Dockerfile がビルドできること
- Phase 1 の `DATABASE_URL`、Phase 2 の `PHOTO_S3_*`、Phase 3 の `SMTP_*` が 1 組ずつ確定していること

**重要**: 4 候補すべてに**同じ DB・同じストレージ・同じメール送信**を向ける。
Render Postgres の同居効果は、Render の計測時に追加パターンとして別に測る。

### 候補と手順

#### 1. Render

1. GitHub を接続し、`backend-go/` をルート、本番用 Dockerfile を指定する。
2. 環境変数を入れる(Phase 0 の一覧すべて)。
3. Pre-Deploy Command にマイグレーションを設定する。
4. ヘルスチェックパスを `/up` にする。
5. デプロイし、公開 URL を控える。`OAUTH_ISSUER` と `APP_BASE_URL` をその URL に合わせて再デプロイする。

**追加パターン**: Render Postgres を作り、内部接続文字列に差し替えたインスタンスをもう 1 つ立てる。
同居の効果はここで測る。

#### 2. Zeabur

1. GitHub 連携でデプロイする。リージョンは**東京**。
2. **記録**: 日本語 UI がどこまで日本語か。ドキュメントの言語も記録する。

#### 3 と 4. Google Cloud Run(512MB と 1GB)

1. GCP プロジェクトを作り、課金アカウントを紐づける。
2. **予算アラートを 1 ドルで設定する**(ここで必ずやる)。
3. デプロイする。

   ```bash
   gcloud run deploy hamburger-api \
     --source backend-go --region asia-northeast1 \
     --allow-unauthenticated --memory 512Mi \
     --set-env-vars "..."
   ```

4. **同じ手順で `--memory 1Gi` の 2 つ目をデプロイする**(サービス名を変える)。
   Phase 0 で判明した OOM を、メモリを上げれば避けられるか、そのとき無料枠がどれだけ減るかを測る。
5. **記録**: プロジェクト作成からデプロイ成功までの所要時間と操作回数。

### 計測(各候補で行う)

```bash
# 定常時
./scripts/bench/latency.sh https://<host>/up    100 api-<host>-up
./scripts/bench/latency.sh https://<host>/shops 100 api-<host>-shops

# 放置後の初回応答(4 候補を同時に流してよい)
nohup ./scripts/bench/coldstart.sh https://<host>/up 30 5 cold-<host> > /tmp/cold-<host>.log 2>&1 &
```

**この Phase 固有の 3 項目**。いずれも机上では分からない。

1. **写真アップロードのメモリ**

   2,400 万画素の JPEG(5MiB 近く)を投稿する。**必ず 1 本投稿してヒープを育ててから同時 2 本**を試す。
   Phase 0 で、起動直後なら 512MB でも通り、1 本処理した後は必ず OOM することが分かっている
   (`docs/benchmarks/phase0-baseline.md`)。冷えた状態だけ測ると誤った結論になる。

   ```bash
   curl -X POST "https://<host>/reviews" -H "Authorization: Bearer $TOKEN" \
     -F 'rating=5' -F 'comment=大きい写真' -F 'photo=@24mp.jpg' -w '\n%{http_code} %{time_total}\n'
   ```

   **記録**: 成功したか、502 / OOM で落ちたか、所要時間。落ちた場合はメモリを上げて再試行し、
   何 MB あれば通るかを記録する。

2. **リクエスト外での統計ワーカーの動作**

   レビューを 1 件投稿し、**その後まったくリクエストを送らずに** 5 分待つ。
   そのあと `burger_stats_recalc_requests` を見て、行が消えている(= 処理された)かを確認する。

   ```sql
   SELECT burger_id, attempts, next_attempt_at FROM burger_stats_recalc_requests;
   ```

   **Cloud Run では残っている可能性がある。** 残った場合、`--no-cpu-throttling` を付けて再測定する。

3. **外向き SMTP の疎通**

   サインアップを実行し、メールが届くかを見る。

   ```sql
   SELECT status, failure_kind, last_error FROM mail_deliveries ORDER BY created_at DESC LIMIT 1;
   ```

   **Cloud Run は塞がれている想定。** 失敗した場合、`SMTP_PORT=465` + 暗黙 TLS でも試し、
   それでも駄目なら「API 送信の実装が必要」と記録する([ADR-0006](adr/0006-email-delivery.md))。

### 記録すること

`docs/benchmarks/phase4-api.md` に書く。

- デプロイ成功までの所要時間・コマンド数・操作回数・カード要否
- 放置 30 分後の初回応答の中央値
- 定常時の p50 / p95、`local-baseline` との差
- 東京(2, 3, 4)とシンガポール(1)の差
- **写真アップロードが 512MB で通ったか**
- **リクエスト外でワーカーが動いたか**
- **SMTP が通ったか**
- Render Postgres 同居パターンの `/shops` が、外部 DB(Neon)パターンと比べてどれだけ速いか
- ハマった点

### 撤収

本採用の 1 つを残し、他 3 つを削除する。
Cloud Run は**プロジェクトごと削除**する(Artifact Registry のイメージ課金を残さない)。

---

## Phase 5: 非同期処理(4 候補)

### 目的

定期ジョブ(全件再計算・期限切れ削除)を誰が起こすかを比べる。
統計ワーカー自体は現行実装を維持するので、検証対象は**起こす側**だけ。

### 前提

Phase 4 で API が 1 つ公開され、内部ジョブのエンドポイント 2 本が動いていること。

### 候補と手順

#### 1. cron-job.org

1. ジョブを登録する。URL は `POST /internal/jobs/recalculate-all`、ヘッダに `X-Job-Secret`。
2. 10 分おきに実行し、24 時間動かす。
3. **記録**: 設定時刻と実際の発火時刻の差。

#### 2. GitHub Actions(schedule)

1. ワークフローを作る。

   ```yaml
   on:
     schedule:
       - cron: "*/10 * * * *"
   jobs:
     call:
       runs-on: ubuntu-latest
       steps:
         - run: |
             curl -fsS -X POST "${{ secrets.API_URL }}/internal/jobs/recalculate-all" \
               -H "X-Job-Secret: ${{ secrets.JOB_SECRET }}" -w '\n%{http_code}\n'
   ```

2. 24 時間動かす。**記録**: 指定時刻からの遅延の分布。

#### 3. Upstash QStash

1. スケジュールを登録し、署名検証を API 側で確認する。
2. **再試行の確認**: API が一時的に 500 を返すようにして、何回リトライするかを記録する。

#### 4. Cloud Scheduler

1. API が Cloud Run の場合のみ。ジョブを登録する。
2. **記録**: 認証方式(OIDC トークン)の設定手数。

### 全候補で確認すること

全件再計算が正しく動くかを、依頼テーブルで追う。

```sql
-- 実行直後: 全バーガー分の行が積まれている
SELECT count(*) FROM burger_stats_recalc_requests;
-- 数十秒後: ワーカーが処理して減っている
SELECT count(*) FROM burger_stats_recalc_requests;
```

期限切れ削除も同様に、実行前後の行数を比べる。

```sql
SELECT count(*) FROM oauth_token_sessions;
SELECT count(*) FROM mail_deliveries;
```

### 記録すること

`docs/benchmarks/phase5-async.md` に書く。

- 設定完了までの手数
- 指定時刻からの発火遅延の分布(24 時間分)
- 全件再計算が処理しきるまでの時間(バーガー件数とあわせて)
- 再試行の挙動(3 と 4)
- ハマった点

### 撤収

外部サービスのジョブを削除する。
GitHub Actions のワークフローは**必ず無効化する**。放置すると 10 分おきに動き続ける。

---

## Phase 6: フロントエンド(4 候補)

### 目的

`/api/*` の転送を 4 方式で書き比べ、OAuth の制約を実証する。

### 前提

Phase 4 で確定した API の公開 URL があること。

**重要**: `frontend/src/api/client/buildApiClient.ts` は変更しない。
`VITE_API_BASE_URL` を使う直接接続方式は、Go に CORS 実装が無いため今回は検証しない。

### 候補と手順

#### 1. Render Static Site

```yaml
services:
  - type: web
    name: hamburger-frontend
    runtime: static
    buildCommand: pnpm install && pnpm build
    staticPublishPath: ./dist
    routes:
      - type: rewrite
        source: /api/*
        destination: https://<api-url>/*
```

#### 2. Cloudflare Workers(Static Assets)

```js
export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    if (url.pathname.startsWith("/api/")) {
      const target = new URL(url.pathname.replace(/^\/api/, ""), env.API_ORIGIN)
      target.search = url.search
      return fetch(new Request(target, request))
    }
    return env.ASSETS.fetch(request)
  },
}
```

#### 3. Netlify

```text
/api/*  https://<api-url>/:splat  200
```

#### 4. Vercel

```json
{ "rewrites": [{ "source": "/api/:path*", "destination": "https://<api-url>/:path*" }] }
```

**記録**: Hobby プランの非商用条項を利用規約から引用し、記事に正確に書く。

### 全候補で確認すること

1. ログイン、レビュー投稿(写真つき)、一覧表示が動くか
2. **deep link**: `https://<host>/reviews/<uuid>` に直接アクセスして 404 にならないか
3. 写真が `PHOTO_PUBLIC_BASE_URL` から表示されるか(API を経由しないこと)
4. **OAuth の制約の実証**: フロントの URL を `APP_BASE_URL` に設定し、`OAUTH_CONSENT_URL` が
   そのフロントを指すことを確認する。プレビュー URL では `redirect_uri` が登録されていないため
   OAuth が失敗することを、実際のエラー画面つきで記録する

### 計測

```bash
./scripts/bench/latency.sh https://<frontend>/api/shops 100 fe-<host>-proxied
./scripts/bench/latency.sh https://<api-url>/shops      100 fe-<host>-direct
```

差が転送のオーバーヘッドになる。

### 記録すること

`docs/benchmarks/phase6-frontend.md` に書く。

- 転送設定の記述量(行数)と疎通までの時間
- 転送経由と直叩きのレイテンシ差
- deep link が設定なしで動いたか
- OAuth の `redirect_uri` に登録が必要だった URL の数
- 無料枠の帯域上限と商用利用の可否
- ハマった点

### 撤収

不採用の 3 つはサイトを削除する。

---

## 全 Phase 完了後

### 撤収チェックリスト

| 項目 | 確認 |
|---|---|
| 不採用の DB インスタンスを削除した | |
| 不採用のストレージバケットを削除した | |
| 不採用のメール送信サービスの API キーを失効させた | |
| 発行したアクセスキーをすべて失効させた | |
| 不採用の API サービスを削除した | |
| Cloud Run のプロジェクトを削除した(Artifact Registry 含む) | |
| GitHub Actions の検証用ワークフローを無効化した | |
| cron-job.org / QStash のジョブを削除した | |
| 各サービスの請求画面で 0 円を確認した | |
| 不採用のフロントサイトを削除した | |
| OAuth の `OAUTH_STATIC_CLIENTS` から検証用の戻り先を消した | |

### まとめ記事

`docs/benchmarks/summary.md` に 24 候補を 1 表にまとめる。
列は「初回応答」「p50」「p95」「手数」「カード要否」「無料枠で先に尽きるもの」。

そのうえで、用途別に 2〜3 パターンの推奨構成を示す。
ADR の決定案と実測が食い違った場合は、**ADR 側を更新する**。検証はそのために行う。
