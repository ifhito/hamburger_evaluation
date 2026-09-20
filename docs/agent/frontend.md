# Frontend エージェント向けメモ

React SPA は `frontend/` にあり、pnpm を使う。

## 技術スタック

- React 19 + TypeScript + Vite
- サーバー状態に SWR
- クライアント状態に Jotai
- react-hook-form + Zod
- リクエストは snake_case、レスポンスは camelCase に変換する axios
- ESLint、TypeScript strict、Vitest

## 境界

- `src/app`: router / provider / app shell
- `src/domains/*`: 機能単位のコード
- `src/api`: HTTP クライアントと API 境界
- `src/states`: 共有クライアント状態
- `src/components`: 共有 UI

Backend の payload 名は snake_case、frontend のコードは camelCase に保つこと。
