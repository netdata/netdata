#!/usr/bin/env python3
"""Normalize go test -json output for gotestfmt.

Ensures every JSON object has a non-empty Package field so
GoTestTools/gotestfmt does not panic when encountering top-level output
(lines such as module download notices or toolchain diagnostics).
If a line is not valid JSON, wrap it in a minimal JSON object so the
parser downstream still receives structured input.
"""

from __future__ import annotations

import json
import sys

_DEFAULT_PACKAGE = "__go_test__"


def _emit(obj: dict[str, object]) -> None:
    sys.stdout.write(json.dumps(obj, ensure_ascii=True))
    sys.stdout.write("\n")


def main() -> int:
    for raw in sys.stdin:
        line = raw.rstrip("\n")
        if not line:
            # Preserve empty lines as synthetic output events.
            _emit({"Action": "output", "Package": _DEFAULT_PACKAGE, "Output": "\n"})
            continue

        try:
            data = json.loads(line)
        except json.JSONDecodeError:
            # Wrap unexpected non-JSON text so downstream tooling keeps working.
            _emit({"Action": "output", "Package": _DEFAULT_PACKAGE, "Output": line + "\n"})
            continue

        if not data.get("Package"):
            data["Package"] = _DEFAULT_PACKAGE

        _emit(data)

    return 0


if __name__ == "__main__":
    sys.exit(main())
