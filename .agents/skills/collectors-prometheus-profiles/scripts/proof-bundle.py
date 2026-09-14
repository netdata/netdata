#!/usr/bin/env python3
"""Launch stock-profile proof discovery and replay verification."""

from __future__ import annotations

from pathlib import Path
import sys

from _go_tool_launcher import exec_go_tool, normalize_path_arguments, repo_root

_FILE_OPTIONS = frozenset({"--testdata-root", "-testdata-root", "--repo-root", "-repo-root"})


def normalize_arguments(arguments: list[str], caller_cwd: Path) -> list[str]:
    """Inject the repository root after the subcommand and keep caller-relative paths valid."""
    if not arguments:
        return []
    normalized = [arguments[0], "--repo-root", str(repo_root(__file__))]
    normalized.extend(normalize_path_arguments(arguments[1:], _FILE_OPTIONS, caller_cwd))
    return normalized


def main() -> None:
    arguments = normalize_arguments(sys.argv[1:], Path.cwd())
    exec_go_tool(__file__, "./tools/prometheus-profile-proof", arguments)


if __name__ == "__main__":
    main()
