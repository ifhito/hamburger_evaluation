# Phase 4: Northflank(Sandbox)

- 計測日: 2026-09-22
- URL: `https://p01--burger-stack--b2q7fqtjc465.code.run`
- 種別: 無料枠(超過時の扱いは未確認)
- カード: 検証のみ(公式の案内。実際に課金されるかは未確認)
- 起動モデル: **常時起動**(スリープしない)
- リージョン: **US**

## 最大の発見: 無料枠では US しか選べない

Sandbox(無料枠)で選択できるリージョンは **US のみ**だった。
ADR-0002 では「Asia East がある」と書いていたが、**それは有料プランの話**である。

**無料枠でリージョンを選べないことが、レイテンシをそのまま決めている。**

## レイテンシ(日本から、接続確立を含む)

| エンドポイント | n | p50 | p95 | 最小 | 最大 |
|---|---|---|---|---|---|
| `GET /meta`(DB を使わない) | 60 | **479ms** | 550ms | 451ms | 664ms |
| `GET /up`(DB に ping) | 60 | **864ms** | 1,244ms | 830ms | 1,433ms |
| `GET /shops` | 60 | 864ms | 1,252ms | 832ms | 1,290ms |

### 内訳の分解

| 経路 | 時間 |
|---|---|
| 日本 → Northflank(往復) | 479ms |
| Northflank → Neon(往復) | **385ms** |

日本 → US の往復に TLS の確立が乗って 479ms。そこから DB(シンガポールの Neon)まで
さらに往復するため、合計で 864ms になる。

**アプリが US、DB がシンガポール、利用者が日本という三角形**が、この数字を作っている。
どの辺も短くできないため、無料枠のままでは改善の余地がない。

## Render との比較

| ホスト | `/meta` | `/up` | DB 往復の差分 |
|---|---|---|---|
| Render | 147ms | 322ms | 175ms |
| Northflank | 479ms | 864ms | 385ms |

Northflank は Render のおよそ 2.7 倍遅い。**どちらも DB がシンガポールなので、
差は主にアプリの配置から来ている。**

## 設定時の注意

Render と同じく、ビルドコンテキストの指定が要る。

| 項目 | 値 |
|---|---|
| Build context | `/backend-go` |
| Dockerfile path | `/backend-go/Dockerfile.prod` |
| Port | 8080 |
| Health check | `/up` |

`Dockerfile.prod` が `COPY go.mod go.sum ./` から始まるため、コンテキストが
`backend-go` でないと `go.sum not found` で失敗する。

## 未計測

- 写真のアップロード(24MP 単発・同時 2 本)
- Sandbox のメモリ割り当て
- 超過時に課金されるか
- SMTP の疎通(Mailjet)
- 常時起動なので、リクエスト外でも統計ワーカーが動くはず。その確認
