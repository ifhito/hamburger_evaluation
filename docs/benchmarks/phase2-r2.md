# Phase 2: r2

- 計測日: 2026-09-21 23:04
- エンドポイント: `https://e94e03d9afff53a8726339f3076be2c1.r2.cloudflarestorage.com`
- バケット: `burger-stack`

## 接続と Region の扱い

- `Region: "auto"` の PUT: **成功**(623ms)
- path-style アドレッシング: 成功

## レイテンシ(300KiB の物体、日本から)

| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |
|---|---|---|---|---|---|---|---|---|---|
| PUT (書き込み) | 50 | 179.4ms | 71.4ms | 160.2ms | 204.7ms | 409.9ms | 449.4ms | 120.1ms | 449.4ms |
| GET (公開 URL から配信) | 50 | 90.5ms | 36.8ms | 80.9ms | 105.7ms | 165.5ms | 307.4ms | 61.3ms | 307.4ms |
| DELETE (削除) | 50 | 81.4ms | 13.3ms | 77.8ms | 93.0ms | 108.1ms | 140.2ms | 66.3ms | 140.2ms |

- 公開配信: 成功(`https://pub-351ec49d6ceb408eac00864b98da4cca.r2.dev`)
- 転送量の目安: PUT 50 回 + GET 50 回 = 約 29.3 MiB

## ハマった点

(記入)
