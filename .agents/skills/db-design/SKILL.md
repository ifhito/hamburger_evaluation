---
name: db-design
description: PostgreSQL スキーマの設計・変更(backend-go/db 配下のテーブル、制約、インデックス、マイグレーション)のときに使う。ドメイン/DB 分離ルール — スキーマとドメインは別々に設計し、リポジトリ層でマッピングする — を徹底する。
allowed-tools: [Read, Grep, Glob, Bash(docker compose run:*), Bash(sqlc:*), Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [database, postgresql, schema, migrations, design]
    related_skills: [backend-go-boundaries, focused-review]
---

# DB Design

## 分離ルール(まずこれを読む)

**DB 設計とドメイン設計は仕える主が異なる別々の活動であり、互いを
鏡写しにしてはならない。**

- **スキーマ**はデータ整合性とクエリの形に最適化する: 正規化、制約、
  インデックス、ストレージ効率の良い表現(例: status を smallint + CHECK に)。
- **ドメイン**は振る舞いと不変条件に最適化する: 値オブジェクト、状態機械、
  ルール(例: 遷移メソッドを持つ `ShopStatus`)。
- 両者の**マッピングはリポジトリ層が担う**。sqlc の行構造体を `adapter/` の
  外に出さない。「テーブルにあるから」という理由でドメイン型にフィールドを
  増やさない。スキーマ変更が機械的にドメイン変更を強制してはならず、
  その逆も同様。

片方からもう片方を導出するのは、このリポジトリが意図的に捨てようとしている
Rails の習慣である。両者の形が乖離していくのは設計が機能している証拠であり、
直すべき問題ではない。

## スキーマの原則

1. **制約はスキーマに置く** — NOT NULL、CHECK、UNIQUE、FK。
   アプリケーション側のバリデーションは UX であり、データベースが最後の砦。
   不変条件を制約として表現できるなら、そちらにも表現する。
2. **すべての FK にインデックスを張る**。加えて実際の `WHERE`/`ORDER BY` が
   必要とするものも。当て推量のインデックスは禁止 — 1 本ごとに全書き込みに税を課す。
3. **退屈な型を選ぶ** — bigint の id、timestamptz、text、閉じた列挙には
   smallint + CHECK。jsonb や配列に手を伸ばすのは理由を明示できるときだけ。
4. **論理削除はカラム(`discarded_at`)で行い**、すべての読み取りパスが
   削除済み行を見るかどうかを明示的に決める。
5. **命名**: snake_case、テーブルは複数形、FK は `<table>_id`、
   結合テーブルはアルファベット順の `a_b`。

## マイグレーションのルール

- マイグレーションは `backend-go/db/migrations` の素の SQL で、up/down を
  対にし、スキーマの単一の source of truth とする。
- まず追加から: バックフィルと `NOT NULL`/制約の強化はカラム作成とは
  別ステップにする — 不可逆なマイグレーション 1 本にまとめない。
- 破壊的変更(カラム/テーブルの削除)は独立したマイグレーションにし、
  ユーザーの明示的な判断を得る。
- スキーマまたはクエリの変更後は必ず: `sqlc generate` を実行し、出力を
  コミットし、`git diff --exit-code` でドリフトゼロを確認する。

## 設計チェックリスト(マイグレーション PR の前に)

- [ ] 各不変条件に制約があるか、なければ理由が書かれている。
- [ ] FK にインデックスがある。クエリ駆動のインデックスは対象クエリにちなんで命名した。
- [ ] up/down の両方を空の DB に対してテストした(ストーリーの AC パターン参照)。
- [ ] ドメインモデルはこれらのテーブルからではなく振る舞いから設計した —
      マッピングは `adapter/repository` にある。
- [ ] データの移動やバックフィルがある場合、`focused-review` の consistency
      レンズでレビューした。
