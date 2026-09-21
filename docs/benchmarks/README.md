# 計測結果

[検証手順書](../deploy-verification-runbook.md) に従って測った結果を置く。

| ファイル | 内容 |
|---|---|
| `raw/*.csv` | `scripts/bench/*.sh` が出力する生データ。編集しない |
| `phase1-database.md` | DB 4 候補の結果 |
| `phase2-photo.md` | 写真ストレージ 4 候補の結果 |
| `phase3-email.md` | メール送信 4 候補の結果 |
| `phase4-api.md` | API 4 候補の結果 |
| `phase5-async.md` | 非同期処理 4 候補の結果 |
| `phase6-frontend.md` | フロントエンド 4 候補の結果 |
| `summary.md` | 24 候補の総まとめ。記事の最終回に対応 |

数字は必ず計測日時とセットで書く。無料枠の条件もサービス性能も変わるため、
「いつ時点の値か」がないと再利用できない。
