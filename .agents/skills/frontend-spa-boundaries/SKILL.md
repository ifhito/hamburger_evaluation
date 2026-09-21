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
TypeScript、Vite、SWR、Jotai、react-hook-form を使い、axios による
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
  `it` / `describe` の名前とコメントは、story を知らない人に伝わる書き方にする(番号を書かず、
  「状況 → 結果」を具体的に書き、意味の伝わりにくい英語を混ぜない。詳細は `backend-go-boundaries` の
  「読む人に伝わる書き方」)。

## ドメインのルールは backend だけが持つ

frontend の責務は、**入力・説明・表示・サーバーのエラーの表示・導線**だけである。
ドメインのルール(何が有効か、誰に何が許されるか、何を返すか)の判断は backend の
`domain` だけが持ち、frontend に複製しない。片方だけ直して食い違う二重管理を防ぐため。

- **持たないもの**:
  - 検証(必須・形式・範囲・長さ・文字種。例: パスワードの強度、email の形式、rating の範囲)。
    違反はサーバーの 422 のメッセージを表示する。
  - 権限の判断(誰が編集・削除できるか。例: `review.userId === authUser.id` の比較)。
    backend が返す値(`can_edit` など)で出し分ける。
  - 計算・導出(平均・スコア・「最終ページか」の判定)。backend が返した値をそのまま表示する。
  - backend と同じ定数の複製(ページサイズ、上限、状態の一覧など)。描画に要る値(rating の範囲など)は、
    backend が `GET /meta` で返し、frontend はそれを使う。ログイン状態の復元も、JWT を decode せず、
    `GET /me` で backend に問い合わせる。
- **許すもの**: 入力の種類を表す HTML の属性(`type="email"` はキーボードの最適化のために付けてよいが、
  ブラウザの標準検証が backend より先に送信を止めないよう、form には `noValidate` を付ける)、
  表示のための整形(日付、小数点以下の桁)、ルートの導線としての redirect(権威は backend。
  管理画面に入れるかは、backend が返す `can_moderate` で決め、`admin` の値から権限を導かない)、
  backend が返した値による出し分け、規則を利用者に伝える説明文。説明文に数値を書くときは、対応する backend の定数と、
  変えるときに直すことをコメントに書く。
- 要件が frontend でルールを検証することを求めていたら、実装せず、判断を backend に置く形に
  直す(story の見直しを提案する)。

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
5. コメントやテスト名を英語で書く(上記の例外を除き日本語で書く)。story・受け入れ条件の番号を入れる。
6. frontend にドメインのルールを複製する(Zod などによる検証、権限の条件、backend と同じ定数、
   backend の値の再計算)。判断は backend に置き、frontend は結果を表示する。

## 検証チェックリスト

- [ ] frontend にドメインのルールの判断(検証・権限の条件・定数・導出)を足していない。
      backend が返した値と、サーバーのエラーを表示している。
- [ ] TypeScript の変更で型チェックが通る。
- [ ] フロントエンドの変更で lint が通る。
- [ ] 変更した挙動を Vitest がカバーしている。
- [ ] ルート/ビルドの変更でプロダクションビルドが通る。
