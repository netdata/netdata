#!/usr/bin/env python3
"""
Validate map.yaml against JSON Schema with additional custom rules.

This validator uses JSON Schema for structure validation and adds custom
checks for rules that can't be expressed in JSON Schema:
- Nodes without children (leaves) must have an edit_url
- No duplicate edit_urls
- A netdata/netdata edit_url must name a file in this repository

Path reconstruction rule (for ingest):
- Nodes WITH items array → label is the path segment (they define hierarchy)
- Nodes WITHOUT items → leaves that belong to their parent's path

Exit codes:
  0 - Validation passed
  1 - Validation failed
"""

import re
import sys
import json
from pathlib import Path
from typing import List, Dict, Any, Iterator, Tuple

try:
    from ruamel.yaml import YAML
except ImportError:
    print("ERROR: ruamel.yaml is required. Install with: pip install ruamel.yaml")
    sys.exit(1)

try:
    import jsonschema
    from jsonschema import Draft7Validator
except ImportError:
    print("ERROR: jsonschema is required. Install with: pip install jsonschema")
    sys.exit(1)


class MapValidationError:
    def __init__(self, path: str, message: str):
        self.path = path
        self.message = message

    def __str__(self):
        return f"[{self.path}] {self.message}"


def load_schema(schema_path: str) -> dict:
    """Load JSON Schema from file."""
    with open(schema_path, "r", encoding="utf-8") as f:
        return json.load(f)


def load_yaml(yaml_path: str) -> dict:
    """Load YAML file."""
    yaml = YAML(typ="safe")
    with open(yaml_path, "r", encoding="utf-8") as f:
        return yaml.load(f)


def format_schema_error(error: jsonschema.ValidationError) -> str:
    """Format a JSON Schema validation error nicely."""
    path = (
        ".".join(str(p) for p in error.absolute_path) if error.absolute_path else "root"
    )
    # For oneOf/anyOf, jsonschema often reports a generic message like
    # "is not valid under any of the given schemas" and puts the real
    # problems into error.context. Surface the most relevant sub-error(s)
    # so users see actionable messages without needing debug output.
    validator = getattr(error, "validator", None)
    if validator in ("oneOf", "anyOf") and getattr(error, "context", None):
        suberrors = list(error.context)

        def _path_depth(e: jsonschema.ValidationError) -> int:
            try:
                return len(list(e.absolute_path))
            except Exception:
                return 0

        # Prefer the deepest (most specific) sub-error.
        suberrors.sort(key=_path_depth, reverse=True)
        primary = suberrors[0]
        sub_path = (
            ".".join(str(p) for p in primary.absolute_path)
            if primary.absolute_path
            else path
        )
        # Collect up to a couple of distinct messages for context.
        messages = [primary.message]
        for sub in suberrors[1:3]:
            if sub.message not in messages:
                messages.append(sub.message)
        details = "; ".join(messages)
        return f"[{sub_path}] {details} (while validating {validator} at {path})"
    return f"[{path}] {error.message}"


# Only this repository's rows can be checked for a file; other repositories
# are not part of this checkout.
NETDATA_EDIT_URL = re.compile(
    r"^https://github\.com/netdata/netdata/edit/[^/]+/(?P<path>.+)$"
)


def iter_sidebar_nodes(
    nodes: Any, path: str = ""
) -> Iterator[Tuple[str, Dict[str, Any], Dict[str, Any]]]:
    """
    Yield (node_path, node, meta) for every sidebar node, depth-first.

    node_path joins the labels from the root. Integration placeholders and
    nodes whose meta is not a mapping are skipped together with their children.
    """
    if not isinstance(nodes, list):
        return
    for node in nodes:
        if not isinstance(node, dict) or node.get("type") == "integration_placeholder":
            continue
        meta = node.get("meta", {})
        if not isinstance(meta, dict):
            continue
        label = meta.get("label", "???")
        node_path = f"{path}/{label}" if path else label
        yield node_path, node, meta
        yield from iter_sidebar_nodes(node.get("items", []), node_path)


def check_duplicate_edit_urls(
    sidebar: List[Any], errors: List[MapValidationError]
) -> None:
    """Check that no two nodes share an edit_url."""
    first_seen: Dict[str, str] = {}
    for node_path, _, meta in iter_sidebar_nodes(sidebar):
        edit_url = meta.get("edit_url")
        if not edit_url or not isinstance(edit_url, str):
            continue
        if edit_url in first_seen:
            errors.append(
                MapValidationError(
                    node_path,
                    f"Duplicate edit_url: '{edit_url}' (first seen at {first_seen[edit_url]})",
                )
            )
        else:
            first_seen[edit_url] = node_path


def check_integration_placeholder_rule(
    sidebar: List[Any], errors: List[MapValidationError]
) -> None:
    """
    Check that leaf nodes have edit_url.

    Custom rule:
    - Structural nodes (with children) may omit edit_url.
    - Leaf nodes (without children) must provide edit_url.
    """
    for node_path, node, meta in iter_sidebar_nodes(sidebar):
        items = node.get("items", [])
        has_items = isinstance(items, list) and len(items) > 0
        if meta.get("edit_url") is None and not has_items:
            errors.append(
                MapValidationError(
                    node_path,
                    "Missing 'edit_url' field (only allowed for structural nodes with children)",
                )
            )


def check_edit_url_files_exist(
    sidebar: List[Any], repo_root: Path, errors: List[MapValidationError]
) -> None:
    """
    Check that every netdata/netdata edit_url names a file in this repository.

    Ingest publishes a row only when a source file matches its edit_url and
    drops the row silently otherwise, so a deleted or renamed page would
    vanish from Learn without an error.
    """
    root = repo_root.resolve()
    for node_path, _, meta in iter_sidebar_nodes(sidebar):
        edit_url = meta.get("edit_url")
        match = NETDATA_EDIT_URL.match(edit_url) if isinstance(edit_url, str) else None
        if not match:
            continue
        relative_path = match.group("path")
        target = (root / relative_path).resolve()
        if not (target.is_relative_to(root) and target.is_file()):
            errors.append(
                MapValidationError(
                    node_path,
                    f"edit_url names no file in this repository: {relative_path} "
                    "(update edit_url to the file's current path, or remove the row of a "
                    "retired page: docs/.map/README.md#unpublishing-files)",
                )
            )


def validate_with_schema(data: dict, schema: dict) -> Tuple[bool, List[str]]:
    """Validate data against JSON Schema."""
    validator = Draft7Validator(schema)
    errors = []

    for error in validator.iter_errors(data):
        errors.append(format_schema_error(error))

    return len(errors) == 0, errors


def validate_custom_rules(
    data: dict, repo_root: Path
) -> Tuple[bool, List[MapValidationError]]:
    """Apply custom validation rules not expressible in JSON Schema."""
    errors: List[MapValidationError] = []

    # Guard against non-dict YAML root
    if not isinstance(data, dict):
        return False, [
            MapValidationError(
                "root", f"YAML root must be a dictionary, got {type(data).__name__}"
            )
        ]

    sidebar = data.get("sidebar", [])
    if not isinstance(sidebar, list):
        return False, [MapValidationError("root", "sidebar must be a list")]

    check_duplicate_edit_urls(sidebar, errors)
    check_integration_placeholder_rule(sidebar, errors)
    check_edit_url_files_exist(sidebar, repo_root, errors)

    return len(errors) == 0, errors


def main():
    """Main validation routine."""
    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parents[1]
    yaml_path = script_dir / "map.yaml"
    schema_path = script_dir / "map.schema.json"

    if not yaml_path.exists():
        print(f"ERROR: {yaml_path} not found")
        sys.exit(1)

    if not schema_path.exists():
        print(f"ERROR: {schema_path} not found")
        sys.exit(1)

    print("Validating map.yaml...")
    print()

    # Load files
    try:
        data = load_yaml(str(yaml_path))
        schema = load_schema(str(schema_path))
    except Exception as e:
        print(f"ERROR loading files: {e}")
        sys.exit(1)

    # Guard against non-dict YAML root
    if not isinstance(data, dict):
        print(f"❌ Validation FAILED:\n")
        print(f"  • YAML root must be a dictionary, got {type(data).__name__}")
        sys.exit(1)

    all_errors = []

    # Validate against JSON Schema
    schema_valid, schema_errors = validate_with_schema(data, schema)
    if not schema_valid:
        all_errors.append("Schema validation errors:")
        all_errors.extend(f"  • {err}" for err in schema_errors)

    # Apply custom rules
    custom_valid, custom_errors = validate_custom_rules(data, repo_root)
    if not custom_valid:
        if all_errors:
            all_errors.append("")
        all_errors.append("Custom rule violations:")
        all_errors.extend(f"  • {err}" for err in custom_errors)

    # Report results
    if all_errors:
        print("❌ Validation FAILED:\n")
        print("\n".join(all_errors))
        sys.exit(1)
    else:
        print("✅ Validation PASSED")
        print(f"   - Validated against schema: {schema_path.name}")
        print(f"   - All custom rules satisfied")
        sys.exit(0)


if __name__ == "__main__":
    main()
