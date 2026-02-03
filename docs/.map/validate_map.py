#!/usr/bin/env python3
"""
Validate map.yaml structure and content.

This script checks:
1. Valid YAML syntax
2. Required fields in meta entries (label, path, edit_url, status)
3. Valid edit_url format (GitHub edit URLs)
4. No duplicate edit_urls
5. Valid status values (Published, Unpublished)
6. Valid integration_placeholder types
7. Proper nesting structure

Exit codes:
  0 - Validation passed
  1 - Validation failed
"""

import sys
import re
from pathlib import Path
from typing import List, Dict, Any, Optional

try:
    import yaml
    from yaml import SafeLoader
except ImportError:
    print("ERROR: PyYAML is required. Install with: pip install pyyaml")
    sys.exit(1)


VALID_STATUSES = {"Published", "Unpublished"}
VALID_INTEGRATION_KINDS = {
    "collectors",
    "exporters", 
    "notifications",
    "authentication",
    "logs",
    "agent_notifications",
    "cloud_notifications"
}
GITHUB_EDIT_URL_PATTERN = re.compile(
    r"^https://github\.com/[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+/edit/[a-zA-Z0-9_.-]+/.+\.(md|mdx)$"
)


class LineTrackingLoader(SafeLoader):
    """Custom YAML loader that tracks line numbers."""
    pass


def construct_with_line(loader, node):
    """Construct a mapping and attach line number info."""
    mapping = loader.construct_mapping(node)
    mapping['__line__'] = node.start_mark.line + 1  # 1-indexed
    return mapping


LineTrackingLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG,
    construct_with_line
)


class ValidationError:
    def __init__(self, path: str, message: str, line: Optional[int] = None):
        self.path = path
        self.message = message
        self.line = line

    def __str__(self):
        if self.line:
            return f"Line {self.line}: [{self.path}] {self.message}"
        return f"[{self.path}] {self.message}"


def get_line(obj: Any) -> Optional[int]:
    """Get the line number from an object if available."""
    if isinstance(obj, dict):
        return obj.get('__line__')
    return None


def validate_meta(meta: Dict[str, Any], node_path: str, edit_urls: Dict[str, int], line: Optional[int], has_integration_placeholder: bool = False) -> List[ValidationError]:
    """
    Validate a meta entry.
    
    Args:
        meta: The meta dictionary to validate
        node_path: The path for error reporting
        edit_urls: Dictionary tracking edit URLs for duplicate detection
        line: Line number for error reporting
        has_integration_placeholder: True if this node has integration_placeholder children
    """
    errors = []
    meta_line = get_line(meta) or line

    # Check required fields
    if "label" not in meta or not meta["label"]:
        errors.append(ValidationError(node_path, "Missing or empty 'label' field", meta_line))

    if "path" not in meta or not meta["path"]:
        errors.append(ValidationError(node_path, "Missing or empty 'path' field", meta_line))

    # Get edit_url and status
    edit_url = meta.get("edit_url")
    status = meta.get("status")
    
    # Nodes with integration placeholders can have null edit_url and status (they're structural/category nodes)
    # All other nodes MUST have both edit_url and status
    if not has_integration_placeholder:
        # Require edit_url for document nodes
        if edit_url is None:
            errors.append(ValidationError(node_path, "Missing 'edit_url' field (required unless node has integration_placeholder child)", meta_line))
        
        # Require status for document nodes  
        if status is None:
            errors.append(ValidationError(node_path, "Missing 'status' field (required unless node has integration_placeholder child)", meta_line))
    
    # Validate edit_url format if present
    if edit_url is not None:
        if not isinstance(edit_url, str):
            errors.append(ValidationError(node_path, f"edit_url must be a string, got {type(edit_url).__name__}", meta_line))
        elif not GITHUB_EDIT_URL_PATTERN.match(edit_url):
            errors.append(ValidationError(
                node_path,
                f"Invalid edit_url format: '{edit_url}'. Must be a GitHub edit URL ending in .md or .mdx",
                meta_line
            ))
        else:
            # Check for duplicates
            if edit_url in edit_urls:
                errors.append(ValidationError(
                    node_path,
                    f"Duplicate edit_url: '{edit_url}' (first seen at line {edit_urls[edit_url]})",
                    meta_line
                ))
            edit_urls[edit_url] = meta_line or 0
    
    # Validate status if present
    if status is not None:
        if not isinstance(status, str):
            errors.append(ValidationError(node_path, f"status must be a string, got {type(status).__name__}", meta_line))
        elif status not in VALID_STATUSES:
            errors.append(ValidationError(
                node_path,
                f"Invalid status '{status}'. Must be one of: {', '.join(VALID_STATUSES)}",
                meta_line
            ))

    # Validate keywords if present
    keywords = meta.get("keywords")
    if keywords is not None:
        if not isinstance(keywords, list):
            errors.append(ValidationError(node_path, f"keywords must be a list, got {type(keywords).__name__}", meta_line))
        else:
            for i, kw in enumerate(keywords):
                if not isinstance(kw, str):
                    errors.append(ValidationError(node_path, f"keywords[{i}] must be a string", meta_line))

    # Validate description if present
    description = meta.get("description")
    if description is not None and not isinstance(description, str):
        errors.append(ValidationError(node_path, f"description must be a string, got {type(description).__name__}", meta_line))

    return errors


def validate_integration_placeholder(node: Dict[str, Any], node_path: str) -> List[ValidationError]:
    """Validate an integration placeholder entry."""
    errors = []
    line = get_line(node)

    if "integration_kind" not in node:
        errors.append(ValidationError(node_path, "integration_placeholder missing 'integration_kind' field", line))
    elif node["integration_kind"] not in VALID_INTEGRATION_KINDS:
        errors.append(ValidationError(
            node_path,
            f"Invalid integration_kind '{node['integration_kind']}'. Must be one of: {', '.join(VALID_INTEGRATION_KINDS)}",
            line
        ))

    return errors


def validate_node(node: Any, path: str, edit_urls: Dict[str, int], depth: int = 0) -> List[ValidationError]:
    """Recursively validate a node in the sidebar tree."""
    errors = []
    line = get_line(node) if isinstance(node, dict) else None

    if not isinstance(node, dict):
        errors.append(ValidationError(path, f"Node must be a dict, got {type(node).__name__}", line))
        return errors

    # Check if it's an integration placeholder
    if node.get("type") == "integration_placeholder":
        errors.extend(validate_integration_placeholder(node, path))
        return errors

    # Regular node must have meta
    if "meta" not in node:
        errors.append(ValidationError(path, "Node missing 'meta' field", line))
        return errors

    meta = node["meta"]
    if not isinstance(meta, dict):
        errors.append(ValidationError(path, f"meta must be a dict, got {type(meta).__name__}", line))
        return errors

    # Build a readable path for error messages
    label = meta.get("label", "???")
    node_path = f"{path}/{label}" if path else label

    # Check if this node has integration placeholder children
    items = node.get("items")
    has_integration_placeholder = False
    if items is not None and isinstance(items, list):
        has_integration_placeholder = any(
            isinstance(item, dict) and item.get("type") == "integration_placeholder"
            for item in items
        )

    errors.extend(validate_meta(meta, node_path, edit_urls, line, has_integration_placeholder=has_integration_placeholder))

    # Validate items if present
    if items is not None:
        if not isinstance(items, list):
            errors.append(ValidationError(node_path, f"items must be a list, got {type(items).__name__}", line))
        else:
            for i, item in enumerate(items):
                errors.extend(validate_node(item, node_path, edit_urls, depth + 1))

    return errors


def validate_sidebar(sidebar: List[Any]) -> List[ValidationError]:
    """Validate the entire sidebar structure."""
    errors = []
    edit_urls: Dict[str, int] = {}  # edit_url -> line number

    if not isinstance(sidebar, list):
        errors.append(ValidationError("root", f"sidebar must be a list, got {type(sidebar).__name__}"))
        return errors

    for i, node in enumerate(sidebar):
        errors.extend(validate_node(node, "", edit_urls, 0))

    return errors


def validate_map_yaml(file_path: str) -> List[ValidationError]:
    """Validate a map.yaml file."""
    errors = []
    path = Path(file_path)

    if not path.exists():
        errors.append(ValidationError(file_path, "File not found"))
        return errors

    # Try to parse YAML with line tracking
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = yaml.load(f, Loader=LineTrackingLoader)
    except yaml.YAMLError as e:
        errors.append(ValidationError(file_path, f"Invalid YAML syntax: {e}"))
        return errors

    if not isinstance(data, dict):
        errors.append(ValidationError(file_path, f"Root must be a dict, got {type(data).__name__}"))
        return errors

    if "sidebar" not in data:
        errors.append(ValidationError(file_path, "Missing 'sidebar' key at root"))
        return errors

    errors.extend(validate_sidebar(data["sidebar"]))

    return errors


def main():
    import argparse

    parser = argparse.ArgumentParser(description="Validate map.yaml structure and content")
    parser.add_argument(
        "file",
        nargs="?",
        default="map.yaml",
        help="Path to map.yaml file (default: map.yaml)"
    )
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Treat warnings as errors"
    )
    args = parser.parse_args()

    print(f"Validating {args.file}...")

    errors = validate_map_yaml(args.file)

    if errors:
        print(f"\n❌ Validation FAILED with {len(errors)} error(s):\n")
        for error in errors:
            print(f"  • {error}")
        sys.exit(1)
    else:
        print("✅ Validation PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
