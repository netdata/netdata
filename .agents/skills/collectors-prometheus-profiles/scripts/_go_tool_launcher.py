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
