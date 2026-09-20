# エージェントのワークフロー

## 編集前

```bash
git status --short --branch
```

変更のある領域を特定し、無関係なローカルファイルを避ける。

## チェック

Backend:

```bash
cd backend
docker compose run --rm -e RAILS_ENV=test api bundle exec rspec
docker compose run --rm api bin/rubocop -f github
docker compose run --rm api bin/brakeman --no-pager
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

`.env*`、`secrets/**`、`backend/.kamal/secrets`、`backend/config/master.key` を読んだり含めたりしてはならない。
