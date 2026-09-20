---
name: frontend-spa-boundaries
description: hamburger_evaluation の React SPA(フロントエンドの API クライアント、ドメインフック、ページ、フォーム、state、フロントエンドテスト)を変更するときに使う。
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [react, frontend, typescript, testing]
    related_skills: [pr-hygiene]
---

# Frontend SPA Boundaries

## 概要

`frontend/` 配下のフロントエンド変更にはこのスキルを使う。SPA は React、
TypeScript、Vite、SWR、Jotai、react-hook-form、Zod を使い、axios による
ケーシング変換を HTTP 境界で行う。

## 使いどころ

- `frontend/src` やフロントエンドテストを編集するとき
- API リクエスト/レスポンス処理を変更するとき
- ドメインフック、ページ、フォーム、共有 UI を追加するとき

## ルール

- フィーチャーコードは `src/domains/*` 配下に置く。
- ルーター/プロバイダー/アプリシェルは `src/app` 配下に置く。
- HTTP とケーシング変換は `src/api` 配下に置く。
- バックエンドのペイロード名は snake_case、フロントエンドのコードは camelCase。
- 認証は localStorage、Jotai の state、Authorization Bearer トークン注入を使う。

## コマンド

`frontend/` から実行する:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

## よくある落とし穴

1. API のケーシング変換をフィーチャーコードで重複させる。
2. バックエンドの snake_case をフロントエンドの state の形として扱う。
3. 複数ドメインが必要とする共有 UI を 1 つのドメイン内に追加する。
4. `frontend/.env*` を読む。

## 検証チェックリスト

- [ ] TypeScript の変更で型チェックが通る。
- [ ] フロントエンドの変更で lint が通る。
- [ ] 変更した挙動を Vitest がカバーしている。
- [ ] ルート/ビルドの変更でプロダクションビルドが通る。
