# Frontend

React + TypeScript + Vite で構成された SPA です。Rails API を呼び出して、ショップ・レビュー・ユーザー画面を提供します。

## Stack

- React 19
- TypeScript
- Vite
- React Router
- TanStack Query
- Storybook

## Architecture

小さめの FSD 構成として、`app`, `pages`, `shared` の 3 レイヤーで組んでいます。

```text
frontend/src/
├── app/
│   ├── providers/
│   ├── router/
│   └── styles/
├── pages/
└── shared/
    ├── lib/
    └── ui/
```

### app

- `AuthProvider`
- Router 定義
- グローバルスタイル

### pages

- shops
- reviews
- signup / signin / signout
- user detail / user update

### shared

- API client
- hooks
- 型定義
- 再利用 UI コンポーネント

## Run Locally

```bash
docker compose up --build
```

App: `http://localhost:5173`

## Useful Commands

```bash
npm run dev
npm run build
npm run storybook
npm run build-storybook
```

Docker を使わずローカルで動かす場合:

```bash
npm install
npm run dev
```

## Routing

- `/shops`
- `/shops/:id`
- `/reviews`
- `/reviews/:id`
- `/reviews/new`
- `/reviews/:id/edit`
- `/signup`
- `/signin`
- `/signout`
- `/users/:id`
- `/users/:id/edit`

## Auth and Data Fetching

- 認証状態は `src/app/providers/AuthProvider.tsx` で管理します。
- API 呼び出しは `src/shared/lib/api.ts` に集約しています。
- 取得系は `useQuery`、更新系は `useMutation` を使っています。

## Notes

- Storybook 用の stories を `src/shared/ui/` に置いています。
- API のベースパスは `/api` 前提です。
- UI はまず素の HTML に近い軽量構成で作っています。
