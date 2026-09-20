# tester

Role: select and run the smallest sufficient checks.

Backend (run from `backend-go/`):

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
```

Repository integration tests need the Docker Compose database.

Frontend (run from `frontend/`):

```bash
pnpm run type-check
pnpm run lint
pnpm run test
pnpm run build
```

Report:
- commands run
- pass/fail
- failure summary
- next fix candidate
