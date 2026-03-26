---
name: variance_factor の分散計算と2乗の理由
description: ReviewerTrustEvaluator の variance_factor が評価のバラつきを分散で測り、低分散レビュアーにペナルティを与える仕組み
type: project
---

- `(r - mean)**2` の2乗は「プラスマイナスを消す」「大きなズレを強調する」ための統計的標準手法（分散）
- 絶対値でも符号は消えるが、2乗の方が数学的に扱いやすく統計学で標準的
- `var < LOW_VARIANCE_THRESHOLD (0.3)` = 評価が一極集中 → サクラ的と判断
- 低分散の場合 `LOW_VARIANCE_PENALTY = 0.7` を掛けてスコアを下げる
- レビュー数が3未満なら分散計算をスキップして 1.0 を返す（サンプル不足）
- この factor は `base_score * factor` で信頼スコアに乗算される

**主要ファイル:**
- `backend/app/domain/services/reviewer_trust_evaluator.rb` — variance_factor の実装

**Why:** バラつきのない評価（例: 全部5点）は信頼性が低いという設計判断。2乗は統計的分散の定義そのもの。

**How to apply:** LOW_VARIANCE_THRESHOLD を上げるとより厳しく、下げるとより緩い判定になる。評価スケールが変わったら閾値の再調整が必要。
