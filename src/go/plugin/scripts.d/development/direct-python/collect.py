#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Direct command using standard JSON, with scalar, enum and bitset families."""
import json
import sys


def snapshot():
    return {
        "version": "v1",
        "metrics": [
            {"name": "temperature", "unit": "Celsius", "samples": [{"value": 21.5}]},
            {"name": "worker", "type": "stateset", "states": ["in progress", "stopped"],
             "samples": [{"active": ["in progress"]}]},
            {"name": "features", "type": "stateset", "mode": "bitset", "states": ["read", "write"],
             "samples": [{"active": ["read", "write"]}]},
        ],
    }


def emit(value):
    print(json.dumps(value), flush=True)


if sys.argv[-1] == "collect":
    emit(snapshot())
elif sys.argv[-1] == "serve":
    emit({"version": "v1", "ready": True})
    for line in sys.stdin:
        request = json.loads(line)
        if request["method"] != "collect":
            sys.exit(2)
        emit({"id": request["id"], "result": snapshot()})
else:
    sys.exit(2)
