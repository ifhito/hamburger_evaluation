# Frontend Agent Notes

React SPA lives in `frontend/` and uses pnpm.

## Stack

- React 19 + TypeScript + Vite
- SWR for server state
- Jotai for client state
- react-hook-form + Zod
- axios with request snake_case and response camelCase conversion
- ESLint, TypeScript strict, Vitest

## Boundaries

- `src/app`: router/providers/app shell
- `src/domains/*`: feature code
- `src/api`: HTTP client and API boundary
- `src/states`: shared client state
- `src/components`: shared UI

Keep backend payload names snake_case and frontend code camelCase.
