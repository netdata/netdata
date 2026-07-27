#!/usr/bin/env python3
"""Compatibility launcher for the authoritative Go profile validator."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import sys


def main() -> None:
    repo_root = Path(__file__).resolve().parents[4]
    go_root = repo_root / "src" / "go"
    tool = go_root / "tools" / "prometheus-profile-validation"

    go = shutil.which("go")
    if go is None:
        raise SystemExit("error: go is not available in PATH")
    if not tool.is_dir():
        raise SystemExit(f"error: validator source not found: {tool}")

    os.chdir(go_root)
    os.execv(go, [go, "run", "./tools/prometheus-profile-validation", *sys.argv[1:]])


if __name__ == "__main__":
    main()
