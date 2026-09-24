# 計測結果

[検証手順書](../deploy-verification-runbook.md) に従って測った結果を置く。

| ファイル | 内容 |
|---|---|
| `raw/*.csv` | `scripts/bench/*.sh` が出力する生データ。編集しない |
| `phase0-baseline.md` | 準備と基準値(本番用イメージ、写真の OOM) |
| `phase1-<社名>.md` | DB 各社の詳細 |
| `phase1-summary.md` | DB 4 候補の総括 |
| `phase1-candidates.md` | DB 候補の解説と、選ばなかった理由 |
| `phase2-<社名>.md` | 写真ストレージ各社の詳細 |
| `phase2-summary.md` | 写真ストレージ 4 候補の総括 |
| `phase2-candidates.md` | 写真ストレージ候補の解説と、選ばなかった理由 |
| `phase3-<社名>.md` | メール送信各社の詳細 |
| `phase3-summary.md` | メール送信 4 候補の総括 |
| `phase3-candidates.md` | メール送信候補の解説と、選ばなかった理由 |
| `phase4-api.md` | API 4 候補の結果 |
| `phase5-async.md` | 非同期処理 4 候補の結果 |
| `phase4-<社名>.md` | API 各社の詳細 |
| `phase4-summary.md` | API 4 候補の総括 |
| `phase6-summary.md` | フロントエンド 4 候補の総括 |
| `phase6-candidates.md` | フロントエンド候補の解説と、選ばなかった理由 |
| `summary.md` | 24 候補の総まとめ。記事の最終回に対応 |

数字は必ず計測日時とセットで書く。無料枠の条件もサービス性能も変わるため、
「いつ時点の値か」がないと再利用できない。
