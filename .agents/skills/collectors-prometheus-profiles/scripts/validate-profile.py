#!/usr/bin/env python3
"""Compatibility launcher for the authoritative Go profile validator."""

from __future__ import annotations

from pathlib import Path
import sys

from _go_tool_launcher import exec_go_tool, normalize_path_arguments

_FILE_OPTIONS = frozenset(
    {
        "--profile",
        "-profile",
        "--dump",
        "-dump",
        "--job",
        "-job",
        "--support-profile",
        "-support-profile",
    }
)


def normalize_file_arguments(arguments: list[str], caller_cwd: Path) -> list[str]:
    """Preserve caller-relative file arguments after the launcher changes directory."""
    return normalize_path_arguments(arguments, _FILE_OPTIONS, caller_cwd)


def main() -> None:
    arguments = normalize_file_arguments(sys.argv[1:], Path.cwd())
    exec_go_tool(__file__, "./tools/prometheus-profile-validation", arguments)


if __name__ == "__main__":
    main()
