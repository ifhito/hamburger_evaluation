# Harness Debt

Updated: 2026-05-10 17:48 JST
Window: last 30 days of git history and available PR comments
Sources:
- `git log --since='30 days ago' --all`
- `gh pr list --state all --limit 50 --json ...`
- `gh pr view 3 --json body,comments,reviews,statusCheckRollup`

## Summary

The strongest repeated pattern is not code style drift; it is agent workflow drift around PR consolidation, validation evidence, and backend/frontend boundary assumptions. PR comments are sparse, so the list below combines explicit PR evidence with commit-level evidence from recent work.

## AGENTS.md size check

- Lines: 74
- Sections: 6
- Bullet/numbered instruction count estimate: 19
- Status: below the 200-instruction ceiling; no refactor proposal required now.

## Candidate debts and proposed mitigations

### 1. Superseded PRs stayed open after a consolidated PR existed

Evidence:
- PR #2 was closed because PR #3 superseded it.
- PR #1 was closed because its authorization work was absorbed into PR #3.

Impact:
- Review attention splits across stale/failing PRs.
- Agents may continue fixing or describing the wrong branch.

Preferred mitigation:
- **Sensor**: add a PR-consolidation check script that lists open PRs, fetches PR refs, and reports ancestor/superset relationships.

Implementation option:
- Add `.agents/skills/pr-self-review/scripts/pr-consolidation-check.sh`.
- Add a `pr-self-review` reference step to run it before PR cleanup.

Priority: High
Status: proposed, not implemented

### 2. Consolidated PR initially missed tests from an absorbed PR

Evidence:
- Commit `12f68bf test: add policy specs` was added after PR #1 policy coverage was identified as useful and absorbed into PR #3.
- PR #3 body explicitly says it absorbed Pundit policy unit specs from #1.

Impact:
- Implementation can be merged while older PR-only test coverage is lost.

Preferred mitigation:
- **Sensor**: when closing a superseded PR, compare test files changed in the closing PR against the surviving PR and fail if unique specs are not present.

Implementation option:
- Add script `.agents/skills/pr-self-review/scripts/absorbed-pr-test-check.sh <old_pr> <surviving_pr>`.
- Keep it manual because it needs PR numbers.

Priority: High
Status: proposed, not implemented

### 3. Partial RSpec success was easy to misread because SimpleCov exits non-zero

Evidence:
- PR #3 Test Plan notes targeted policy specs passed examples but exited non-zero due to SimpleCov global coverage.

Impact:
- Agents can report tests as failed or passed incorrectly unless full-suite context is included.

Preferred mitigation:
- **Hook/Sensor**: parse targeted RSpec output and print a special note when failures are zero but SimpleCov caused the exit.

Implementation option:
- Update `.claude/hooks/stop-sensors.py` or backend check script to recognize this pattern and require full RSpec before final status.

Priority: Medium
Status: proposed, not implemented

### 4. Backend boundary refactor needed multiple follow-up passes to move persistence access

Evidence:
- Commit `d2ba217` clarified backend domain boundaries.
- Commit `42d02f2` later moved persistence access behind repositories.

Impact:
- Agents can stop after moving files without removing direct ActiveRecord access from controllers/jobs/domain paths.

Preferred mitigation:
- **Deterministic Sensor**: add a repository architecture grep that blocks direct ActiveRecord calls in selected boundary files.

Implemented:
- `.claude/hooks/stop-sensors.py` now scans changed `backend/app/controllers/`, `backend/app/jobs/`, and `backend/app/domain/` files for persistence-looking calls such as `.where(`, `.find(`, `.save!(`, `.update!(`, `.discard!(`, and `.upsert(`.

Priority: High
Status: implemented

### 5. Duplicate backend CI files existed under `backend/.github`

Evidence:
- Commit `61a3cd2 Fix proxy error handling and remove duplicate backend CI files` removed duplicate backend CI files.

Impact:
- Agents may edit or create nested CI workflows that GitHub will not run from the repo root.

Preferred mitigation:
- **Sensor**: fail if `.github` directories exist outside repo root.

Implementation option:
- Add to `.claude/hooks/stop-sensors.py`: detect `*/.github/workflows/*.yml` outside root `.github/workflows`.

Priority: Medium
Status: proposed, not implemented

### 6. Frontend proxy/API boundary required correction after initial implementation

Evidence:
- Commit `61a3cd2` fixed proxy error handling and touched `frontend/src/api/client/buildApiClient.ts`, domain hooks, and `frontend/vite.config.ts`.

Impact:
- API error shape and Vite proxy assumptions can drift across hooks.

Preferred mitigation:
- **Test/type Sensor**: require frontend type-check, lint, unit tests, and build when API client or Vite config changes.

Implemented:
- `.claude/hooks/stop-sensors.py` now explicitly marks frontend build escalation active for `frontend/src/api/client/`, `frontend/vite.config*`, and `frontend/tsconfig*` changes. The existing frontend sensor runs `pnpm run build` for frontend changes.

Priority: Medium
Status: implemented

### 7. SETUP.md and actual auth architecture diverged

Evidence:
- Existing `AGENTS.md` documents custom JWT Bearer auth despite `SETUP.md` assumptions around `devise_token_auth`.

Impact:
- Agents may follow stale setup instructions and generate incompatible auth changes.

Preferred mitigation:
- **AGENTS.md one-line guard**: already present in Conventions.

Implementation option:
- No change unless future mistakes repeat; current AGENTS.md line is enough.

Priority: Low
Status: monitor

### 8. Local unrelated files remain easy to mix into agent commits

Evidence:
- Current working tree contains pre-existing modified/untracked files such as `AGENT.md`, `CLAUDE.md`, `SETUP.md`, and `plans/*.md`.
- Prior PR cleanup required explicit path staging to avoid unrelated local files.

Impact:
- Agents may stage unrelated docs/plans with harness or code changes.

Preferred mitigation:
- **Hook/Sensor**: Stop hook already prints `git status`; add stricter detection for known out-of-scope paths when they are staged.

Implemented:
- `.claude/hooks/stop-sensors.py` now fails if `SETUP.md`, `plans/*`, `memory/*`, or `plan/*` are staged without `AGENT_ALLOW_OUT_OF_SCOPE_STAGED=1`.

Priority: Medium
Status: implemented

## Recommended implementation set for this session

Choose any subset to implement:

1. **A: Architecture boundary sensor** — implemented.
2. **B: Nested CI sensor** — not implemented.
3. **C: Staged out-of-scope sensor** — implemented.
4. **D: Frontend build escalation sensor** — implemented.
5. **E: Absorbed PR test comparison script** — not implemented.
6. **F: RSpec SimpleCov note parser** — not implemented.

## Items intentionally not changed yet

Only the user-approved recommended set (A, C, D) was implemented. B, E, and F remain candidates for future harness sessions.
