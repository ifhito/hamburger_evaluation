---
name: pr-hygiene
description: hamburger_evaluation でコミットの準備、PR の更新、差分レビュー、Claude/Codex/Hermes の作業調整をするときに使う。
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [git, pr, review, hygiene]
    related_skills: [backend-go-boundaries, frontend-spa-boundaries]
---

# PR Hygiene

## 概要

ステージング、コミット、プッシュ、PR 更新の前にこのスキルを使う。リポジトリには
無関係なローカルファイルが存在しうる。エージェントの変更は明示的かつ
スコープ内に保つこと。

## 使いどころ

- `git add`、`git commit`、`git push` の前
- PR 本文の編集や PR クローズの前
- 差分レビューのとき
- Claude Code、Codex、Hermes Agent の作業を調整するとき

## ワークフロー

```bash
git status --short --branch --untracked-files=all
git diff --check
git diff --stat
```

明示したパスだけをステージする。ユーザーが求めない限り、`SETUP.md` や
`plans/*.md` などの無関係なローカルファイルを含めない。

## シークレット

以下は決して読まない・ステージしない・要約しない・コミットしない:

- `.env*`(どのディレクトリでも)
- `secrets/**`

## PR サマリの形

PR 本文は [[pr-template]] スキルに従う: 日本語で、
概要 / 関連 Issue / 変更内容 / テスト (実測のみ) / レビュー観点 / 備考 の構成。

## よくある落とし穴

1. 無関係な変更済みファイルをステージする。
2. フルスイートが必要な場面で、絞ったテストだけを十分と報告する。
3. ユーザーの承認なしに PR をクローズ・編集する。
4. エージェントが生成したファイルを未検証のまま残す。

## 検証チェックリスト

- [ ] `git status --short --branch` を確認した。
- [ ] `git diff --check` が通る。
- [ ] 意図したパスだけがステージ/コミットされている。
- [ ] PR 本文に実際に実行したコマンドが含まれている。
