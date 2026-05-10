# Frontend Change Validation Reference

## Boundary Review

Check changed frontend files for these repo-specific rules:

1. Feature behavior should stay under the relevant domain.
2. HTTP and casing conversion should stay at the API boundary.
3. Backend JSON is snake_case; frontend state and props are camelCase.
4. Auth token behavior uses localStorage, Jotai state, and axios Bearer injection.
5. Shared UI should not be duplicated across domains.

## Checks

Run from `frontend/`:

```bash
pnpm run type-check
pnpm run lint
pnpm run test
```

Run build when routing, Vite config, TypeScript config, or API boundary changed:

```bash
pnpm run build
```
