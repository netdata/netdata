#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Function-only example, usable in both manifest modes."""
import json
import sys


def answer(request):
    if request["method"] != "function" or request["function"] != "items":
        result = {"version": "v1", "status": 404, "message": "Unknown Function"}
    elif request["info"]:
        result = {"version": "v1", "status": 200}
    else:
        result = {
            "version": "v1", "status": 200,
            "columns": {
                "worker": {"index": 0, "name": "Worker", "type": "string", "unique_key": True},
                "jobs": {"index": 1, "name": "Jobs", "type": "integer"},
            },
            "data": [["worker-1", 17], ["worker-2", 9]],
            "default_sort_column": "jobs",
            "charts": {"jobs": {"name": "Jobs", "type": "stacked-bar", "columns": ["jobs"]}},
            "group_by": {"worker": {"name": "Worker", "columns": ["worker"]}},
            "default_charts": [["jobs", "worker"]],
        }
    print(json.dumps({"id": request["id"], "result": result}, separators=(",", ":")), flush=True)


if sys.argv[-1] == "serve":
    print('{"version":"v1","ready":true}', flush=True)
    for line in sys.stdin:
        answer(json.loads(line))
elif sys.argv[-1] == "function":
    answer(json.loads(sys.stdin.readline()))
else:
    sys.exit(2)
