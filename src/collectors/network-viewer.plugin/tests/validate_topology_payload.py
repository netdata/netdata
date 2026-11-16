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
    """Require well-formed actor scope memberships and declared scope IDs."""
    if not isinstance(types, dict):
        return ["types is not an object"]

    actor_types = types.get("actor_types")
    aggregation_scopes = types.get("aggregation_scopes", {})
    if not isinstance(actor_types, dict):
        return ["types.actor_types is not an object"]
    if not isinstance(aggregation_scopes, dict):
        return ["types.aggregation_scopes is not an object"]

    errors = []
    for actor_type_id, actor_type in actor_types.items():
        if not isinstance(actor_type, dict):
            errors.append(f"actor type {actor_type_id!r} is not an object")
            continue
        actor_scopes = actor_type.get("aggregation_scopes", [])
        if not isinstance(actor_scopes, list):
            errors.append(
                f"actor type {actor_type_id!r} aggregation_scopes is not an array")
            continue
        seen = set()
        for scope_id in actor_scopes:
            if not isinstance(scope_id, str) or not scope_id:
                errors.append(
                    f"actor type {actor_type_id!r} aggregation_scopes members must be non-empty strings")
                continue
            if scope_id in seen:
                errors.append(f"actor type {actor_type_id!r} repeats aggregation scope {scope_id!r}")
            seen.add(scope_id)
            if scope_id not in aggregation_scopes:
                errors.append(
                    f"actor type {actor_type_id!r} references undefined "
                    f"aggregation scope {scope_id!r}")
    return errors


def actor_search_label_key_errors(types):
    """Validate optional search label keys without normalizing literal keys."""
    if not isinstance(types, dict) or not isinstance(types.get("actor_types"), dict):
        return []

    errors = []
    for actor_type_id, actor_type in types["actor_types"].items():
        if not isinstance(actor_type, dict):
            continue
        search = actor_type.get("search")
        # A null policy is absent in semantic-only validation, matching the Go helper.
        if search is None:
            continue
        path = f"actor type {actor_type_id!r} search"
        if not isinstance(search, dict):
            errors.append(f"{path} is not an object")
            continue
        if "label_keys" not in search:
            continue
        keys = search["label_keys"]
        if not isinstance(keys, list):
            errors.append(f"{path}.label_keys is not an array")
            continue
        seen = set()
        for key in keys:
            if not isinstance(key, str) or not key:
                errors.append(f"{path}.label_keys members must be non-empty strings")
                continue
            if key in seen:
                errors.append(f"{path}.label_keys repeats {key!r}")
            seen.add(key)
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
    if not isinstance(d.get("types"), dict) or "actor_types" not in d["types"]:
        errors.append("missing types.actor_types registry")
    else:
        errors.extend(aggregation_scope_reference_errors(d["types"]))
        errors.extend(network_viewer_actor_type_errors(d["types"]))
        errors.extend(actor_search_label_key_errors(d["types"]))
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
    if run_payload_validation_self_test(schema_file):
        return 1
    print("self-test OK")
    return 0


def self_test_payload_result(data, schema_file, missing_dependency, expected, prefix):
    """Check status and diagnostic prefix to pin validation-branch precedence."""
    from contextlib import redirect_stdout
    from unittest.mock import patch

    unavailable = {"jsonschema": None} if missing_dependency else {}
    with patch.dict(sys.modules, unavailable), redirect_stdout(io.StringIO()) as output:
        result = validate_payload(data, schema_file, None, None)
    if result != expected or not output.getvalue().startswith(prefix):
        print("self-test FAILED: payload validation result", result, output.getvalue())
        return False
    return True


def run_payload_validation_self_test(schema_file):
    """Exercise the actual validator with and without its optional dependency."""
    fixture = os.path.join(
        os.path.dirname(schema_file), "..", "go", "tools", "functions-validation",
        "fixtures", "topology-v1", "network-connections.json")
    payload = load_payload(fixture)
    payload["v"] = 3
    for missing in (False, True):
        cases = (
            (payload, 0, "jsonschema not available;" if missing else "OK:"),
            (dict(payload, v=2), 1, "SEMANTIC FAILURES:"),
            (dict(payload, unexpected=True), 0 if missing else 1,
             "jsonschema not available;" if missing else "SCHEMA FAILURES"),
            (dict(payload, v=2, unexpected=True), 1,
             "SEMANTIC FAILURES:" if missing else "SCHEMA FAILURES"),
        )
        for data, expected, prefix in cases:
            if not self_test_payload_result(data, schema_file, missing, expected, prefix):
                return 1
    return (run_scope_validation_self_test(payload, schema_file)
            or run_search_validation_self_test(payload, schema_file)
            or run_schema_errors_self_test(payload))


def run_scope_validation_self_test(payload, schema_file):
    """Reject malformed scope metadata even when JSON Schema is unavailable."""
    import copy

    bad_objects = (None, True, 7, 1.5, "scope", [])
    cases = (
        (("types",), bad_objects, True),
        (("types", "actor_types"), bad_objects, True),
        (("types", "aggregation_scopes"), bad_objects, True),
        (("types", "actor_types", "process"), bad_objects, True),
        (("types", "actor_types", "process", "aggregation_scopes"),
         (None, True, 7, 1.5, "pid", {}, [None], [True], [7], [1.5], [{}], [[]], [""],
          ["pid", "pid"]), True),
        (("types", "actor_types", "process", "aggregation_scopes"), (["undefined_scope"],), False),
    )
    for path, values, schema_rejects in cases:
        for value in values:
            data = copy.deepcopy(payload)
            target = data["data"]
            for key in path[:-1]:
                target = target[key]
            target[path[-1]] = value
            for missing in (False, True):
                prefix = "SCHEMA FAILURES" if schema_rejects and not missing else "SEMANTIC FAILURES:"
                if not self_test_payload_result(data, schema_file, missing, 1, prefix):
                    print("scope case:", path, repr(value), "missing dependency:", missing)
                    return 1

    for omit_memberships in (False, True):
        for omit_registry in (False, True):
            data = copy.deepcopy(payload)
            types = data["data"]["types"]
            for actor_type in types["actor_types"].values():
                actor_type["aggregation_scopes"] = []
                if omit_memberships:
                    actor_type.pop("aggregation_scopes")
            types["aggregation_scopes"] = {}
            if omit_registry:
                types.pop("aggregation_scopes")
            for missing in (False, True):
                prefix = "jsonschema not available;" if missing else "OK:"
                if not self_test_payload_result(data, schema_file, missing, 0, prefix):
                    return 1
    return 0


def run_search_validation_self_test(payload, schema_file):
    """Validate literal label keys independently of presentation and schema availability."""
    import copy

    omitted = object()
    cases = [
        (omitted, 0, 0), ({}, 0, 0), ({"label_keys": []}, 0, 0),
        ({"label_keys": ["_hostname", "_os", "_labels", "app/name", "主机", " ", "\t",
                         " _hostname ", "é", "e\u0301"]}, 0, 0),
        (None, 1, 0), ({"enabled": 7}, 1, 0), ({"columns": None}, 1, 0),
        ({"enabled": False, "label_keys": [""]}, 1, 1),
    ]
    cases.extend((value, 1, 1) for value in (False, 7, 1.5, "", []))
    invalid_keys = (None, False, 7, 1.5, "", {}, [""], ["_hostname", "_hostname"],
                    [7], [None], [True], [{}], [[]], [" ", " "])
    cases.extend(({"label_keys": value}, 1, 1) for value in invalid_keys)
    for search, schema_result, fallback_result in cases:
        for omit_presentation in (False, True):
            data = copy.deepcopy(payload)
            actor_type = data["data"]["types"]["actor_types"]["process"]
            if omit_presentation:
                actor_type.pop("presentation", None)
            if search is omitted:
                actor_type.pop("search", None)
            else:
                actor_type["search"] = search
            for missing in (False, True):
                expected = fallback_result if missing else schema_result
                prefix = "jsonschema not available;" if missing else "OK:"
                if expected:
                    prefix = "SEMANTIC FAILURES:" if missing else "SCHEMA FAILURES"
                if not self_test_payload_result(data, schema_file, missing, expected, prefix):
                    print("search case:", repr(search), "missing dependency:", missing,
                          "without presentation:", omit_presentation)
                    return 1
    return 0


def run_schema_errors_self_test(payload):
    """Schema file errors must not be mistaken for a missing optional library."""
    import tempfile
    from contextlib import redirect_stdout
    from unittest.mock import patch
    from jsonschema.exceptions import SchemaError

    cases = (("{", False, json.JSONDecodeError), ("{", True, json.JSONDecodeError),
             ('{"type":"invalid"}', False, SchemaError), ('{"type":"invalid"}', True, None))
    for content, missing, exception in cases:
        with tempfile.TemporaryDirectory() as directory:
            schema_file = os.path.join(directory, "schema.json")
            with io.open(schema_file, "w", encoding="utf-8") as output:
                output.write(content)
            if exception is None:
                if not self_test_payload_result(payload, schema_file, missing, 0, "jsonschema not available;"):
                    return 1
                continue
            unavailable = {"jsonschema": None} if missing else {}
            try:
                with patch.dict(sys.modules, unavailable), redirect_stdout(io.StringIO()):
                    validate_payload(payload, schema_file, None, None)
            except exception:
                continue
            print("self-test FAILED: schema file did not raise", exception.__name__)
            return 1
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
        validator = schema_validator(schema)
    except ImportError:
        errors = semantic_checks(data, expect_mode, expect_group_by)
        if errors:
            print("SEMANTIC FAILURES:")
            for e in errors:
                print(" -", e)
            return 1
        print("jsonschema not available; semantic checks passed")
        return 0

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
