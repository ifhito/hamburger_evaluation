#!/usr/bin/env python3
"""Fast stop sensors for Claude Code, Hermes, and Codex sessions.

The script is intentionally conservative: it always checks git hygiene and
secret paths, then runs area-specific checks only when backend/frontend source
files changed in this working tree.
"""
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SECRET_MARKERS = (
    ".env",
    ".env.",
    "secrets/",
    "backend/.kamal/secrets",
    "backend/config/master.key",
)
BACKEND_PREFIXES = ("backend/app/", "backend/config/", "backend/db/", "backend/lib/", "backend/spec/", "backend/Gemfile", "backend/Gemfile.lock")
FRONTEND_PREFIXES = ("frontend/src/", "frontend/package.json", "frontend/pnpm-lock.yaml", "frontend/vite.config", "frontend/tsconfig", "frontend/eslint")
ARCHITECTURE_BOUNDARY_PREFIXES = (
    "backend/app/controllers/",
    "backend/app/jobs/",
    "backend/app/domain/",
)
ARCHITECTURE_FORBIDDEN_PATTERNS = (
    ".find(",
    ".find_by(",
    ".where(",
    ".joins(",
    ".includes(",
    ".create(",
    ".create!(",
    ".save(",
    ".save!(",
    ".update(",
    ".update!(",
    ".destroy(",
    ".destroy!(",
    ".discard(",
    ".discard!(",
    ".upsert(",
)
OUT_OF_SCOPE_STAGED_PREFIXES = ("plans/", "memory/", "plan/")
OUT_OF_SCOPE_STAGED_FILES = {"SETUP.md"}
FRONTEND_BUILD_ESCALATION_PREFIXES = (
    "frontend/src/api/client/",
    "frontend/vite.config",
    "frontend/tsconfig",
)


def run(cmd: list[str], cwd: Path = ROOT) -> int:
    print(f"$ {' '.join(cmd)}  # cwd={cwd.relative_to(ROOT) if cwd != ROOT else '.'}")
    result = subprocess.run(cmd, cwd=cwd, text=True)
    return result.returncode


def capture(cmd: list[str], cwd: Path = ROOT) -> str:
    return subprocess.check_output(cmd, cwd=cwd, text=True, stderr=subprocess.STDOUT)


def changed_paths() -> list[str]:
    porcelain = capture(["git", "status", "--porcelain", "--untracked-files=all"])
    paths: list[str] = []
    for line in porcelain.splitlines():
        if not line:
            continue
        path = line[3:]
        if " -> " in path:
            path = path.split(" -> ", 1)[1]
        paths.append(path)
    return paths


def is_secret_path(path: str) -> bool:
    normalized = path.strip("/")
    name = Path(normalized).name
    return (
        name == ".env"
        or name.startswith(".env.")
        or any(marker in normalized for marker in SECRET_MARKERS)
    )


def staged_paths() -> list[str]:
    output = capture(["git", "diff", "--cached", "--name-only", "--diff-filter=ACMR"])
    return [line.strip() for line in output.splitlines() if line.strip()]


def is_out_of_scope_staged(path: str) -> bool:
    return path in OUT_OF_SCOPE_STAGED_FILES or path.startswith(OUT_OF_SCOPE_STAGED_PREFIXES)


def architecture_boundary_violations(paths: list[str]) -> list[str]:
    violations: list[str] = []
    for path in paths:
        if not path.startswith(ARCHITECTURE_BOUNDARY_PREFIXES):
            continue
        file_path = ROOT / path
        if not file_path.is_file():
            continue
        for index, line in enumerate(file_path.read_text(errors="ignore").splitlines(), start=1):
            stripped = line.strip()
            if stripped.startswith("#"):
                continue
            for pattern in ARCHITECTURE_FORBIDDEN_PATTERNS:
                if pattern in stripped:
                    violations.append(f"{path}:{index}: contains `{pattern}`")
                    break
    return violations


def main() -> int:
    os.chdir(ROOT)
    failures: list[str] = []

    if run(["git", "status", "--short", "--branch", "--untracked-files=all"]) != 0:
        failures.append("git status failed")

    try:
        paths = changed_paths()
    except subprocess.CalledProcessError as exc:
        print(exc.output)
        return 1

    leaked = [p for p in paths if is_secret_path(p)]
    if leaked:
        print("Secret-like paths are present in the working tree:")
        for path in leaked:
            print(f"- {path}")
        failures.append("secret path hygiene failed")

    try:
        staged = staged_paths()
    except subprocess.CalledProcessError as exc:
        print(exc.output)
        return 1

    if os.environ.get("AGENT_ALLOW_OUT_OF_SCOPE_STAGED") != "1":
        out_of_scope = [p for p in staged if is_out_of_scope_staged(p)]
        if out_of_scope:
            print("Out-of-scope paths are staged. Unstage them or set AGENT_ALLOW_OUT_OF_SCOPE_STAGED=1 with user approval:")
            for path in out_of_scope:
                print(f"- {path}")
            failures.append("out-of-scope staged path sensor failed")

    architecture_violations = architecture_boundary_violations(paths)
    if architecture_violations:
        print("Backend architecture boundary violations found in changed files:")
        for violation in architecture_violations:
            print(f"- {violation}")
        print("Move persistence access to query/repository/service boundaries before finishing.")
        failures.append("backend architecture boundary sensor failed")

    if run(["git", "diff", "--check"]) != 0:
        failures.append("git diff --check failed")

    backend_changed = any(p.startswith(BACKEND_PREFIXES) for p in paths)
    frontend_changed = any(p.startswith(FRONTEND_PREFIXES) for p in paths)

    if backend_changed:
        backend = ROOT / "backend"
        for cmd in (
            ["docker", "compose", "run", "--rm", "-e", "RAILS_ENV=test", "api", "bundle", "exec", "rspec"],
            ["docker", "compose", "run", "--rm", "api", "bin/rubocop", "-f", "github"],
            ["docker", "compose", "run", "--rm", "api", "bin/brakeman", "--no-pager"],
        ):
            if run(cmd, backend) != 0:
                failures.append("backend sensor failed: " + " ".join(cmd))
    else:
        print("backend sensors skipped: no backend source changes detected")

    if frontend_changed:
        frontend = ROOT / "frontend"
        build_escalated = any(p.startswith(FRONTEND_BUILD_ESCALATION_PREFIXES) for p in paths)
        if build_escalated:
            print("frontend build escalation active: API client/Vite/TypeScript boundary changed")
        for cmd in (
            ["pnpm", "run", "type-check"],
            ["pnpm", "run", "lint"],
            ["pnpm", "run", "test"],
            ["pnpm", "run", "build"],
        ):
            if run(cmd, frontend) != 0:
                failures.append("frontend sensor failed: " + " ".join(cmd))
    else:
        print("frontend sensors skipped: no frontend source changes detected")

    if failures:
        print("FAILED sensors:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("All active sensors passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
