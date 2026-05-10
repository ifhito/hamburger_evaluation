---
name: recency_factor の指数減衰（half-life decay）
description: BurgerScoreCalculator の recency_factor メソッドが使う半減期ベースの指数減衰式の仕組み
type: project
---

- `Math.exp(-days_ago * Math.log(2) / RECENCY_HALF_LIFE_DAYS)` は `2^(-days_ago / 180)` と等価
- `RECENCY_HALF_LIFE_DAYS = 180` → 180日ごとにレビューの重みが半分になる
- 0日前=1.0、180日前=0.5、360日前=0.25 というグラフ
- `Math.log(2)` = ln(2) ≈ 0.693 は「2のべき乗に変換するための係数」
- `weight_for(review)` で信頼スコア（ReviewerTrustEvaluator）と掛け合わされて最終重みになる

**主要ファイル:**
- `backend/app/domain/services/burger_score_calculator.rb` — 重み付きスコア計算・recency_factor・confidence 計算

**Why:** 古いレビューを急に切り捨てず、滑らかな曲線で緩やかに評価を下げる設計。半減期180日は放射性崩壊と同じ数学的構造。

**How to apply:** half-life を変えたいなら `RECENCY_HALF_LIFE_DAYS` 定数を変更するだけでOK。値を小さくすると最近のレビューをより重視する。
