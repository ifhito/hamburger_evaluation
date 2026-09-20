# Harness Evidence

Updated: 2026-05-10 17:48 JST
Scope: recommended harness improvements selected by user: A, C, D.

## Implemented mitigations

### A. Architecture boundary sensor

Change:
- Updated `.claude/hooks/stop-sensors.py` to inspect changed files under:
  - `backend/app/controllers/`
  - `backend/app/jobs/`
  - `backend/app/domain/`
- The sensor reports forbidden persistence-looking calls such as `.where(`, `.find(`, `.save!(`, `.update!(`, `.discard!(`, and `.upsert(`.

Past failure class:
- Backend boundary refactor needed a later pass to move persistence access behind repositories.

Replay/evidence:

```bash
python3 - <<'PY'
import importlib.util
from pathlib import Path
root = Path('.')
script = root / '.claude/hooks/stop-sensors.py'
spec = importlib.util.spec_from_file_location('stop_sensors', script)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
tmp = root / 'backend/app/controllers/__sensor_tmp_controller.rb'
tmp.write_text('class TmpController\n  def show\n    Review.where(user_id: 1)\n  end\nend\n')
try:
    violations = mod.architecture_boundary_violations(['backend/app/controllers/__sensor_tmp_controller.rb'])
    assert violations
finally:
    tmp.unlink()
PY
```

Observed:

```text
architecture violations: ['backend/app/controllers/__sensor_tmp_controller.rb:3: contains `.where(`']
```

Result: the repeated direct ActiveRecord boundary mistake is now caught before finishing when it appears in changed boundary files.

### C. Staged out-of-scope sensor

Change:
- Updated `.claude/hooks/stop-sensors.py` to inspect staged files with `git diff --cached --name-only --diff-filter=ACMR`.
- The sensor fails if these are staged without explicit override:
  - `SETUP.md`
  - `plans/*`
  - `memory/*`
  - `plan/*`
- Override requires `AGENT_ALLOW_OUT_OF_SCOPE_STAGED=1`, which should only be used after user approval.

Past failure class:
- Existing unrelated local files can be accidentally included in harness/code commits.

Replay/evidence:

```bash
python3 - <<'PY'
import importlib.util
from pathlib import Path
script = Path('.claude/hooks/stop-sensors.py')
spec = importlib.util.spec_from_file_location('stop_sensors', script)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
assert mod.is_out_of_scope_staged('SETUP.md')
assert mod.is_out_of_scope_staged('plans/example.md')
assert not mod.is_out_of_scope_staged('AGENTS.md')
PY
```

Observed:

```text
out-of-scope classifier: ok
```

Result: unrelated staged paths are now blocked deterministically before completion.

### D. Frontend build escalation sensor

Change:
- Updated `.claude/hooks/stop-sensors.py` to mark build escalation active when changed paths include:
  - `frontend/src/api/client/`
  - `frontend/vite.config*`
  - `frontend/tsconfig*`
- The Stop sensor already runs `pnpm run build` for frontend changes; the new classifier makes API/proxy/build-boundary cases explicit in output.

Past failure class:
- Frontend proxy/API boundary changes previously required correction after implementation.

Replay/evidence:

```bash
python3 - <<'PY'
import importlib.util
from pathlib import Path
script = Path('.claude/hooks/stop-sensors.py')
spec = importlib.util.spec_from_file_location('stop_sensors', script)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
assert any('frontend/src/api/client/buildApiClient.ts'.startswith(p) for p in mod.FRONTEND_BUILD_ESCALATION_PREFIXES)
assert any('frontend/vite.config.ts'.startswith(p) for p in mod.FRONTEND_BUILD_ESCALATION_PREFIXES)
PY
```

Observed:

```text
frontend build escalation classifier: ok
```

Result: future API client/Vite/TypeScript boundary edits are explicitly routed through production build validation.

## End-to-end Stop sensor run

Command:

```bash
python3 .claude/hooks/stop-sensors.py
```

Observed:

```text
backend sensors skipped: no backend source changes detected
frontend sensors skipped: no frontend source changes detected
All active sensors passed.
```

## AGENTS.md size check after implementation

- Lines: 74
- Status: under the 200-instruction ceiling.

## Not implemented in this pass

- Nested CI sensor.
- Absorbed PR test comparison script.
- RSpec SimpleCov note parser.

These remain in `harness-debt.md` as candidates for future harness sessions.
