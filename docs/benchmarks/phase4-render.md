# Phase 4: Render(無料プラン)

- 計測日: 2026-09-22
- URL: `https://hamburger-evaluation.onrender.com`
- 種別: **無料プラン**(上限に達すると止まる。請求されない)
- カード: 不要
- メモリ: 512MB
- 起動モデル: 15 分無通信でスリープ

## レイテンシ(日本から、接続確立を含む)

| エンドポイント | n | p50 | p95 | 最小 | 最大 |
|---|---|---|---|---|---|
| `GET /meta`(DB を使わない) | 30 | **147ms** | 335ms | 141ms | 381ms |
| `GET /up`(DB に ping) | 100 | **322ms** | 514ms | 312ms | 714ms |
| `GET /shops` | 100 | 322ms | 346ms | 312ms | 512ms |
| `GET /reviews?per_page=20` | 100 | 325ms | 350ms | 313ms | 547ms |

ローカルの基準値は `/up` 2ms、`/shops` 3ms、`/reviews` 6ms(`phase0-baseline.md`)。

### 内訳の分解

`/meta` は DB を使わず静的な値を返す。`/up` は DB に ping する。この差が DB 往復のコストになる。

| 経路 | 時間 |
|---|---|
| 日本 → Render(往復) | 147ms |
| Render → Neon(往復) | **175ms** |

### リージョンが想定と違った(確認済み)

**実際の配置はオレゴン(US West)だった。** `render.yaml` に `region: singapore` と
書いたにもかかわらず反映されていない。手動でサービスを作ると既定のオレゴンになるため、
Blueprint を経由しなかったか、設定が上書きされたと考えられる。

これで数字がすべて説明できる。

| 経路 | 実測 | 妥当性 |
|---|---|---|
| 日本 → オレゴン(往復 + TLS) | 147ms | ○ 太平洋横断として妥当 |
| オレゴン → シンガポール(往復) | 175ms | ○ 米西海岸と東南アジア間として妥当 |

**アプリが米国、DB がシンガポール、利用者が日本という三角形**になっており、
どの辺も無駄に長い。`region: singapore` が効いていれば DB 往復は数 ms で済み、
`/up` は 150ms 程度に収まったはずである。

**Render はリージョンを後から変更できない。** 直すにはサービスを作り直す必要がある。

### 接続確立の影響は小さい

計測は毎回新しい接続を張るが、TCP 接続は平均 13ms しかかかっていない。
接続を使い回して測っても 290〜310ms で、**接続確立は主因ではない**。

## ビルドの問題

Blueprint または手動作成の設定で、次のエラーが出た。

```text
error: failed to solve: failed to compute cache key:
failed to calculate checksum of ref ...: "/go.sum": not found
```

`Dockerfile.prod` は `COPY go.mod go.sum ./` から始まるため、**ビルドコンテキストが
`backend-go` でないと失敗する**。Root Directory に `backend-go` を指定する必要がある。

Dockerfile 自体は見つかっているので、`dockerfilePath` の指定は合っていて `rootDir` だけがずれている。

**Render はビルドが失敗すると直前に成功したデプロイを動かし続ける。** 上のレイテンシは
その古いビルドに対する計測である。設定を直して再ビルドしたら測り直す。

## 未計測

- 写真のアップロード(24MP 単発・同時 2 本)。**Phase 0 では 512MB で OOM した**
- 30 分放置後のコールドスタート
- SMTP の疎通(Mailjet)
- リクエスト外での統計ワーカーの動作
- 実際のリージョン
