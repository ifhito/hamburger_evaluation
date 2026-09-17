---
name: pr-template
description: Use when creating or updating any pull request description in this repo. PR bodies are written in Japanese with a fixed section structure; evidence-backed test results are mandatory.
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*), Bash(git log:*), Bash(gh pr:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [git, pr, template, japanese]
    related_skills: [pr-hygiene, github-story]
---

# PR Template

## Rules

1. **PR タイトルと本文は日本語で書く**(コード識別子・コマンド・パスは原文のまま)。
2. セクション構成は下のテンプレートに従う。空になるセクションは見出しごと削除してよい(空欄のまま残さない)。
3. **テスト欄には実際に実行したコマンドと結果だけを書く**。実行していない検証を書かない。スキップしたものは理由付きで明記する。
4. ストーリー issue から作る PR は必ず `Closes #<issue>` を関連 Issue 欄に入れる。
5. 規模ゲートで「大」判定の PR は、レビュー観点欄に**推奨レビュー順**(どのファイルからどの順で読むか)を書く。
6. 末尾の attribution(Generated with Claude Code / セッションリンク)はハーネスの規約どおり付与する。

## Template

```markdown
## 概要

<!-- 何を・なぜ。1〜3行。ユーザー視点またはアーキテクチャ視点の変化を書く -->

## 関連 Issue

Closes #<番号>

## 変更内容

- <!-- 意図ごとに箇条書き。ファイル列挙ではなく変更の意味を書く -->

## テスト

<!-- 実行したコマンド + 結果(pass/fail)。実測のみ。 -->
- `go-checks.sh` → exit 0
- `docker compose up` スモーク: /up → 200

## レビュー観点

<!-- レビュアーに最初に見てほしい箇所、設計判断、リスク。大きい PR は推奨レビュー順 -->

## 備考

<!-- マイグレーション有無、先送り事項(P3等)、フォローアップ、既知の制約 -->
```

## When Updating an Existing PR

- 追加コミットで内容が変わったら本文も追従させる(テスト欄に最新の実測を追記)。
- レビュー往復で決まったこと(採用した修正方針・棄却理由)は備考に残す。
