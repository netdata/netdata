#!/usr/bin/env python3
"""Launch stock-profile proof discovery and replay verification."""

from __future__ import annotations

from pathlib import Path
import sys

from _go_tool_launcher import exec_go_tool, repo_root

_FILE_OPTIONS = frozenset({"--testdata-root", "-testdata-root"})


def normalize_arguments(arguments: list[str], caller_cwd: Path) -> list[str]:
    if not arguments:
        return []
    normalized = [arguments[0], "--repo-root", str(repo_root(__file__))]
    index = 1
    while index < len(arguments):
        argument = arguments[index]
        option, separator, value = argument.partition("=")
        if separator and option in _FILE_OPTIONS:
            normalized.append(f"{option}={_absolute_argument(value, caller_cwd)}")
        elif argument in _FILE_OPTIONS and index + 1 < len(arguments):
            normalized.extend([argument, _absolute_argument(arguments[index + 1], caller_cwd)])
            index += 1
        else:
            normalized.append(argument)
        index += 1
    return normalized


def _absolute_argument(value: str, caller_cwd: Path) -> str:
    if not value:
        return value
    path = Path(value)
    return str(path if path.is_absolute() else caller_cwd / path)


def main() -> None:
    arguments = normalize_arguments(sys.argv[1:], Path.cwd())
    exec_go_tool(__file__, "./tools/prometheus-profile-proof", arguments)


if __name__ == "__main__":
    main()
