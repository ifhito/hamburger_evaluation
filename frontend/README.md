# Frontend

React 19 + TypeScript + Vite で構成された SPA です。Rails API を呼び出して、ショップ・レビュー・ユーザー画面を提供します。

## Stack

- React 19
- TypeScript (strict)
- Vite
- React Router v6
- SWR (データフェッチ)
- Jotai (認証状態グローバル管理)
- react-hook-form + Zod (フォームバリデーション)
- axios (camelcase-keys / snakecase-keys で HTTP 境界の命名変換)
- ESLint + typescript-eslint
- Vitest (ユニットテスト)
- Storybook (コンポーネント開発)

## Architecture

ドメインごとにコードを整理する `domains/<name>` 構成です。

```text
src/
├── app/
│   ├── router/           # React Router — ProtectedRoute / GuestRoute
│   │   ├── index.tsx
│   │   ├── ProtectedRoute.tsx
│   │   └── GuestRoute.tsx
│   └── App.tsx           # JotaiProvider > AuthProvider > RouterProvider
├── domains/
│   ├── auth/
│   │   ├── api/          # authApiClient.ts
│   │   ├── hooks/        # useAuthForm.ts (Zod + react-hook-form)
│   │   ├── pages/        # SigninPage / SignupPage / SignoutPage
│   │   ├── AuthProvider.tsx  # Context + useAuth hook
│   │   ├── storage.ts    # localStorage 操作
│   │   └── types.ts
│   ├── reviews/
│   │   ├── api/          # reviewApiClient.ts, types.ts (camelCase)
│   │   ├── hooks/        # useReviews / useReview / useReviewMutations / useReviewForm
│   │   └── pages/        # ReviewListPage / ReviewDetailPage / ReviewNewPage / ReviewEditPage
│   ├── shops/
│   │   ├── api/
│   │   ├── hooks/
│   │   └── pages/        # ShopListPage / ShopDetailPage
│   └── users/
│       ├── api/
│       ├── hooks/
│       └── pages/        # UserDetailPage / UserUpdatePage
├── api/client/
│   └── buildApiClient.ts # axios factory (interceptors: camelcase / snakecase 変換, Bearer token)
├── states/
│   └── authAtom.ts       # Jotai atoms: authUserAtom, authTokenAtom
└── components/           # 共有 UI: Button, Input, Textarea, RatingSelect, Layout
```

## Run Locally

```bash
docker compose up --build
```

App: `http://localhost:5173`

Docker を使わずローカルで動かす場合:

```bash
pnpm install
pnpm run dev
```

## Quality Checks

```bash
pnpm run lint          # ESLint
pnpm run type-check    # tsc --noEmit
pnpm run test          # Vitest
pnpm run build         # プロダクションビルド (型チェック込み)
```

## Routing

| パス | 認証 | ページ |
|------|------|--------|
| `/shops` | public | ShopListPage |
| `/shops/:id` | public | ShopDetailPage |
| `/reviews` | public | ReviewListPage |
| `/reviews/:id` | public | ReviewDetailPage |
| `/reviews/new` | 要認証 | ReviewNewPage |
| `/reviews/:id/edit` | 要認証 | ReviewEditPage |
| `/signup` | ゲストのみ | SignupPage |
| `/signin` | ゲストのみ | SigninPage |
| `/signout` | 要認証 | SignoutPage |
| `/users/:id` | public | UserDetailPage |
| `/users/:id/edit` | 要認証 | UserUpdatePage |

## Auth and Data Fetching

- 認証状態は `src/domains/auth/AuthProvider.tsx` で管理し、Jotai atom (`authUserAtom`, `authTokenAtom`) でグローバル共有します。
- API クライアントは `src/api/client/buildApiClient.ts` の axios factory を使います。
  - リクエスト: snakecase-keys で自動変換 + `Authorization: Bearer <token>` を付与
  - レスポンス: camelcase-keys で自動変換
- データ取得は SWR (`useSWR`)、更新は SWR mutation (`useSWRMutation`) を使います。

## Notes

- Storybook 用の stories は `src/shared/ui/` に置いています。
- API のベースパスは `/api` 前提です。Vite proxy で `http://localhost:3000` へ転送します。
- React コンポーネント内ではすべてのフィールド名が camelCase になります（変換は HTTP 境界で完結）。
