# Memory Index

- [recency_factor の指数減衰（half-life decay）](project_recency_factor_decay.md) — BurgerScoreCalculator の半減期ベース重み計算。180日で重みが半分になる指数減衰式の解説
- [ReviewerTrust::LEVELS による信頼スコア判定](project_reviewer_trust_levels.md) — select+values.last パターンで階段状しきい値を判定し base_score を返す仕組み
- [variance_factor の分散計算と2乗の理由](project_variance_factor.md) — 評価のバラつきを分散で測り低分散レビュアーにペナルティを与える仕組み
- [calculate_confidence のスコア信頼度計算](project_calculate_confidence.md) — レビュー件数60%・重みの平均40%でスコアの信頼度 0〜1 を算出する仕組み
- [weighted_score/confidence の保存と利用の流れ](project_burger_stat_pipeline.md) — レビュー作成/削除→非同期ジョブ→BurgerStat upsert→APIレスポンス までのパイプライン
