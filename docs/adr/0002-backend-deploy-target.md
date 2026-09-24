# ADR-0002: API のデプロイ先

- ステータス: 承認(Accepted)。2026-09-24 に本番で採用
- 日付: 2026-09-21(2026-09-20 初版を、Rails 退役と実装の確認を受けて全面改訂)
- 対象: `backend-go/`(Go 1.27 / 標準 `net/http` / sqlc / pgx / PostgreSQL)
- 方針: **無料で始める**。個人が一人で運用できることを条件にする
- 関連: [ADR-0001](0001-frontend-deploy-target.md)、[ADR-0003](0003-database-hosting.md)、[ADR-0004](0004-photo-storage.md)、[ADR-0005](0005-async-job-platform.md)、[ADR-0006](0006-email-delivery.md)

## 背景

Rails は退役済み(`chore!: Rails 退役 — backend/ を削除`)。`backend-go` が唯一のバックエンドである。
初版は「Go へ移行予定」という前提で書いたが、実装は完成している。以下は実装を読んで確認した事実。

### 動かすうえで効く性質

| 性質 | 実態 | デプロイ先選びへの影響 |
|---|---|---|
| 成果物 | 静的リンクのバイナリ 1 本 | どのコンテナ基盤でも動く |
| Dockerfile | **開発用**。`golang:1.27` のフルイメージで、コンテナ起動時に `go build` してから実行 | 本番用の作り直しが必須(下記) |
| 常駐処理 | 統計再計算ワーカーが goroutine で常駐(既定 1 秒間隔) | **リクエスト外の CPU 割り当て**が要る |
| 画像処理 | アップロードを API が受けてサーバー側でデコード・縮小 | **メモリ**が要る。dev は `mem_limit: 1g` |
| メール送信 | SMTP で外部へ送信 | **送信ポートの開放**が要る |
| OAuth | CIMD でクライアントの説明を https で取得しに行く | **外向き HTTPS**が要る |
| 認証 | JWT(stateless)+ OAuth トークンは DB 保存 | セッションストア・スティッキーセッション不要 |
| ヘルスチェック | `GET /up`(DB への ping) | そのまま使える |

### 本番用 Dockerfile(作成済み)

当初の `backend-go/Dockerfile` は開発用で、`CMD ["sh", "-c", "go build ... && exec /tmp/api"]`
となっていた。イメージは 1GB 近く、**起動のたびにコンパイルが走る**。512MB の無料枠では
起動中に OOM しうるうえ、コールドスタートの計測も成立しない。

そこで本番用(distroless + 静的リンク)を用意し、**名前を入れ替えた**。

| ファイル | 用途 |
|---|---|
| `Dockerfile` | **本番用**。30.9MB、起動は即時 |
| `Dockerfile.dev` | 開発用。`docker-compose.yml` が使う |
| `Dockerfile.prod` | 互換のための別名 |

入れ替えた理由は、**多くの基盤が Dockerfile 名を選べない**ためである。
Back4App Containers は指定できず、既定の `Dockerfile` を見る(2026-09-22 実測)。
本番用を既定の名前に置かないと、そうした基盤で開発用がビルドされてしまう。

実測: **イメージ 29MB、待機時メモリ 12.75 MiB、起動は即時**(`docs/benchmarks/phase0-baseline.md`)。

### メモリが無料枠の壁になる(実測で確認済み)

写真は API がデコードして長辺 1600px に縮小する。上限は 1 辺 10,000px かつ 2,400 万画素、ファイル 5MiB。

Phase 0 の実測結果(2,400 万画素・4.6MB の JPEG)。

| 条件 | 結果 |
|---|---|
| 512MB・単発 | 成功。ピーク 260.5 MiB、1.07 秒 |
| 512MB・**起動直後**に同時 2 本 | 5 回中 0 回 OOM |
| 512MB・**1 本処理した後**に同時 2 本 | 3 回中 **3 回 OOM**(ExitCode 137) |
| 512MB + `GOMEMLIMIT=400MiB`・同上 | 3 回中 **3 回 OOM** |
| 768MB・同上 | 3 回中 0 回 OOM |
| 1GB・同上 | 3 回中 0 回 OOM |

Go の GC が約 260MiB のヒープを OS へすぐ返さないため、一度使ったあとは前回分が残った状態で
次の山が来て超える。**実運用は常に「育った状態」なので、512MB では落ちる。**
`GOMEMLIMIT` は効かない。**必要なのは 768MB 以上。**

### 環境変数

起動に必要なものが多い。全ホストで同じ名前を使う。

| 分類 | 変数 |
|---|---|
| 基本 | `PORT`、`APP_BASE_URL`、`DATABASE_URL`、`DB_MAX_CONNS` |
| 認証 | `JWT_SECRET`、`JWT_TTL` |
| 写真 | `PHOTO_STORAGE`、`PHOTO_S3_ENDPOINT`、`PHOTO_S3_BUCKET`、`PHOTO_S3_ACCESS_KEY_ID`、`PHOTO_S3_SECRET_ACCESS_KEY`、`PHOTO_PUBLIC_BASE_URL`、`PHOTO_DISK_DIR` |
| メール | `SMTP_HOST`、`SMTP_PORT`、`SMTP_USER`、`SMTP_PASSWORD`、`SMTP_SECURITY`、`MAIL_FROM` |
| OAuth | `OAUTH_ISSUER`、`OAUTH_TOKEN_SECRET`、`OAUTH_RESOURCE_URL`、`OAUTH_CONSENT_URL`、`OAUTH_STATIC_CLIENTS` |
| ワーカー | `STATS_WORKER_INTERVAL`、`STATS_WORKER_BATCH`、`STATS_WORKER_MAX_ATTEMPTS` |

`OAUTH_ISSUER` と `APP_BASE_URL` は環境ごとの絶対 URL で、フロント(ADR-0001)の URL と整合させる必要がある。

## 評価軸

初版の 6 項目に、実装を読んで判明した 3 項目を足す。

1. **無料枠の条件**(期限、スリープ・scale-to-zero、上限、カード要否)
2. **メモリ**(768MB 以上を無料または安価に確保できるか)
3. **リクエスト外の CPU 割り当て**(常駐ワーカーが動くか)
4. **外向き SMTP**(送信ポートが塞がれていないか)
5. コンテナをそのまま動かせるか
6. 東京または近傍リージョン
7. デプロイ前コマンド(マイグレーション)の仕組み
8. 同一事業者に DB があるか
9. 運用の手間

2〜4 は初版になかった軸で、いずれも候補の順位を変えうる。2 は実測で確定した。

## 無料で始められる候補

条件は 2026-09 時点の目安。

| 候補 | 無料枠 | メモリ | リクエスト外 CPU | SMTP | 立地 | デプロイ前 |
|---|---|---|---|---|---|---|
| Render | 無期限。750 時間/月。15 分でスリープ。カード不要 | 512MB | ○ プロセスは生きている | **✕ 587 を塞ぐ(実測)** | **オレゴン(変更不可)** | ◎ Pre-Deploy Command |
| Zeabur | 無料枠あり(要確認) | 要確認 | ○ | 要確認 | ◎ 東京 | ○ |
| Google Cloud Run | 無期限。200 万リクエスト/月、36 万 GB 秒/月。カード要 | **設定可**(既定 512MB) | △ リクエスト外は絞られる | **○ 通る(実測で確認)** | ◎ 東京 | ○ Jobs |
| Northflank(Sandbox) | サービス 2 つ・DB 1 つ・cron 2 つ。**常時起動** | 要確認 | ○ | 要確認 | Asia East | ○ |
| Back4App Containers | 600 時間/月・5 プロジェクト。カード不要 | **256MB** | ○ 常時起動 | 要確認 | 要確認 | ○ |

### 所感

- **Render**: 手数最少でカード不要。スリープからの復帰は Go なら速い。フロントと同居できる。
  ただし**無料枠は 512MB で、写真の同時アップロードで落ちる**。
- **Zeabur**: 東京かつ日本語 UI。個人開発者向け。無料枠の条件が変動しやすい。
- **Cloud Run**: 無料枠は最大で立地も最良。**メモリを自由に設定でき、従量課金なので
  1GB に上げても低トラフィックならほぼ無料のまま**でありうる。これは実測で判明したメモリ問題に対する
  最も安い解になりうる。一方、リクエスト外の CPU 抑制(常駐ワーカー)と外向き SMTP の遮断という
  2 つの懸念があり、いずれも本アプリが実際に使っている機能に当たる。

**Koyeb は候補から外す。** 2026-09-21 時点で利用登録ができなかったため、検証もできない。
条件が変われば再評価の価値はある(東京で API・DB・静的を同居できる数少ない無料 PaaS)。

## それ以外(有料のみ、または個人向けでない)

| 候補 | 理由 |
|---|---|
| Railway | 初回トライアルのみ無料。$5/月〜 |
| Fly.io | 新規向け無料枠が廃止 |
| Oracle Cloud Always Free(VM) | 無料だが OS・TLS・バックアップ・監視を自分で持つ |
| AWS App Runner / ECS、Azure Container Apps | 権限とネットワークの設計が要る |
| Kamal / Coolify + VPS | サーバー運用を自分で持つ |
| Cloudflare Workers 本体 | Go は TinyGo → Wasm になり pgx が動かない |

## 実測(2026-09-22、Phase 4)

4 ホストに本番用イメージをデプロイし、同じ Neon・R2・Mailjet を向けて計測した。

| ホスト | 種別 | 立地 | メモリ | 写真(24MP) | SMTP | 総合 |
|---|---|---|---|---|---|---|
| **Cloud Run** | 無料枠(超過で課金) | **東京** | 1GB | **通る** | **通る** | **○** |
| Render | 無料プラン | オレゴン | 512MB | 落ちる | **塞がれている** | ✕ |
| Back4App | 無料プラン | 米国 | 256MB | 落ちる | 通る | △ |
| Northflank | 無料枠 | US | 256MB | 落ちる | 通る | △ |

**このアプリを全機能動かせたのは Cloud Run だけだった。**

### 第一候補を Render から Cloud Run に変える

本 ADR は当初 Render を第一候補としていたが、実測で 3 つの問題が出た。

1. **SMTP が塞がれている**(`dial tcp ...:587: i/o timeout`)。確認メールが送れず、
   **サインアップが完了しない**。これは致命的である
2. 512MB では 24MP の写真が単発でも落ちる
3. **リージョンがオレゴンで、後から変更できない**。`render.yaml` に `region: singapore` と
   書いたが反映されなかった

**Render では運用できない。**

ただし Cloud Run に変えると、本 ADR で重視した「事故で請求が発生しない」という利点を失う。
**「請求リスクなし」と「機能が動く」が両立しなかった**のが Phase 4 の結論である。

### レイテンシ

| ホスト | `/meta`(DB なし) | `/up`(DB あり) | DB 往復 |
|---|---|---|---|
| Cloud Run(東京) | **46ms** | 287ms | 241ms |
| Render(オレゴン) | 147ms | 322ms | **175ms** |
| Back4App(米国) | 193ms | 421ms | 228ms |
| Northflank(US) | 479ms | 864ms | 385ms |

**「アプリが近い」ことと「DB が速い」ことは別問題だった。** Cloud Run は `/meta` で圧勝だが、
DB 往復ではオレゴンの Render より遅い。scale-to-zero で DB 接続が維持されず、
毎回張り直しているためと考えられる。**Neon を東京に置けば解消する**見込み。

### 無料枠ではリージョンが選べない

| ホスト | 希望 | 実際 |
|---|---|---|
| Render | シンガポール | **オレゴン**(変更不可) |
| Northflank | Asia East | **US**(無料枠は US のみ) |
| Cloud Run | 東京 | **東京** |

**無料枠で東京に置けたのは Cloud Run だけ。** レイテンシの順位はほぼこれで決まっている。

### 常駐ワーカーの懸念は実害が小さい

本 ADR で Cloud Run の順位を下げた根拠の 1 つだったが、影響は限定的である。

ワーカーはプロセス内の goroutine なので、scale-to-zero では動かない。ただし実装は
依頼を DB に積み、**起動直後に 1 サイクル回して取りこぼしを回収する**。
したがって「動かなくなる」のではなく「**次のアクセスまで遅れる**」だけである。

Render のスリープと同じ構造で、Cloud Run 固有の問題ではない。

詳細は `docs/benchmarks/phase4-summary.md` にある。

## 決定

**Google Cloud Run(東京 `asia-northeast1`、メモリ 1GB)** に置く。サービス名は `burger-stack`。

検証前の案は Render を第一候補、Cloud Run を第 3 位としていた。実測で順位が入れ替わった。

| 候補 | 写真(2,400 万画素) | SMTP | 立地 | 結果 |
|---|---|---|---|---|
| **Cloud Run** | 通る(1GB) | 通る | **東京** | **採用** |
| Render | 502(512MB) | **塞がれている** | オレゴン(変更不可) | サインアップが完結しない |
| Back4App | 502(256MB) | 通る | 米国 | 一時 URL が期限切れになる |
| Northflank | 503(256MB) | 通る | US のみ | 最も遅い |

**全機能を動かせたのは Cloud Run だけだった。** 代わりに、「上限で止まるだけで請求されない」という無料プランの利点を失う。Cloud Run は超過すると課金され、予算アラートは通知するだけで支払いを止めない。

検証前に挙げたメモリ問題の 4 つの手のうち、「Cloud Run で 1GB に設定する」を選んだことになる。

## 結果・影響

- **本番用の Dockerfile** は `backend-go/Dockerfile`(多段ビルド + distroless、約 31MB、非 root)。開発の compose は `Dockerfile.dev` を使う。
- **配るのは GitHub Actions**(`.github/workflows/deploy-api.yml`)。マイグレーションを当ててから、要求を流さないリビジョンを作り、その専用の URL で確かめてから切り替える。GCP へは Workload Identity Federation で入り、長く使える鍵を持たない。
- **Cloud Run のコンソールの継続的デプロイは無効にした**(2026-09-24)。マイグレーションを当てずにコードだけ入れ替え、GitHub Actions と後勝ちで競合するため。
- **環境変数は Cloud Run 側に置く。** CD はイメージだけを入れ替え、環境変数に触れない。ただし画面で変数を変えると、切り替え前の確認を通らずに本番へ出る。
- `GET /up` がヘルスチェック。DB への ping を含む。
- **要求がない間はインスタンスが 0 になる。** 統計の再計算ワーカーも止まり、次の起動直後の 1 サイクルで溜まった依頼を処理する([ADR-0005](0005-async-job-platform.md))。
- **OOM はログに何も残らない**(カーネルによる強制終了)。監視は `ExitCode=137` と再起動回数で行う。
- 手順と必要な権限は [本番へのデプロイ](../production-deploy.md) にある。
