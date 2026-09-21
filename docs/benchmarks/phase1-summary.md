# Phase 1: データベース 4 候補のまとめ

- 計測日: 2026-09-21
- 計測元: 日本(自宅回線)のローカル Docker から、本番用イメージ `hamburger-api:prod` 経由
- データ量: 全社そろえて shops 54 / burgers 206 / reviews 1,009
- 個別の結果: `phase1-neon.md`、`phase1-supabase.md`、`phase1-render.md`、`phase1-xata.md`

## 結果

| 候補 | リージョン | PostgreSQL | マイグレーション | `GET /up` p50 | `GET /shops` p50 | `DB_MAX_CONNS` |
|---|---|---|---|---|---|---|
| Neon | Singapore(ap-southeast-1) | 18.6 | 12 本 OK(37 秒) | **79ms** | **80ms** | 50 まで OK |
| Render Postgres | Oregon(us-west) | 18.6 | 12 本 OK(23 秒) | 139ms | 139ms | 50 まで OK |
| Supabase | Mumbai(ap-south-1) | 17.6 | 12 本 OK(22 秒) | 142ms | 142ms | 50 まで OK |
| Xata | US East(us-east-1) | 18.6 | 12 本 OK(29 秒) | 186ms | 187ms | 50 まで OK |

参考: ローカルの PostgreSQL 16 コンテナでは `/up` 2ms、`/shops` 2〜3ms(`phase0-baseline.md`)。

## 読み取り

### 1. 互換性はすべて問題なし

4 社とも PostgreSQL 17 以上で、`gen_random_uuid()` が使え、マイグレーション 12 本が
**無改変で適用できた**。UUID 主キー、外部キー 6 本、`CHECK` 制約、`db/migrations_test.go` が
求める制約名も含めてそのまま通る。sqlc の生成コードも 4 社すべてで無改変に動いた。

**互換性は候補選びの判断材料にならない。** 差が出たのはレイテンシだけだった。

### 2. 差はほぼ地理的距離で決まる

| 候補 | 距離 | p50 |
|---|---|---|
| Neon | シンガポール | 79ms |
| Render | オレゴン | 139ms |
| Supabase | ムンバイ | 142ms |
| Xata | 米国東部 | 187ms |

`/up`(DB に ping するだけ)と `/shops`(実クエリ)の p50 がほぼ同じであることに注目する。
**DB の処理時間ではなく往復の時間が支配的**で、この規模のデータでは DB の性能差は測定に現れない。

### 3. リージョンの選択が結果を決めている

今回はアカウント作成時に選べるリージョンをそのまま使った。実際には次の制約があった。

- Neon: 無料枠で ap-southeast-1(シンガポール)が選べた。東京はない
- Supabase: 既存プロジェクトが ap-south-1(ムンバイ)だった。**新規なら東京も選べる**
- Render: 無料枠は us-west(オレゴン)。シンガポールは有料プランのみ
- Xata: us-east-1

**Supabase を東京で作り直せば、この表の順位は変わる可能性が高い。** 東京なら往復は 10〜30ms 程度に
なるはずで、Neon の 79ms を大きく下回る。これは再計測する価値がある。

### 4. 接続数は 4 社とも 50 まで問題なし

`DB_MAX_CONNS` を 5 / 10 / 20 / 50 と変えて、起動と同時リクエストを試した。
どれも失敗しなかった。無料枠でも、この規模なら接続数が制約にならない。

## ハマった点

### Supabase の Direct connection は IPv6 専用で Docker から繋がらない

最初に設定された URL は `db.<ref>.supabase.co`(Direct connection)で、次のエラーが出た。

```text
hostname resolving error: lookup db.<ref>.supabase.co ... : no such host
```

`dig` で確認したところ **A レコードがなく AAAA のみ**だった。macOS のホストからは
`psql` で繋がるが、Docker コンテナの DNS では解決できない。

**Session pooler**(`aws-0-<region>.pooler.supabase.com`)に替えると通った。こちらは
AWS の ELB を指す IPv4 のホスト名である。

同じ理由で、Transaction pooler(6543)も避けている。pgx は既定で prepared statement を
使う(`QueryExecModeCacheStatement`)が、transaction モードのプーラーはコネクションを
トランザクション単位で使い回すため相性が悪い。

### psql とコンテナで到達性が違う

上の件の直接の帰結として、**ホストの `psql` で繋がっても、コンテナから繋がるとは限らない**。
`db-verify.sh` は psql をホストで、マイグレーションと API をコンテナで動かすため、
この差が「psql は通ったがマイグレーションが失敗し、テーブルが無い」という形で現れた。

## 未実施

**休止からの復帰時間**は自動化できないため未計測。放置してから 1 クエリ投げて測る。

```bash
source backend-go/.env.bench
time psql "$NEON_URL" -c 'select 1'
```

| 候補 | 放置 | 期待される挙動 | 実測 |
|---|---|---|---|
| Neon | 30 分 | 自動復帰。1 秒未満 | (未計測) |
| Supabase | **8 日** | 停止。手動復帰が要る可能性 | (未計測) |
| Render | 30 分 | 休止しない想定 | (未計測) |
| Xata | 30 分 | 自動休止 | (未計測) |

Supabase の 8 日は、**今回アクセスした 2026-09-21 が起点**になる。2026-09-29 以降に測る。

## ADR への反映

[ADR-0003](../adr/0003-database-hosting.md) の決定(Neon を第一候補)は**実測で支持された**。
ただし理由は当初の想定と違う。

- 当初の理由: 無料枠が無期限で、休止からの復帰が速い
- 実測で判明した理由: **日本から最も近い**(シンガポール)。互換性では差がつかない

したがって「Supabase を東京で作り直す」が有効な打ち手になりうる。ADR-0003 に追記した。
