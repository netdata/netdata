"""Shared execution support for Prometheus profile authoring Go tools."""

from __future__ import annotations

import os
from pathlib import Path
import shutil


def repo_root(script: str) -> Path:
    start = Path(script).resolve().parent
    for candidate in (start, *start.parents):
        if (candidate / "src" / "go" / "go.mod").is_file():
            return candidate
    raise SystemExit(f"error: repository root not found from: {script}")


def normalize_path_arguments(arguments: list[str], path_options: frozenset[str], caller_cwd: Path) -> list[str]:
    """Make caller-relative values of the given options absolute so they survive the chdir into src/go.

    Both `--opt value` and `--opt=value` forms are handled. A token that starts with `-` is never absolutized
    as a value, so a missing value reaches the Go tool unchanged and is rejected there instead of becoming a
    bogus path; a path that really starts with `-` must use the `--opt=value` form.
    """
    normalized: list[str] = []
    index = 0
    while index < len(arguments):
        argument = arguments[index]
        option, separator, value = argument.partition("=")
        if separator and option in path_options:
            normalized.append(f"{option}={_absolute_path(value, caller_cwd)}")
        elif (
            argument in path_options
            and index + 1 < len(arguments)
            and not arguments[index + 1].startswith("-")
        ):
            normalized.extend([argument, _absolute_path(arguments[index + 1], caller_cwd)])
            index += 1
        else:
            normalized.append(argument)
        index += 1
    return normalized


def _absolute_path(value: str, caller_cwd: Path) -> str:
    if not value:
        return value
    path = Path(value)
    return str(path if path.is_absolute() else caller_cwd / path)


def exec_go_tool(script: str, tool: str, arguments: list[str]) -> None:
    root = repo_root(script)
    go_root = root / "src" / "go"
    tool_path = go_root / tool.removeprefix("./")

    go_path = shutil.which("go")
    if go_path is None:
        raise SystemExit("error: go is not available in PATH")
    go = str(Path(go_path).resolve())
    if not tool_path.is_dir():
        raise SystemExit(f"error: tool source not found: {tool_path}")

    os.chdir(go_root)
    try:
        os.execv(go, [go, "run", tool, *arguments])
    except OSError as error:
        raise SystemExit(f"error: failed to execute {go}: {error}") from error
