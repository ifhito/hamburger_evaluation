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
- **コード内の文章は日本語で書く**: コメント(`//`、`/* */`、JSDoc)とテスト名(`it("…")` /
  `describe("…")` の文字列)は日本語。識別子、API の JSON キー、ログ文言は英語のまま。
  画面に表示する文言は i18n(`src/locale`)で管理し、この規約の対象外。
  既存の英語コメントは、その行を触るときに日本語へ直す。

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
5. コメントやテスト名を英語で書く(上記の例外を除き日本語で書く)。

## 検証チェックリスト

- [ ] TypeScript の変更で型チェックが通る。
- [ ] フロントエンドの変更で lint が通る。
- [ ] 変更した挙動を Vitest がカバーしている。
- [ ] ルート/ビルドの変更でプロダクションビルドが通る。
