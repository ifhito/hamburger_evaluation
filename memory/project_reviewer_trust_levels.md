---
name: ReviewerTrust::LEVELS による信頼スコア判定
description: select + values.last/.keys.last パターンで階段状しきい値を判定し base_score またはレベル名を返す仕組み
type: project
---

- `LEVELS` は newcomer/regular/veteran/expert の4段階、min_reviews と base_score をペアで持つ
- `select { |_, v| count >= v[:min_reviews] }` で条件を満たすレベルだけ残す
- `.values.last[:base_score]` で「満たした中で最も上位のレベル」の base_score を取得
- Ruby の Hash はキー挿入順を保持するため、昇順に定義されていることが前提
- `min_reviews: 0` の newcomer は常に条件を満たすため最低でも 0.5 が返る
- 同じパターンが `determine_level` でも使われており `.keys.last` でレベル名を取得

**主要ファイル:**
- `backend/app/domain/values/reviewer_trust.rb` — LEVELS 定数の定義
- `backend/app/domain/services/reviewer_trust_evaluator.rb` — base_score/determine_level の実装

**Why:** if-elsif や case 文を使わず、LEVELS 定数の定義を変えるだけで判定ロジックも追従する柔軟な設計。

**How to apply:** レベルを追加・変更するときは LEVELS の順序（min_reviews 昇順）を崩さないこと。逆順にすると `.values.last` が常に newcomer を返す壊れた動作になる。
