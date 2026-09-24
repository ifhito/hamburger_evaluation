# Phase 2: b2

- 計測日: 2026-09-21 23:09
- エンドポイント: `https://s3.us-east-005.backblazeb2.com`
- バケット: `burger-image`

## 接続と Region の扱い

- `Region: "auto"` の PUT: **成功**(1295ms)
- path-style アドレッシング: 成功

## レイテンシ(300KiB の物体、日本から)

| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |
|---|---|---|---|---|---|---|---|---|---|
| PUT (書き込み) | 50 | 262.0ms | 63.0ms | 233.2ms | 327.0ms | 417.7ms | 556.1ms | 217.8ms | 556.1ms |
| GET (公開 URL から配信) | 50 | 284.2ms | 8.5ms | 280.7ms | 292.6ms | 309.4ms | 315.5ms | 278.4ms | 315.5ms |
| DELETE (削除) | 50 | 197.3ms | 52.1ms | 185.6ms | 210.6ms | 244.5ms | 542.5ms | 176.3ms | 542.5ms |

- 公開配信: 成功(`https://f005.backblazeb2.com/file/burger-image`)
- 転送量の目安: PUT 50 回 + GET 50 回 = 約 29.3 MiB

## ハマった点

(記入)
