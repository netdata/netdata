#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""One file supplies metadata, configured collection and an interactive Function."""
import json
import sys

DESCRIPTION = {
    "version": "v1",
    "mode": "persistent" if "--persistent" in sys.argv[1:-1] else "oneshot",
    "functions": [{"id": "items", "name": "Items", "help": "Show the synthetic queue."}],
    "charts": """version: v1
context_namespace: selfcontained_python
groups:
  - family: Queue
    metrics: [depth]
    charts:
      - id: depth
        title: Queue depth
        context: depth
        units: jobs
        dimensions: [{selector: depth, name: depth}]
""",
    "config_schema": {
        "jsonSchema": {
            "$schema": "http://json-schema.org/draft-07/schema#",
            "title": "Synthetic queue configuration.",
            "description": "Configure the synthetic queue.",
            "type": "object", "additionalProperties": False,
            "properties": {
                "count": {"title": "Count", "description": "Synthetic queue depth in jobs.",
                          "type": "integer", "minimum": 0, "default": 17},
            },
        },
        "uiSchema": {},
    },
}


def emit(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)


def snapshot(count):
    return {"version": "v1", "metrics": [{"name": "depth", "unit": "jobs", "samples": [{"value": count}]}]}


def answer(request, count):
    if request["method"] == "collect":
        result = snapshot(count)
    elif request["function"] != "items":
        result = {"version": "v1", "status": 404, "message": "Unknown Function"}
    elif request["info"]:
        result = {"version": "v1", "status": 200}
    else:
        result = {"version": "v1", "status": 200,
                  "columns": {"jobs": {"index": 0, "name": "Jobs", "type": "integer"}},
                  "data": [[count]]}
    emit({"id": request["id"], "result": result})


operation = sys.argv[-1]
if operation == "describe":
    emit(DESCRIPTION)
    sys.exit(0)

# Description is independent of job configuration and source connectivity.
count = json.loads(sys.stdin.readline())["config"]["count"]
if operation == "collect":
    emit(snapshot(count))
elif operation == "function":
    answer(json.loads(sys.stdin.readline()), count)
elif operation == "serve":
    emit({"version": "v1", "ready": True})
    for line in sys.stdin:
        answer(json.loads(line), count)
else:
    sys.exit(2)
