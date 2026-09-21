---
name: pr-self-review
description: PR を準備するときに使う。人間に依頼する前にスコープを絞ったセルフレビューを実行する。
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: BurgerStack Agents
license: MIT
metadata:
  hermes:
    tags: [pr, review, git]
    related_skills: [backend-go-change-validation, frontend-change-validation]
---

# PR Self Review

## 概要

PR 更新や人間へのレビュー依頼の前に、現在のワーキングツリーをレビューする
ためにこのスキルを使う。仕事は、無関係な変更、リスクの高い差分、検証
エビデンスの欠落を見つけること。

## 使いどころ

- PR を開く・更新する前。
- 生成された変更のレビューをユーザーに頼む前。
- エージェントが生成した作業をステージ・コミットする前。

## 手順

1. `references/pr-self-review.md` を読む。
2. 次のスクリプトを実行する:

```bash
.agents/skills/pr-self-review/scripts/pr-self-review.sh
```

3. 以下だけを要約する:
   - 意図ごとにグループ化した変更ファイル、
   - 無関係の可能性があるファイル、
   - 不足しているチェック、
   - 主要なレビューリスク。

## 出力

最大 5 個の箇条書きで返す。各項目に重大度と、該当する場合はファイルパスを付ける。

## よくある落とし穴

1. このスキルの一部としてファイルをステージしない。
2. このスキルの一部として PR 本文を編集しない。
3. シークレットや env ファイルを読まない。

## 検証チェックリスト

- [ ] git status と diff check を確認した。
- [ ] 無関係なファイルを指摘した。
- [ ] 必要なチェックを実際のエビデンスに基づいて列挙した。
