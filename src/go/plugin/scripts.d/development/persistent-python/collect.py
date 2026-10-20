#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Synthetic lifecycle example: critical, failed collection, then recovery."""
import json
import sys


def emit(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)


def main():
    if sys.argv[1:] != ["serve"]:
        return 2
    emit({"version": "v1", "ready": True})
    count = 0
    for line in sys.stdin:
        request = json.loads(line)
        if (set(request) != {"id", "method"} or request["method"] != "collect"
                or not isinstance(request["id"], str)):
            return 2
        count += 1
        if count == 2:
            emit({"id": request["id"], "error": "collection_failed"})
            continue
        emit({"id": request["id"], "result": {
            "version": "v1",
            "metrics": [{"name": "processed_total", "value": count, "labels": {"queue": "mail"}}],
            "checks": [{"id": "backlog", "state": "critical" if count == 1 else "ok",
                        "labels": {"queue": "mail"}}],
        }})
    return 0


if __name__ == "__main__":
    sys.exit(main())
