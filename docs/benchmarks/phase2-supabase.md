# Phase 2: supabase

- 計測日: 2026-09-21 23:03
- エンドポイント: `https://ssophyxexoulighcurni.storage.supabase.co/storage/v1/s3`
- バケット: `burger-image`

## 接続と Region の扱い

- `Region: "auto"` の PUT: **成功**(1655ms)
- path-style アドレッシング: 成功

## レイテンシ(300KiB の物体、日本から)

| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |
|---|---|---|---|---|---|---|---|---|---|
| PUT (書き込み) | 50 | 483.5ms | 136.3ms | 433.2ms | 511.9ms | 878.0ms | 1034.0ms | 416.2ms | 1034.0ms |
| GET (公開 URL から配信) | 50 | 59.6ms | 106.1ms | 38.5ms | 49.8ms | 97.1ms | 779.9ms | 31.6ms | 779.9ms |
| DELETE (削除) | 50 | 205.8ms | 23.4ms | 199.8ms | 211.3ms | 254.5ms | 329.1ms | 191.3ms | 329.1ms |

- 公開配信: 成功(`https://ssophyxexoulighcurni.supabase.co/storage/v1/object/public/burger-image`)
- 転送量の目安: PUT 50 回 + GET 50 回 = 約 29.3 MiB

## ハマった点

(記入)
