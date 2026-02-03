#!/usr/bin/env python3
"""
Validate map.yaml against JSON Schema with additional custom rules.

This validator uses JSON Schema for structure validation and adds custom
checks for rules that can't be expressed in JSON Schema, such as:
- Nodes with integration_placeholder children can omit edit_url and status
- No duplicate edit_urls

Exit codes:
  0 - Validation passed
  1 - Validation failed
"""

import sys
import json
from pathlib import Path
from typing import List, Dict, Any, Tuple

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


class ValidationError:
    def __init__(self, path: str, message: str):
        self.path = path
        self.message = message

    def __str__(self):
        return f"[{self.path}] {self.message}"


def load_schema(schema_path: str) -> dict:
    """Load JSON Schema from file."""
    with open(schema_path, 'r', encoding='utf-8') as f:
        return json.load(f)


def load_yaml(yaml_path: str) -> dict:
    """Load YAML file."""
    yaml = YAML(typ='safe')
    with open(yaml_path, 'r', encoding='utf-8') as f:
        return yaml.load(f)


def format_schema_error(error: jsonschema.ValidationError) -> str:
    """Format a JSON Schema validation error nicely."""
    path = '.'.join(str(p) for p in error.absolute_path) if error.absolute_path else 'root'
    return f"[{path}] {error.message}"


def check_has_integration_placeholder(items: List[Any]) -> bool:
    """Check if a node's items contain an integration_placeholder."""
    if not isinstance(items, list):
        return False
    return any(
        isinstance(item, dict) and item.get('type') == 'integration_placeholder'
        for item in items
    )


def check_duplicate_edit_urls(node: Any, path: str, edit_urls: Dict[str, str], errors: List[ValidationError]) -> None:
    """Recursively check for duplicate edit_urls."""
    if not isinstance(node, dict):
        return
    
    # Skip integration placeholders
    if node.get('type') == 'integration_placeholder':
        return
    
    # Check meta
    meta = node.get('meta', {})
    if isinstance(meta, dict):
        label = meta.get('label', '???')
        node_path = f"{path}/{label}" if path else label
        edit_url = meta.get('edit_url')
        
        if edit_url and isinstance(edit_url, str):
            if edit_url in edit_urls:
                errors.append(ValidationError(
                    node_path,
                    f"Duplicate edit_url: '{edit_url}' (first seen at {edit_urls[edit_url]})"
                ))
            else:
                edit_urls[edit_url] = node_path
        
        # Recurse into children
        items = node.get('items', [])
        if isinstance(items, list):
            for item in items:
                check_duplicate_edit_urls(item, node_path, edit_urls, errors)


def check_integration_placeholder_rule(node: Any, path: str, errors: List[ValidationError]) -> None:
    """
    Check that nodes without edit_url/status have integration_placeholder children.
    
    Custom rule: A node can only have null/missing edit_url and status if it has
    at least one integration_placeholder child.
    """
    if not isinstance(node, dict):
        return
    
    # Skip integration placeholders themselves
    if node.get('type') == 'integration_placeholder':
        return
    
    meta = node.get('meta', {})
    if not isinstance(meta, dict):
        return
    
    label = meta.get('label', '???')
    node_path = f"{path}/{label}" if path else label
    edit_url = meta.get('edit_url')
    status = meta.get('status')
    items = node.get('items', [])
    
    # If edit_url or status is missing/null, check if there's an integration placeholder
    if (edit_url is None or status is None):
        if not check_has_integration_placeholder(items):
            if edit_url is None:
                errors.append(ValidationError(
                    node_path,
                    "Missing 'edit_url' field (only allowed for nodes with integration_placeholder children)"
                ))
            if status is None:
                errors.append(ValidationError(
                    node_path,
                    "Missing 'status' field (only allowed for nodes with integration_placeholder children)"
                ))
    
    # Recurse into children
    if isinstance(items, list):
        for item in items:
            check_integration_placeholder_rule(item, node_path, errors)


def validate_with_schema(data: dict, schema: dict) -> Tuple[bool, List[str]]:
    """Validate data against JSON Schema."""
    validator = Draft7Validator(schema)
    errors = []
    
    for error in validator.iter_errors(data):
        errors.append(format_schema_error(error))
    
    return len(errors) == 0, errors


def validate_custom_rules(data: dict) -> Tuple[bool, List[ValidationError]]:
    """Apply custom validation rules not expressible in JSON Schema."""
    errors: List[ValidationError] = []
    edit_urls: Dict[str, str] = {}
    
    # Guard against non-dict YAML root
    if not isinstance(data, dict):
        return False, [ValidationError('root', f'YAML root must be a dictionary, got {type(data).__name__}')]
    
    sidebar = data.get('sidebar', [])
    if not isinstance(sidebar, list):
        return False, [ValidationError('root', 'sidebar must be a list')]
    
    # Check for duplicate edit_urls
    for node in sidebar:
        check_duplicate_edit_urls(node, '', edit_urls, errors)
    
    # Check integration placeholder rule
    for node in sidebar:
        check_integration_placeholder_rule(node, '', errors)
    
    return len(errors) == 0, errors


def main():
    """Main validation routine."""
    script_dir = Path(__file__).parent
    yaml_path = script_dir / 'map.yaml'
    schema_path = script_dir / 'map.schema.json'
    
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
    custom_valid, custom_errors = validate_custom_rules(data)
    if not custom_valid:
        if all_errors:
            all_errors.append("")
        all_errors.append("Custom rule violations:")
        all_errors.extend(f"  • {err}" for err in custom_errors)
    
    # Report results
    if all_errors:
        print("❌ Validation FAILED:\n")
        print('\n'.join(all_errors))
        sys.exit(1)
    else:
        print("✅ Validation PASSED")
        print(f"   - Validated against schema: {schema_path.name}")
        print(f"   - All custom rules satisfied")
        sys.exit(0)


if __name__ == '__main__':
    main()
