#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Synthetic configured example; works with either manifest mode."""
import json
import sys

config = json.loads(sys.stdin.readline())["config"]

def snapshot():
    labels = {"queue": config["queue"]}
    state = "warning" if config["depth"] >= config["warning"] else "ok"
    return {
        "version": "v1",
        "metrics": [{"name": "depth", "value": config["depth"], "labels": labels}],
        "checks": [{"id": "backlog", "state": state, "labels": labels}],
    }

if sys.argv[-1] == "collect":
    print(json.dumps(snapshot()))
elif sys.argv[-1] == "serve":
    print(json.dumps({"version": "v1", "ready": True}), flush=True)
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({"id": request["id"], "result": snapshot()}), flush=True)
else:
    sys.exit(2)
