# ADR-0003: データベースのホスティング

- ステータス: 提案中(Proposed)
- 日付: 2026-09-21(2026-09-20 初版を、実スキーマの確認を受けて改訂)
- 対象: PostgreSQL(`backend-go` から pgx / sqlc で接続)
- 方針: **無料で始める**
- 関連: [ADR-0002](0002-backend-deploy-target.md)、[ADR-0005](0005-async-job-platform.md)

## 背景

初版は Rails の `db/schema.rb` を見て書いた。実体は `backend-go/db/migrations/` の SQL 12 本で、
**初版の前提が 2 つ間違っていた**。

### 訂正 1: 主キーは UUID(bigint 連番ではない)

```sql
CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    ...
```

- `gen_random_uuid()` を使うため **PostgreSQL 13 以上**が必須。
- 初版は「bigint 連番」を前提に互換性を判定していた。分散 SQL の評価をやり直す(下記)。

### 訂正 2: テーブルは 11 本(6 本ではない)

| テーブル | 用途 |
|---|---|
| `users` / `shops` / `burgers` / `shops_burgers` / `reviews` / `burger_stats` | 本体 |
| `signup_verifications` / `mail_deliveries` | サインアップ確認メール([ADR-0006](0006-email-delivery.md)) |
| `burger_stats_recalc_requests` | 統計再計算の待ち行列([ADR-0005](0005-async-job-platform.md)) |
| `oauth_grants` / `oauth_token_sessions` | OAuth 認可サーバー |

### 行が増え続けるテーブルがある

- `oauth_token_sessions`: アクセストークン 15 分、更新トークン 30 日。期限切れ行が残る。
- `mail_deliveries`: 送信履歴。冪等キーのため消さずに溜まる。
- `burger_stats_recalc_requests`: 処理済みは消えるので溜まらない。

**期限切れ行の定期削除が要る。** 無料枠は容量が 0.5GB 程度なので、放置すると効く。
削除ジョブは [ADR-0005](0005-async-job-platform.md) で扱う。

### その他の前提

- ドライバは pgx v5(`pgxpool`)。接続数は `DB_MAX_CONNS` で設定できる。
- 外部キー制約と `CHECK` 制約を多用する(文字数上限を DB にも二重に持つ方針)。
- `db/migrations_test.go` が、domain の定数と DB の `CHECK` の食い違いを検出する。
- データ規模は小さく、無料枠の 0.5GB に収まる見込み。

## 評価軸

1. **無料枠の条件**(期限、容量、休止・停止の条件と復帰方法、カード要否)
2. **PostgreSQL 13 以上**(`gen_random_uuid()`)
3. 東京または近傍リージョン
4. バックアップと復元
5. 接続数の上限とプーラーの要否(`DB_MAX_CONNS` の設定値に効く)
6. pgx / sqlc との互換性(外部キー・`CHECK` 制約・`FOR UPDATE SKIP LOCKED`)

## 無料で始められる候補

条件は 2026-09 時点の目安。

| 候補 | 無料枠 | 休止の挙動 | 立地 | バックアップ | 接続の注意 |
|---|---|---|---|---|---|
| Neon | 無期限。0.5GB。カード不要 | 無通信で自動休止、復帰 1 秒未満 | シンガポール | PITR 6 時間 | プーラー経由。`DB_MAX_CONNS` を小さく |
| Supabase | 無期限。500MB。カード不要 | **1 週間無通信で停止。手動復帰** | ◎ 東京 | 日次 7 日保持 | Supavisor 経由推奨 |
| Render Postgres | **30 日で失効**。以後 $6〜7/月。カード不要 | 休止しない | シンガポール | 有料プランのみ | API が Render なら内部 URL |
| Xata | 無期限(要確認) | 自動休止 | 要確認 | ブランチ | 直接接続 |

いずれも PostgreSQL 13 以上なので `gen_random_uuid()` は動く。

### 所感

- **Neon**: 無料枠で本番運用が最も現実的。休止復帰が速い。Render / Cloud Run どちらとも組める。
- **Supabase**: 東京が使える一方、1 週間放置で停止し手動復帰が要る。個人アプリの放置期間と相性が悪い。
- **Render Postgres**: API(ADR-0002 の第一候補)と同居したときの内部接続の速さを測るための枠。
  無料枠は 30 日で失効するが、検証は数日で終わるので支障はない。**実際に採用しうる構成そのもの**を測れる。
- **Xata**: Neon の代替枠。

## ニッチ・分散 DB の再評価

**初版で下げた理由の一部が、UUID 主キーによって消えた。**

| 候補 | 初版の評価 | 改訂後 | 理由 |
|---|---|---|---|
| CockroachDB Cloud | △ 連番 PK が分散で非推奨 | **○ に格上げ** | UUID なので hotspot の問題が起きない。pgx は公式サポート |
| YugabyteDB Aeon | △ 同上 | **○ に格上げ** | 同上。Postgres 互換が最も深い |
| Amazon Aurora DSQL | ✕ シーケンス・外部キー非対応 | **✕ 維持** | 外部キーを多用するため依然として不可 |
| TiDB(MySQL 互換) | ✕ | ✕ 維持 | sqlc のエンジン変更とクエリ書き直しが要る |
| Turso / SQLite | △ | ✕ | `gen_random_uuid()`・`SKIP LOCKED`・pgx 前提と噛み合わない |

ただし格上げは「技術的に動く」という意味であり、**採用の推奨ではない**。
単一リージョンの個人アプリでは、分散のコスト(書き込みレイテンシ、運用の複雑さ)だけを払う。
[検証計画](../deploy-verification-plan.md)では、個人向けでないという理由で検証対象から外している。

## それ以外(有料のみ)

| 候補 | 最小コスト | 備考 |
|---|---|---|
| Fly.io Managed Postgres | $5/月前後 | 東京 |
| PlanetScale Postgres | $39/月〜 | 過剰 |
| AWS RDS / Cloud SQL | $10〜15/月〜 | 過剰 |

## 実測(2026-09-21、Phase 1)

4 社すべてで**マイグレーション 12 本が無改変で適用でき、sqlc の生成コードもそのまま動いた**。
PostgreSQL は 17 以上で `gen_random_uuid()` も問題ない。**互換性では差がつかなかった。**

差が出たのはレイテンシだけで、しかもほぼ地理的距離で決まった(日本から計測)。

| 候補 | リージョン | `GET /shops` p50 | 接続数 |
|---|---|---|---|
| Neon | Singapore | **80ms** | 50 まで OK |
| Render Postgres | Oregon | 139ms | 50 まで OK |
| Supabase | Mumbai | 142ms | 50 まで OK |
| Xata | US East | 187ms | 50 まで OK |

`/up`(DB に ping するだけ)と `/shops`(実クエリ)の p50 がほぼ同じで、
**この規模では DB の処理時間ではなく往復の時間が支配的**である。

詳細と、Supabase の Direct connection が IPv6 専用で Docker から繋がらない件は
`docs/benchmarks/phase1-summary.md` にある。

### リージョンの制約

今回の順位は、無料枠で選べるリージョンがそのまま出たものである。

- Neon: 無料枠は ap-southeast-1(シンガポール)。東京はない
- Supabase: **新規プロジェクトなら東京(ap-northeast-1)を選べる**。今回は既存の
  ムンバイのプロジェクトを使ったため不利に出た
- Render: 無料枠は us-west(オレゴン)。シンガポールは有料プランのみ

**Supabase を東京で作り直せば Neon を下回る可能性が高い。** 再計測の価値がある。

## 決定(案)

- **Neon(無料枠)** を第一候補とする。
- API を Render に置く場合、同居構成として **Render Postgres** も測る。ただし無料枠が 30 日で失効するため、
  長期に無料で運用するなら Neon のままにする。
- 分散 DB は採用しない。技術的な障害は減ったが、利点が要件に無い。

有料化するとき: Neon の Launch($19/月)、または API が Render なら Render Postgres($6〜7/月)へ
`pg_dump` / `pg_restore` で移す。

## 結果・影響

- `DB_MAX_CONNS` を無料枠の上限に合わせる。Neon のプーラー経由なら 5〜10 程度から試す。
- Neon の休止復帰は速いが、最初の 1 クエリだけ遅い。`GET /up` が DB に ping するので、
  ヘルスチェックが実質的なウォームアップになる。
- **期限切れ行の削除ジョブが要る**(`oauth_token_sessions`、`mail_deliveries`)。ADR-0005 で扱う。
- マイグレーションは `backend-go/db/migrations/` の SQL をそのまま適用する。ツールは golang-migrate 等。
- 無料枠のバックアップ保持は短い(Neon は 6 時間)。重要データが増えたら、週次 `pg_dump` を
  GitHub Actions で R2([ADR-0004](0004-photo-storage.md))に置く。
