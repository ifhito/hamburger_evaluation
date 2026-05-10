---
name: calculate_confidence のスコア信頼度計算
description: BurgerScoreCalculator がレビュー件数（60%）と重みの平均（40%）を合算してスコアの信頼度 0〜1 を算出する仕組み
type: project
---

- `review_factor = [count / 10.0, 1.0].min` — 10件で上限1.0、未満は線形増加
- `weight_factor = [total_weight / count.to_f, 1.0].min` — 重みの平均（レビュアーの質）、上限1.0
- `review_factor * 0.6 + weight_factor * 0.4` — 件数60%・質40%の加重平均
- `.clamp(0.0, 1.0)` で念のため範囲保証
- 結果は `BurgerScore.new(confidence: ...)` に格納されて返される

**主要ファイル:**
- `backend/app/domain/services/burger_score_calculator.rb` — calculate_confidence の実装

**Why:** 件数だけだと少数の expert に過信、質だけだと母数不足を見逃す。両方を組み合わせてバランスをとる設計。

**How to apply:** 比率（0.6/0.4）や件数の上限（10件）を変えることで信頼度の感度を調整できる。
