# PR Self Review Reference

## Scope

This skill reviews the working tree only. It does not decide whether to merge,
close, or rewrite a PR.

## Procedure

1. Inspect `git status --short --branch --untracked-files=all`.
2. Inspect `git diff --check`.
3. Inspect `git diff --stat`.
4. If staged changes exist, inspect `git diff --cached --stat`.
5. Separate files into:
   - intended implementation,
   - tests/checks/docs supporting that implementation,
   - unrelated or pre-existing local files.
6. Report missing validation based on changed area:
   - backend: RSpec, RuboCop, Brakeman,
   - frontend: type-check, lint, test,
   - build/API boundary: frontend build.

## Secret Paths

Never read or include contents from `.env*`, `secrets/**`,
`backend/.kamal/secrets`, or `backend/config/master.key`.
