#!/usr/bin/env python3
"""Claude Code・Hermes・Codex のセッション向けの、終了前の高速なセンサー。

意図的に保守的にしてある。git の衛生と秘密パスは常に確認し、Go の API(backend-go/)
または frontend のソースが変更されたときだけ、その領域の検査を走らせる。
"""
from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SECRET_MARKERS = (
    ".env",
    ".env.",
    "secrets/",
    # 旧 API(backend/)の名残として手元に残りうる秘密ファイル。.gitignore の除外がない環境でも誤って commit しない
    "backend/.kamal/secrets",
    "backend/config/master.key",
)
FRONTEND_PREFIXES = ("frontend/src/", "frontend/package.json", "frontend/pnpm-lock.yaml", "frontend/vite.config", "frontend/tsconfig", "frontend/eslint")
GO_PREFIXES = ("backend-go/",)
# クリーンアーキテクチャの内向き依存ルール: domain/usecase から外側への import を禁止
GO_BOUNDARY_RULES = (
    ("backend-go/internal/domain/", ('"net/http"', "database/sql", "pgx", "/adapter/", "/usecase/", "/internal/photo")),
    ("backend-go/internal/usecase/", ('"net/http"', "database/sql", "pgx", "/adapter/")),
)
# repository は domain からだけ使う(S15 / #46 の規約):
# - *Repository の interface(書き込み専用)は domain が宣言し、呼べるのは domain のコードだけ
#   (単一集約の書き込みは集約ごとの書き込みオブジェクト。*Service は集約を跨ぐ更新だけ)
# - usecase は repository を宣言も保持も呼び出しもしない。読み取りは usecase が宣言する *Query、
#   書き込みは domain の書き込みオブジェクトを通す
# interface のメソッド名は、*Query が Get*/List* だけ、*Repository が Create*/Update*/Discard* だけ
GO_INTERFACE_START_RE = re.compile(r"^type\s+(\w+)\s+interface\s*\{", re.M)
GO_ALLOWED_PREFIXES = {"Query": ("Get", "List"), "Repository": ("Create", "Update", "Discard")}
GO_REPOSITORY_TYPE_RE = re.compile(r"^type\s+(\w*Repository)\b", re.M)
# パッケージ名は問わず(import の別名でも)、名前の接尾辞だけで判定する
GO_REPOSITORY_REF_RE = re.compile(r"\b(\w+)\.(\w*Repository)\b")
GO_REPO_CALL_RE = re.compile(r"\.repo\.[A-Z]")
GO_DOT_IMPORT_RE = re.compile(r'^\s*(?:import\s+)?\.\s+"')
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
    if name == ".env.example":  # 値の雛形(秘密の値を書かない)。design/.env.example など
        return False
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


def go_boundary_violations(paths: list[str]) -> list[str]:
    violations: list[str] = []
    for path in paths:
        if not path.endswith(".go"):
            continue
        matched = [rule for prefix, rule in GO_BOUNDARY_RULES if path.startswith(prefix)]
        if not matched:
            continue
        file_path = ROOT / path
        if not file_path.is_file():
            continue
        for index, line in enumerate(file_path.read_text(errors="ignore").splitlines(), start=1):
            stripped = line.strip()
            if stripped.startswith("//"):
                continue
            for pattern in matched[0]:
                if pattern in stripped:
                    violations.append(f"{path}:{index}: contains `{pattern}`")
                    break
    return violations


def go_interfaces(text: str):
    """(名前, 本体, 開始行)を返す。波括弧の対応で本体を切り出すので、1 行の interface も扱える。"""
    for match in GO_INTERFACE_START_RE.finditer(text):
        depth, i = 1, match.end()
        while i < len(text) and depth:
            depth += {"{": 1, "}": -1}.get(text[i], 0)
            i += 1
        yield match.group(1), text[match.end() : i - 1], text[: match.start()].count("\n") + 1


def go_query_repository_violations(paths: list[str]) -> list[str]:
    violations: list[str] = []
    for path in paths:
        in_usecase = path.startswith("backend-go/internal/usecase/")
        in_domain = path.startswith("backend-go/internal/domain/")
        if not ((in_usecase or in_domain) and path.endswith(".go") and not path.endswith("_test.go")):
            continue
        file_path = ROOT / path
        if not file_path.is_file():
            continue
        text = file_path.read_text(errors="ignore")
        # interface のメソッド名(*Query は Get*/List* だけ、*Repository は Create*/Update*/Discard* だけ)
        for name, body, first_line in go_interfaces(text):
            kind = next((k for k in GO_ALLOWED_PREFIXES if name.endswith(k)), None)
            if kind is None:
                continue
            for offset, raw in enumerate(body.split("\n")):
                for part in raw.split(";"):
                    part = part.split("//", 1)[0].strip()
                    member = re.match(r"([\w.]+)(\()?", part)
                    if not member:
                        continue
                    if member.group(2) is None:
                        violations.append(f"{path}:{first_line + offset}: {name} embeds {member.group(1)} (embedded interfaces are not allowed)")
                    elif not member.group(1).startswith(GO_ALLOWED_PREFIXES[kind]):
                        violations.append(
                            f"{path}:{first_line + offset}: {name}.{member.group(1)} must start with "
                            + "/".join(f"{p}*" for p in GO_ALLOWED_PREFIXES[kind]) + f" (*{kind})"
                        )
        if not in_usecase:
            continue
        # usecase は repository を宣言も保持も呼び出しもしない(repository は domain のコードからだけ使う)
        for match in GO_REPOSITORY_TYPE_RE.finditer(text):
            line = text[: match.start()].count("\n") + 1
            violations.append(f"{path}:{line}: usecase declares {match.group(1)} (repository types belong to domain)")
        for index, raw in enumerate(text.splitlines(), start=1):
            code = raw.split("//", 1)[0]
            if GO_DOT_IMPORT_RE.match(code):
                violations.append(f"{path}:{index}: dot import hides repository references (do not use it in usecase)")
            for ref in GO_REPOSITORY_REF_RE.finditer(code):
                violations.append(f"{path}:{index}: usecase references {ref.group(1)}.{ref.group(2)} (writes go through domain write objects)")
            if GO_REPO_CALL_RE.search(code):
                violations.append(f"{path}:{index}: usecase calls a repository (writes go through domain write objects)")
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

    go_violations = go_boundary_violations(paths)
    if go_violations:
        print("Go clean-architecture boundary violations found in changed files:")
        for violation in go_violations:
            print(f"- {violation}")
        print("Keep domain/usecase free of net/http, sql drivers, and adapter imports.")
        failures.append("go architecture boundary sensor failed")

    query_violations = go_query_repository_violations(paths)
    if query_violations:
        print("Repository dependency violations found in changed files:")
        for violation in query_violations:
            print(f"- {violation}")
        print(
            "Repositories are used only from domain: *Repository interfaces (Create*/Update*/Discard* only) live in domain "
            "and are called only by domain code; usecase reads via *Query (Get*/List* only) and writes via domain write objects (per-aggregate; *Service only for cross-aggregate updates)."
        )
        failures.append("go query/repository split sensor failed")

    if run(["git", "diff", "--check"]) != 0:
        failures.append("git diff --check failed")

    frontend_changed = any(p.startswith(FRONTEND_PREFIXES) for p in paths)

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

    go_dir = ROOT / "backend-go"
    go_changed = any(p.startswith(GO_PREFIXES) for p in paths)
    if go_changed and go_dir.is_dir():
        for cmd in (
            ["bash", "-c", 'test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }'],
            ["go", "vet", "./..."],
            ["go", "build", "./..."],
            ["go", "test", "./..."],
        ):
            if run(cmd, go_dir) != 0:
                failures.append("go sensor failed: " + " ".join(cmd))
    else:
        print("go sensors skipped: no backend-go source changes detected")

    if failures:
        print("FAILED sensors:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("All active sensors passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
