#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Validate a network-viewer.plugin --test topology:network-connections payload."""

# Reads a pluginsd FUNCTION_RESULT payload (or the raw JSON a --test run emits)
# from the given file and checks it against FUNCTION_TOPOLOGY_SCHEMA.json plus
# a few semantic invariants.
#
# Usage:
#   validate_topology_payload.py <payload_file> [<schema_file>] [--mode <mode>]
#                                [--group-by <group_by>] [--self-test]
# When <schema_file> is omitted it defaults to
# src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json relative to the repository root
# (taken from the script location). --mode / --group-by assert the view the
# caller requested; --self-test runs the built-in regression tests and exits.


import io
import json
import sys
import os


def schema_validator(schema):
    """Build the validator selected by the schema's declared draft."""
    import jsonschema

    validator_class = jsonschema.validators.validator_for(schema)
    validator_class.check_schema(schema)
    return validator_class(schema)


def load_payload(path):
    raw = io.open(path, encoding='utf-8', errors='replace').read()
    if raw.startswith("FUNCTION_RESULT_BEGIN"):
        # pluginsd wire format: strip the BEGIN header line and the
        # FUNCTION_RESULT_END trailing line. The trailer is only removed when
        # it is a complete trailing line, never when the text appears inside
        # a JSON string value.
        start = raw.find("\n")
        start = 0 if start == -1 else start + 1
        lines = raw[start:].split("\n")
        while lines and not lines[-1].strip():
            lines.pop()
        if lines and lines[-1].strip() == "FUNCTION_RESULT_END":
            lines.pop()
        raw = "\n".join(lines).strip()
    else:
        # --test emits the JSON body directly (no pluginsd wrapper).
        raw = raw.strip()
    return json.loads(raw)


def aggregation_scope_reference_errors(types):
    """Return actor-type references that have no matching scope definition."""
    if not isinstance(types, dict):
        return []

    actor_types = types.get("actor_types")
    aggregation_scopes = types.get("aggregation_scopes", {})
    if not isinstance(actor_types, dict) or not isinstance(aggregation_scopes, dict):
        return []

    errors = []
    for actor_type_id, actor_type in actor_types.items():
        if not isinstance(actor_type, dict):
            continue
        actor_scopes = actor_type.get("aggregation_scopes", [])
        if not isinstance(actor_scopes, list):
            errors.append(
                f"actor type {actor_type_id!r} aggregation_scopes is not an array")
            continue
        for scope_id in actor_scopes:
            if isinstance(scope_id, str) and scope_id not in aggregation_scopes:
                errors.append(
                    f"actor type {actor_type_id!r} references undefined "
                    f"aggregation scope {scope_id!r}")
    return errors


def network_viewer_actor_type_errors(types):
    """Enforce network-viewer's endpoint actor contract."""
    if not isinstance(types, dict) or not isinstance(types.get("actor_types"), dict):
        return []

    endpoint = types["actor_types"].get("endpoint")
    if not isinstance(endpoint, dict):
        return ["missing endpoint actor type"]
    endpoint_scopes = endpoint.get("aggregation_scopes", [])
    if isinstance(endpoint_scopes, list) and endpoint_scopes:
        return ["endpoint actor type must not declare aggregation scopes"]
    return []


def semantic_checks(data, expect_mode=None, expect_group_by=None):
    errors = []
    if data.get("type") != "topology":
        errors.append("payload type is not 'topology'")
    if data.get("v") != 3:
        errors.append("payload version is not 3")
    d = data.get("data")
    if not isinstance(d, dict):
        return errors + ["missing data object"]
    for table in ("actors", "links"):
        t = d.get(table)
        if not isinstance(t, dict) or "columns" not in t or "values" not in t:
            errors.append(f"{table} table missing columns/values")
    if "types" not in d or "actor_types" not in d.get("types", {}):
        errors.append("missing types.actor_types registry")
    else:
        errors.extend(aggregation_scope_reference_errors(d["types"]))
        errors.extend(network_viewer_actor_type_errors(d["types"]))
    if "presentation" not in d:
        errors.append("missing presentation")
    view = d.get("view")
    if not isinstance(view, dict):
        return errors + ["missing view object"]
    mode = view.get("mode")
    if mode not in ("aggregated", "detailed"):
        errors.append("missing/invalid view.mode")
    elif expect_mode and mode != expect_mode:
        errors.append(f"view.mode is {mode!r}, expected {expect_mode!r}")
    if expect_group_by and view.get("scope") != expect_group_by:
        errors.append(
            f"view.scope is {view.get('scope')!r}, expected {expect_group_by!r}")
    return errors


def run_self_test():
    """Regression tests for payload parsing edge cases."""
    payload = {
        "status": 200, "type": "topology", "v": 3,
        "data": {"view": {"mode": "aggregated", "scope": "process_name"}}}
    body = json.dumps(payload)
    # A JSON string that contains the trailer text must survive parsing.
    tricky = body.replace('"process_name"', '"FUNCTION_RESULT_END"')
    wrapped = "FUNCTION_RESULT_BEGIN \"123\" 200 \"result\" 0\n" + tricky + "\nFUNCTION_RESULT_END\n"
    import tempfile
    with tempfile.NamedTemporaryFile("w", suffix=".out", delete=False) as f:
        f.write(wrapped)
        name = f.name
    try:
        parsed = load_payload(name)
    finally:
        os.unlink(name)
    if parsed["data"]["view"]["scope"] != "FUNCTION_RESULT_END":
        print("self-test FAILED: FUNCTION_RESULT_END inside JSON string was stripped")
        return 1

    valid_types = {
        "actor_types": {
            "process": {"aggregation_scopes": ["process_name"]},
            "endpoint": {"aggregation_scopes": []},
            "scope_less": {},
        },
        "aggregation_scopes": {"process_name": {}},
    }
    if errors := aggregation_scope_reference_errors(valid_types):
        print("self-test FAILED: valid scoped/scope-less actor types rejected:", errors)
        return 1
    if errors := network_viewer_actor_type_errors(valid_types):
        print("self-test FAILED: valid endpoint actor type rejected:", errors)
        return 1

    dangling_types = {
        "actor_types": {"endpoint": {"aggregation_scopes": ["endpoint"]}},
        "aggregation_scopes": {},
    }
    expected = ["actor type 'endpoint' references undefined aggregation scope 'endpoint'"]
    if aggregation_scope_reference_errors(dangling_types) != expected:
        print("self-test FAILED: dangling actor aggregation scope was not rejected")
        return 1
    expected = ["endpoint actor type must not declare aggregation scopes"]
    if network_viewer_actor_type_errors(dangling_types) != expected:
        print("self-test FAILED: endpoint aggregation scope was not rejected")
        return 1

    if network_viewer_actor_type_errors({"actor_types": {}}) != ["missing endpoint actor type"]:
        print("self-test FAILED: missing endpoint actor type was not rejected")
        return 1

    for malformed_scopes in (None, 7, "endpoint", {}):
        malformed_types = {
            "actor_types": {"endpoint": {"aggregation_scopes": malformed_scopes}},
            "aggregation_scopes": {},
        }
        expected = ["actor type 'endpoint' aggregation_scopes is not an array"]
        if aggregation_scope_reference_errors(malformed_types) != expected:
            print(
                "self-test FAILED: malformed endpoint aggregation_scopes was not rejected:",
                repr(malformed_scopes))
            return 1

    try:
        import jsonschema
    except ImportError:
        print("self-test FAILED: jsonschema is required to test schema draft selection")
        return 1
    here = os.path.dirname(os.path.abspath(__file__))
    schema_file = os.path.join(
        here, "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json")
    schema = json.load(io.open(schema_file, encoding="utf-8"))
    validator = schema_validator(schema)
    if validator.__class__ is not jsonschema.Draft202012Validator:
        print(
            "self-test FAILED: topology schema did not select Draft 2020-12; got",
            validator.__class__.__name__)
        return 1
    print("self-test OK")
    return 0


def _take_option_value(args, name):
    """Consume the value following --name; rejects a missing value or a value that is another option."""
    i = args.index(name)
    if i + 1 >= len(args) or args[i + 1].startswith("--"):
        print(f"{name} requires a value")
        return None, 2
    value = args[i + 1]
    del args[i:i + 2]
    return value, 0


def parse_view_expectations(args):
    """Pull --mode/--group-by from the argument list; returns (mode, group_by, rc)."""
    expect_mode = None
    expect_group_by = None
    while "--mode" in args:
        expect_mode, rc = _take_option_value(args, "--mode")
        if rc:
            return None, None, rc
    while "--group-by" in args:
        expect_group_by, rc = _take_option_value(args, "--group-by")
        if rc:
            return None, None, rc
    return expect_mode, expect_group_by, 0


def validate_payload(data, schema_file, expect_mode, expect_group_by):
    """Run the JSON schema and semantic checks; returns 0 on success."""
    schema = json.load(io.open(schema_file, encoding='utf-8'))

    try:
        import jsonschema
    except ImportError:
        errors = semantic_checks(data, expect_mode, expect_group_by)
        if errors:
            print("SEMANTIC FAILURES:")
            for e in errors:
                print(" -", e)
            return 1
        print("jsonschema not available; semantic checks passed")
        return 0

    validator = schema_validator(schema)
    errs = sorted(validator.iter_errors(data), key=lambda e: list(e.path))
    if errs:
        print(f"SCHEMA FAILURES ({len(errs)}):")
        for e in errs[:20]:
            print(" -", "/".join(str(p) for p in e.path), ":", e.message[:200])
        if len(errs) > 20:
            print(f" ... and {len(errs) - 20} more")
        return 1

    errors = semantic_checks(data, expect_mode, expect_group_by)
    if errors:
        print("SEMANTIC FAILURES:")
        for e in errors:
            print(" -", e)
        return 1

    d = data["data"]
    print(f"OK: payload validates against {os.path.basename(schema_file)}"
          f" (actors={d['actors'].get('rows')}, links={d['links'].get('rows')}, "
          f"mode={d['view'].get('mode')})")
    return 0


def main(argv):
    args = list(argv)
    if "--self-test" in args:
        return run_self_test()

    expect_mode, expect_group_by, rc = parse_view_expectations(args)
    if rc:
        return rc

    payload_file = args[0] if len(args) > 0 else None
    if payload_file is None or payload_file == "-":
        payload_file = "/dev/stdin"
    here = os.path.dirname(os.path.abspath(__file__))
    schema_file = args[1] if len(args) > 1 else os.path.join(
        here, "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json")

    data = load_payload(payload_file)
    return validate_payload(data, schema_file, expect_mode, expect_group_by)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
