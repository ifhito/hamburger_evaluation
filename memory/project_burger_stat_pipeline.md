---
name: weighted_score/confidence の保存と利用の流れ
description: レビュー作成/削除 → 非同期ジョブ → BurgerStat テーブルに upsert → API レスポンスに含まれるパイプライン
type: project
---

- Review の after_create/after_discard で BurgerStatUpdateJob が非同期起動（review.rb:14-15）
- ジョブが BurgerStat.recalculate_for(burger) を呼び BurgerScoreCalculator でスコア計算
- burger_stats テーブル（burger_id UNIQUE）に weighted_score/confidence を upsert
- ReviewSerializer#burger_json が stat から weighted_score/confidence を取り出して API レスポンスに含める（review_serializer.rb:32-33）
- BurgerScore#reliable? (confidence >= 0.6) は現状未使用だが将来の警告表示用に定義済み

**主要ファイル:**
- `backend/app/models/review.rb` — コールバックでジョブをキック
- `backend/app/jobs/burger_stat_update_job.rb` — 非同期ジョブ
- `backend/app/models/burger_stat.rb` — recalculate_for で upsert
- `backend/app/serializers/review_serializer.rb` — API レスポンスに含める
- `backend/app/domain/values/burger_score.rb` — reliable? メソッド定義

**Why:** 毎リクエストでの重いスコア計算を避けるため事前計算してキャッシュ。非同期ジョブにすることでレスポンス速度を維持。

**How to apply:** weighted_score/confidence を使う新機能を作るときは burger_stats テーブルから読むだけでよい。スコアロジックを変えたときは全バーガーに対して再計算ジョブを流す必要がある。
