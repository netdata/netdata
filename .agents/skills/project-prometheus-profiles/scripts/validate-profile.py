#!/usr/bin/env python3
"""Compatibility launcher for the authoritative Go profile validator."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import sys

_FILE_OPTIONS = frozenset(
    {
        "--profile",
        "-profile",
        "--dump",
        "-dump",
        "--job",
        "-job",
    }
)


def normalize_file_arguments(arguments: list[str], caller_cwd: Path) -> list[str]:
    """Preserve caller-relative file arguments after the launcher changes directory."""
    normalized: list[str] = []
    index = 0
    while index < len(arguments):
        argument = arguments[index]
        option, separator, value = argument.partition("=")

        if separator and option in _FILE_OPTIONS:
            normalized.append(f"{option}={_absolute_argument(value, caller_cwd)}")
        elif argument in _FILE_OPTIONS and index + 1 < len(arguments):
            normalized.extend(
                [
                    argument,
                    _absolute_argument(arguments[index + 1], caller_cwd),
                ]
            )
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
    repo_root = Path(__file__).resolve().parents[4]
    go_root = repo_root / "src" / "go"
    tool = go_root / "tools" / "prometheus-profile-validation"
    arguments = normalize_file_arguments(sys.argv[1:], Path.cwd())

    go = shutil.which("go")
    if go is None:
        raise SystemExit("error: go is not available in PATH")
    if not tool.is_dir():
        raise SystemExit(f"error: validator source not found: {tool}")

    os.chdir(go_root)
    try:
        os.execv(go, [go, "run", "./tools/prometheus-profile-validation", *arguments])
    except OSError as error:
        raise SystemExit(f"error: failed to execute {go}: {error}") from error


if __name__ == "__main__":
    main()
