# エージェントのワークフロー

## 編集前

```bash
git status --short --branch
```

変更のある領域を特定し、無関係なローカルファイルを避ける。

## チェック

Backend(リポジトリのルートから):

```bash
.agents/skills/backend-go-change-validation/scripts/go-checks.sh
cd backend-go && docker compose run --rm sqlc generate   # db/queries/ を変更したとき
```

Frontend:

```bash
cd frontend
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

## シークレット

`.env*`(`backend-go/.env*`、`frontend/.env*` を含む)と `secrets/**`、および旧 API(`backend/`)の名残として手元に残りうる秘密ファイルを読んだり含めたりしてはならない。
