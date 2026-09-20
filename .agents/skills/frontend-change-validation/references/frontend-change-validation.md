# Frontend Change Validation リファレンス

## 境界レビュー

変更されたフロントエンドファイルを、このリポジトリ固有のルールで確認する:

1. フィーチャーの挙動は該当ドメインの配下にとどめる。
2. HTTP とケーシング変換は API 境界にとどめる。
3. バックエンドの JSON は snake_case、フロントエンドの state と props は camelCase。
4. 認証トークンの挙動は localStorage、Jotai の state、axios の Bearer 注入を使う。
5. 共有 UI をドメイン間で重複させない。

## チェック

`frontend/` から実行する:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
```

ルーティング、Vite 設定、TypeScript 設定、API 境界が変わった場合はビルドを実行する:

```bash
pnpm run build
```
