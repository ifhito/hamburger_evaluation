---
name: frontend-change-validation
description: フロントエンドの React の挙動が変わったときに使う。型、lint、テストを検証する。
allowed-tools: [Read, Grep, Glob, Bash(pnpm run:*)]
version: 1.0.0
author: BurgerStack Agents
license: MIT
metadata:
  hermes:
    tags: [react, frontend, validation]
    related_skills: [pr-self-review]
---

# Frontend Change Validation

## 概要

フロントエンドの挙動、API 呼び出し、state、ルーティング、ビルド設定が
変わったときにこのスキルを使う。仕事は TypeScript、lint、ユニットテスト、
必要に応じてビルドを検証すること。

## 使いどころ

- React のページ、フック、API クライアント、フォーム、state を変更した後。
- ルートや Vite/TypeScript の設定を変更した後。
- フロントエンドの作業を完了と報告する前。

## 手順

1. `references/frontend-change-validation.md` を読む。
2. 次のスクリプトを実行する:

```bash
.agents/skills/frontend-change-validation/scripts/frontend-checks.sh
```

3. ルーティング/ビルド/API 境界が変わった場合は、`frontend/` から `pnpm run build` も実行する。

## 出力

コマンドごとに pass/fail を返す。失敗した場合は、失敗したファイル/テストと
最短で対処可能なエラーを含める。

## よくある落とし穴

1. snake_case/camelCase 変換をフィーチャーコードで重複させない。
2. `frontend/.env*` を読まない。
3. TypeScript の変更で type-check をスキップしない。

## 検証チェックリスト

- [ ] 型チェックが通った。
- [ ] ESLint が通った。
- [ ] Vitest が通った。
- [ ] ルート/ビルド/API 境界が変わった場合、ビルドを実行した。
