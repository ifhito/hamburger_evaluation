#!/usr/bin/env python3
"""Claude Code・Hermes・Codex のセッション向けの、終了前の高速なセンサー。

意図的に保守的にしてある。git の衛生と秘密パスは常に確認し、Go の API(backend-go/)
または frontend のソースが変更されたときだけ、その領域の検査を走らせる。

Go の検査は、ホストの Go が go.mod の版より古いときは、`golang:<版>` の使い捨て Docker で動かす
(DB は使わないので、DB のテストは skip される。完全な検査は CI の Backend Go が行う)。
"""
from __future__ import annotations

import os
import re
import shutil
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
# Go の検査(gofmt / vet / build / test)の意味があるファイル。`.env.example`・文書・compose の例などでは起動しない
GO_CHECK_SUFFIXES = (".go", ".sql")
GO_CHECK_NAMES = {"go.mod", "go.sum", "sqlc.yaml", "sqlc.yml"}
GOFMT_CHECK = 'test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }'
GO_NATIVE_COMMANDS = (
    ["bash", "-c", GOFMT_CHECK],
    ["go", "vet", "./..."],
    ["go", "build", "./..."],
    ["go", "test", "./..."],
)
# Docker の中では 4 つの検査を 1 回の起動にまとめ、どれが失敗したかを出力に残す
GO_DOCKER_SCRIPT = "\n".join(
    [
        "rc=0",
        'unformatted="$(gofmt -l .)"',
        '[ -z "$unformatted" ] || { echo "$unformatted"; echo "FAILED: gofmt"; rc=1; }',
        'go vet ./... || { echo "FAILED: go vet"; rc=1; }',
        'go build ./... || { echo "FAILED: go build"; rc=1; }',
        'go test ./... || { echo "FAILED: go test"; rc=1; }',
        "exit $rc",
    ]
)
GO_CACHE_VOLUME = "he-sensor-gocache"
STATUS_LINES_MAX = 30
# クリーンアーキテクチャの内向き依存ルール: domain/usecase から外側への import を禁止
GO_BOUNDARY_RULES = (
    ("backend-go/internal/domain/", ('"net/http"', "database/sql", "pgx", "/adapter/", "/usecase/", "/internal/photo")),
    ("backend-go/internal/usecase/", ('"net/http"', "database/sql", "pgx", "/adapter/")),
)
# repository は domain からだけ使う規約:
# - *Repository の interface(書き込み専用)は domain が宣言し、呼べるのは domain のコードだけ
#   (単一集約の書き込みは集約ごとの書き込みオブジェクト。*Service は、間に読み取りを挟まない、集約を跨ぐ更新だけ。
#   読み取りを挟む手順は、usecase が UnitOfWork.Do の中で組み立てる)
# - usecase は repository を宣言も保持も呼び出しもしない。読み取りは usecase が宣言する *Query、
#   書き込みは domain の書き込みオブジェクトを通す
# interface のメソッド名は、*Query が Get*/List* だけ、*Repository が Create*/Update*/Discard* と、
# 書き込みの前段の排他ロックの Lock*(統計の再計算の前に、バーガーの行をロックして、並行する書き込みの取りこぼしを防ぐ。
# 値を返さず、行も変えない)だけ
GO_INTERFACE_START_RE = re.compile(r"^type\s+(\w+)\s+interface\s*\{", re.M)
GO_ALLOWED_PREFIXES = {"Query": ("Get", "List"), "Repository": ("Create", "Update", "Discard", "Lock")}
GO_REPOSITORY_TYPE_RE = re.compile(r"^type\s+(\w*Repository)\b", re.M)
# パッケージ名は問わず(import の別名でも)、名前の接尾辞だけで判定する
GO_REPOSITORY_REF_RE = re.compile(r"\b(\w+)\.(\w*Repository)\b")
GO_REPO_CALL_RE = re.compile(r"\.repo\.[A-Z]")
GO_DOT_IMPORT_RE = re.compile(r'^\s*(?:import\s+)?\.\s+"')
# テスト名・コメントに story・受け入れ条件・issue の番号を書かない(story を知らない人にも伝わる書き方にする)。
# 追加した行だけを検査し、既存の行は対象外にする。TODO(#123) の追跡用の番号は許す(`issue #` などの形だけを拾う)。
# `S3`(オブジェクトストレージ)のような誤検出を避けるため、`S<数字>` 単独は対象にしない。
STORY_REF_RE = re.compile(r"\bAC[0-9]+\b|Story\s*#?[0-9]+|[Ss]tory\s*S[0-9]+|\bS[0-9]{1,2}\s+AC[0-9]+|issue\s*#[0-9]+|PR\s*#[0-9]+")
TEST_FILE_SUFFIXES = ("_test.go", ".test.ts", ".test.tsx")
CODE_FILE_SUFFIXES = (".go", ".ts", ".tsx")
HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")
OUT_OF_SCOPE_STAGED_PREFIXES = ("plans/", "memory/", "plan/")
OUT_OF_SCOPE_STAGED_FILES = {"SETUP.md"}
FRONTEND_BUILD_ESCALATION_PREFIXES = (
    "frontend/src/api/client/",
    "frontend/vite.config",
    "frontend/tsconfig",
)


def run(cmd: list[str], cwd: Path = ROOT, label: str | None = None) -> int:
    print(f"$ {label or ' '.join(cmd)}  # cwd={cwd.relative_to(ROOT) if cwd != ROOT else '.'}")
    result = subprocess.run(cmd, cwd=cwd, text=True)
    return result.returncode


def capture(cmd: list[str], cwd: Path = ROOT) -> str:
    return subprocess.check_output(cmd, cwd=cwd, text=True, stderr=subprocess.STDOUT)


def go_relevant(path: str) -> bool:
    if not path.startswith(GO_PREFIXES):
        return False
    return Path(path).name in GO_CHECK_NAMES or path.endswith(GO_CHECK_SUFFIXES)


def parse_go_version(text: str) -> tuple[int, int] | None:
    """`go 1.27`(go.mod)や `go version go1.19 darwin/arm64` から (1, 27) のような版を取り出す。"""
    match = re.search(r"(?:^|\s)go\s*(\d+)\.(\d+)", text)
    return (int(match.group(1)), int(match.group(2))) if match else None


def host_go_satisfies(host: tuple[int, int] | None, required: tuple[int, int] | None) -> bool:
    if required is None:
        return True
    return host is not None and host >= required


def limit_lines(text: str, limit: int) -> str:
    lines = text.splitlines()
    if len(lines) <= limit:
        return "\n".join(lines)
    return "\n".join(lines[:limit] + [f"... ほか {len(lines) - limit} 行"])


def print_git_status() -> bool:
    # 未追跡のディレクトリは 1 行にまとめ、行数も切り詰める(本当の失敗が埋もれないように)
    cmd = ["git", "status", "--short", "--branch", "--untracked-files=normal"]
    print(f"$ {' '.join(cmd)}  # cwd=.")
    try:
        print(limit_lines(capture(cmd), STATUS_LINES_MAX))
    except subprocess.CalledProcessError as exc:
        print(exc.output)
        return False
    return True


def docker_available() -> bool:
    if not shutil.which("docker"):
        return False
    try:
        return subprocess.run(["docker", "info"], capture_output=True, timeout=15).returncode == 0
    except subprocess.TimeoutExpired:
        return False


def run_go_sensors(go_dir: Path) -> list[str]:
    try:
        required = parse_go_version((go_dir / "go.mod").read_text())
    except OSError:
        required = None
    try:
        host = parse_go_version(capture(["go", "version"]))
    except (OSError, subprocess.CalledProcessError):
        host = None

    if host_go_satisfies(host, required):
        return ["go sensor failed: " + " ".join(cmd) for cmd in GO_NATIVE_COMMANDS if run(cmd, go_dir) != 0]

    need = f"{required[0]}.{required[1]}"
    have = f"{host[0]}.{host[1]}" if host else "なし"
    if not docker_available():
        print(f"go sensors skipped: ホストの Go({have})が go.mod の版({need})より古く、Docker も使えない。完全な検査は CI の Backend Go が行う")
        return []

    image = f"golang:{need}"
    print(f"go sensors: ホストの Go({have})が go.mod の版({need})より古いので、{image} の Docker で動かす。DB を使うテストは skip される(完全な検査は CI の Backend Go)")
    # 名前つき volume は root の所有で作られるので、非 root で書けるようにしてから、キャッシュとして共有する
    run(["docker", "run", "--rm", "-v", f"{GO_CACHE_VOLUME}:/cache", image, "chmod", "a+rwx", "/cache"])
    cmd = [
        "docker", "run", "--rm", "-u", f"{os.getuid()}:{os.getgid()}",
        "-v", f"{go_dir}:/src", "-w", "/src", "-v", f"{GO_CACHE_VOLUME}:/cache",
        "-e", "HOME=/tmp", "-e", "GOCACHE=/cache/build", "-e", "GOMODCACHE=/cache/mod", "-e", "GOFLAGS=-buildvcs=false",
        image, "sh", "-c", GO_DOCKER_SCRIPT,
    ]
    if run(cmd, ROOT, label=f"docker run {image}: gofmt / go vet / go build / go test") != 0:
        return [f"go sensor failed in Docker {image}(出力の FAILED: の行を参照)"]
    return []


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
    if name.startswith(".env.") and name.endswith(".example"):  # 値の雛形(秘密の値を書かない)。.env.example・.env.bench.example など
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


def added_lines(path: str) -> list[tuple[int, str]]:
    """path の追加された行(行番号, 本文)を返す。未追跡のファイルは全行を返す。"""
    file = ROOT / path
    if not file.is_file():
        return []
    tracked = subprocess.run(["git", "ls-files", "--error-unmatch", "--", path], cwd=ROOT, capture_output=True).returncode == 0
    if not tracked:
        return list(enumerate(file.read_text(encoding="utf-8", errors="replace").splitlines(), start=1))
    diff = capture(["git", "diff", "-U0", "HEAD", "--", path])
    lines: list[tuple[int, str]] = []
    current = 0
    for line in diff.splitlines():
        hunk = HUNK_RE.match(line)
        if hunk:
            current = int(hunk.group(1))
        elif line.startswith("+") and not line.startswith("+++"):
            lines.append((current, line[1:]))
            current += 1
    return lines


def readability_violations(paths: list[str]) -> list[str]:
    """追加した行のテスト名・コメントにある story・受け入れ条件・issue の番号を返す。

    テストファイルは追加行のすべて(テスト名の文字列とコメント)を、本番のコード(.go / .ts / .tsx)は
    追加行のコメント部分だけを検査する。
    """
    violations: list[str] = []
    for path in paths:
        is_test = path.endswith(TEST_FILE_SUFFIXES)
        if not (is_test or path.endswith(CODE_FILE_SUFFIXES)):
            continue
        for number, text in added_lines(path):
            target = text
            if not is_test:
                if "//" not in text and not text.lstrip().startswith(("/*", "*")):
                    continue
                target = text.split("//", 1)[1] if "//" in text else text
            match = STORY_REF_RE.search(target)
            if match:
                violations.append(f"{path}:{number}: 「{match.group(0)}」 {text.strip()[:100]}")
    return violations


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
        # interface のメソッド名(*Query は Get*/List* だけ、*Repository は Create*/Update*/Discard*/Lock* だけ)
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
    # 子プロセス(docker など)の出力と順番が入れ替わらないよう、1 行ずつ出す
    sys.stdout.reconfigure(line_buffering=True)
    os.chdir(ROOT)
    failures: list[str] = []

    if not print_git_status():
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
            "Repositories are used only from domain: *Repository interfaces (Create*/Update*/Discard* and the pre-write row lock Lock* only) live in domain "
            "and are called only by domain code; usecase reads via *Query (Get*/List* only) and writes via domain write objects (per-aggregate; *Service only for cross-aggregate updates with no read in between; procedures that read in between are built by the usecase inside UnitOfWork.Do)."
        )
        failures.append("go query/repository split sensor failed")

    readability = readability_violations(paths)
    if readability:
        print("Test names / comments added in this change contain story, acceptance-criteria or issue numbers:")
        for violation in readability:
            print(f"- {violation}")
        print(
            "Write test names and comments so that a reader who does not know the story understands them: "
            "no numbers such as AC1 / Story #12 / issue #12 (they belong in the issue and PR only), "
            "and state 'situation -> result'. See 「読む人に伝わる書き方」 in .agents/skills/backend-go-boundaries."
        )
        failures.append("readable test names/comments sensor failed")

    if run(["git", "diff", "--check"]) != 0:
        failures.append("git diff --check failed")

    frontend_changed = any(p.startswith(FRONTEND_PREFIXES) for p in paths)

    if frontend_changed:
        frontend = ROOT / "frontend"
        build_escalated = any(p.startswith(FRONTEND_BUILD_ESCALATION_PREFIXES) for p in paths)
        if build_escalated:
            print("frontend build escalation active: API client/Vite/TypeScript boundary changed")
        # 環境がそろっていないだけのときは、コードの誤りと区別して skip にする(完全な検査は CI の Frontend が行う)
        if not shutil.which("pnpm") or not (frontend / "node_modules").is_dir():
            print("frontend sensors skipped: pnpm または frontend/node_modules がない(`cd frontend && pnpm install` が必要)。完全な検査は CI の Frontend が行う")
        else:
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
    go_changed = any(go_relevant(p) for p in paths)
    if go_changed and go_dir.is_dir():
        failures.extend(run_go_sensors(go_dir))
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
