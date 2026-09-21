# Phase 2: tigris

- 計測日: 2026-09-21 23:03
- エンドポイント: `https://t3.storage.dev`
- バケット: `burger-image`

## 接続と Region の扱い

- `Region: "auto"` の PUT: **成功**(313ms)
- path-style アドレッシング: 成功

## レイテンシ(300KiB の物体、日本から)

| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |
|---|---|---|---|---|---|---|---|---|---|
| PUT (書き込み) | 50 | 166.7ms | 191.4ms | 89.2ms | 322.6ms | 560.1ms | 1047.0ms | 48.0ms | 1047.0ms |
| GET (公開 URL から配信) | 50 | 34.8ms | 19.3ms | 26.0ms | 61.0ms | 77.1ms | 115.5ms | 22.7ms | 115.5ms |
| DELETE (削除) | 50 | 23.9ms | 6.9ms | 22.3ms | 25.7ms | 29.0ms | 65.8ms | 18.7ms | 65.8ms |

- 公開配信: 成功(`https://burger-image.t3.tigrisfiles.io`)
- 転送量の目安: PUT 50 回 + GET 50 回 = 約 29.3 MiB

## ハマった点

(記入)
