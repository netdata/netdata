#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Validate a network-viewer.plugin --test topology:network-connections payload.

Reads a pluginsd FUNCTION_RESULT payload from the given file (or stdin) and
checks it against FUNCTION_TOPOLOGY_SCHEMA.json and a few semantic invariants.

Usage:
  validate_topology_payload.py <payload_file> [<schema_file>]
When <schema_file> is omitted it defaults to src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json
relative to the repository root (taken from the script location).
"""

import io
import json
import sys
import os

def load_payload(path):
    raw = io.open(path, encoding='utf-8', errors='replace').read()
    # payload is between the FUNCTION_RESULT_BEGIN header line and
    # the FUNCTION_RESULT_END trailing line
    start = raw.find("\n")
    if start == -1:
        start = 0
    else:
        start += 1
    end = raw.find("FUNCTION_RESULT_END")
    if end != -1:
        raw = raw[start:end]
    else:
        raw = raw[start:]
    return json.loads(raw)

def semantic_checks(data):
    errors = []
    if data.get("type") != "topology":
        errors.append("payload type is not 'topology'")
    if data.get("v") != 3:
        errors.append("payload version is not 3")
    d = data.get("data")
    if not isinstance(d, dict):
        return ["missing data object"]
    for table in ("actors", "links"):
        t = d.get(table)
        if not isinstance(t, dict) or "columns" not in t or "values" not in t:
            errors.append(f"{table} table missing columns/values")
    if "types" not in d or "actor_types" not in d.get("types", {}):
        errors.append("missing types.actor_types registry")
    if "presentation" not in d:
        errors.append("missing presentation")
    if "view" not in d or d["view"].get("mode") not in ("aggregated", "detailed"):
        errors.append("missing/invalid view.mode")
    return errors

def main(argv):
    payload_file = argv[0] if len(argv) > 0 else None
    if payload_file is None or payload_file == "-":
        payload_file = "/dev/stdin"
    here = os.path.dirname(os.path.abspath(__file__))
    schema_file = argv[1] if len(argv) > 1 else os.path.join(
        here, "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json")

    data = load_payload(payload_file)
    schema = json.load(io.open(schema_file, encoding='utf-8'))

    try:
        import jsonschema
    except ImportError:
        errors = semantic_checks(data)
        if errors:
            print("SEMANTIC FAILURES:")
            for e in errors:
                print(" -", e)
            return 1
        print("jsonschema not available; semantic checks passed")
        return 0

    validator = jsonschema.Draft7Validator(schema)
    errs = sorted(validator.iter_errors(data), key=lambda e: list(e.path))
    if errs:
        print(f"SCHEMA FAILURES ({len(errs)}):")
        for e in errs[:20]:
            print(" -", "/".join(str(p) for p in e.path), ":", e.message[:200])
        if len(errs) > 20:
            print(f" ... and {len(errs) - 20} more")
        return 1

    errors = semantic_checks(data)
    if errors:
        print("SEMANTIC FAILURES:")
        for e in errors:
            print(" -", e)
        return 1

    print(f"OK: payload validates against {os.path.basename(schema_file)}"
          f" (actors={data['data']['actors']['rows']}, links={data['data']['links']['rows']}, "
          f"mode={data['data']['view']['mode']})")
    return 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
