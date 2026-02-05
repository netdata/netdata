#!/usr/bin/env python3
"""
Verify that the nesting structure in map.yaml matches the structure implied by map.csv.

CSV nesting is defined by learn_rel_path:
- "A" = top level
- "A/B" = B is under A
- "A/B/C" = C is under B which is under A

YAML nesting is explicit via items arrays.
YAML paths are now segment-only (e.g., "Installation" not "Netdata Agent/Installation").
Full paths are reconstructed by walking the tree and accumulating parent segments.
"""

import csv
from typing import Dict, List, Set, Tuple
import yaml

PLACEHOLDER_SENTINELS = {
    "authentication_integrations",
    "collectors_integrations",
    "agent_notifications_integrations",
    "cloud_notifications_integrations",
    "exporters_integrations",
    "logs_integrations",
}


def build_csv_structure(csv_file: str) -> Tuple[Dict[str, Set[str]], Set[str]]:
    """
    Build a parent->children mapping from CSV paths.
    Returns:
        - dict where key is parent path, value is set of direct child paths
        - set of paths that appear multiple times (duplicate paths)
    """
    parent_to_children: Dict[str, Set[str]] = {}
    all_paths: List[str] = []
    path_counts: Dict[str, int] = {}

    with open(csv_file, "r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            # Skip empty rows and placeholders
            custom_edit_url = (row.get("custom_edit_url") or "").strip()
            if custom_edit_url in PLACEHOLDER_SENTINELS:
                continue
            if not any((val or "").strip() for val in row.values()):
                continue

            path = (row.get("learn_rel_path") or "").strip()
            if not path or path == "root":
                continue

            all_paths.append(path)
            path_counts[path] = path_counts.get(path, 0) + 1

    # Paths that appear more than once
    duplicate_paths = {p for p, count in path_counts.items() if count > 1}

    # Build parent-child relationships from unique paths
    unique_paths = set(all_paths)
    for path in unique_paths:
        parts = path.split("/")
        if len(parts) == 1:
            # Top-level item, parent is "root"
            parent_to_children.setdefault("root", set()).add(path)
        else:
            # Has a parent
            parent_path = "/".join(parts[:-1])
            parent_to_children.setdefault(parent_path, set()).add(path)

    # Duplicate paths are already handled in the main loop - no need for special handling

    return parent_to_children, duplicate_paths


def build_yaml_structure(yaml_file: str) -> Dict[str, Set[str]]:
    """
    Build a parent->children mapping from YAML structure.
    Returns dict where key is parent path, value is set of direct child paths.

    Path reconstruction rule:
    - Nodes WITH items array → use 'path' if present, else 'label' as segment
    - Nodes WITHOUT items → leaves that belong to their parent's path
    """
    parent_to_children: Dict[str, Set[str]] = {}

    with open(yaml_file, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    def process_items(items: List[dict], parent_full_path: str):
        """Process items with correct path logic."""
        for item in items:
            # Skip integration placeholders
            if item.get("type") == "integration_placeholder":
                continue

            meta = item.get("meta", {})
            label = meta.get("label", "")
            has_explicit_path = "path" in meta
            has_items = "items" in item

            if has_explicit_path:
                # Node with explicit path - defines its own hierarchy level
                path_segment = meta["path"]
                if parent_full_path == "root":
                    full_path = path_segment
                else:
                    full_path = f"{parent_full_path}/{path_segment}"

                if full_path and full_path != "root":
                    parent_to_children.setdefault(parent_full_path, set()).add(
                        full_path
                    )

                # Recurse into children if any
                if has_items:
                    process_items(item["items"], full_path)
            elif has_items:
                # Structural parent (no path, has items) - label is the segment
                if parent_full_path == "root":
                    full_path = label
                else:
                    full_path = f"{parent_full_path}/{label}"

                if full_path and full_path != "root":
                    parent_to_children.setdefault(parent_full_path, set()).add(
                        full_path
                    )

                process_items(item["items"], full_path)
            else:
                # Leaf node (no path, no items) - construct full path from parent + label
                if parent_full_path == "root":
                    full_path = label
                else:
                    full_path = f"{parent_full_path}/{label}"

                if full_path and full_path != "root":
                    parent_to_children.setdefault(parent_full_path, set()).add(
                        full_path
                    )

    sidebar = data.get("sidebar", [])
    process_items(sidebar, "root")

    return parent_to_children


def compare_structures(
    csv_struct: Dict[str, Set[str]],
    yaml_struct: Dict[str, Set[str]],
    structural_paths: Set[str],
) -> Tuple[bool, List[str]]:
    """Compare CSV and YAML structures, return (is_match, list of differences).

    Args:
        csv_struct: parent->children mapping from CSV
        yaml_struct: parent->children mapping from YAML
        structural_paths: paths that are auto-created structural parents (OK to be extra in YAML)
    """
    differences = []

    all_parents = set(csv_struct.keys()) | set(yaml_struct.keys())

    for parent in sorted(all_parents):
        csv_children = csv_struct.get(parent, set())
        yaml_children = yaml_struct.get(parent, set())

        # Children in CSV but not in YAML
        missing_in_yaml = csv_children - yaml_children
        if missing_in_yaml:
            for child in sorted(missing_in_yaml):
                differences.append(
                    f"MISSING in YAML: '{child}' should be under '{parent}'"
                )

        # Children in YAML but not in CSV
        extra_in_yaml = yaml_children - csv_children
        if extra_in_yaml:
            for child in sorted(extra_in_yaml):
                # Skip if this is an auto-created structural parent
                if child in structural_paths:
                    continue
                differences.append(
                    f"EXTRA in YAML: '{child}' under '{parent}' (not in CSV)"
                )

    return len(differences) == 0, differences


def get_all_paths_csv(csv_file: str) -> Set[str]:
    """Get all unique paths from CSV."""
    paths = set()
    with open(csv_file, "r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            custom_edit_url = (row.get("custom_edit_url") or "").strip()
            if custom_edit_url in PLACEHOLDER_SENTINELS:
                continue
            if not any((val or "").strip() for val in row.values()):
                continue
            path = (row.get("learn_rel_path") or "").strip()
            if path and path != "root":
                paths.add(path)
    return paths


def get_all_paths_yaml(yaml_file: str) -> Set[str]:
    """Get all unique full paths from YAML by reconstructing them.

    Path reconstruction rule:
    - Nodes WITH items → use 'path' if present, else 'label' as segment
    - Nodes WITHOUT items → leaves at parent's path level
    """
    paths = set()

    with open(yaml_file, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    def collect_paths(items: List[dict], parent_full_path: str):
        """Collect paths with correct logic."""
        for item in items:
            if item.get("type") == "integration_placeholder":
                continue
            meta = item.get("meta", {})
            label = meta.get("label", "")
            has_explicit_path = "path" in meta
            has_items = "items" in item

            if has_explicit_path:
                # Node with path - defines its own level
                path_segment = meta["path"]
                if parent_full_path == "root":
                    full_path = path_segment
                else:
                    full_path = f"{parent_full_path}/{path_segment}"

                if full_path and full_path != "root":
                    paths.add(full_path)
                if has_items:
                    collect_paths(item["items"], full_path)
            elif has_items:
                # Structural parent - label is segment
                if parent_full_path == "root":
                    full_path = label
                else:
                    full_path = f"{parent_full_path}/{label}"

                if full_path and full_path != "root":
                    paths.add(full_path)
                collect_paths(item["items"], full_path)
            else:
                # Leaf node - construct full path from parent + label
                if parent_full_path == "root":
                    full_path = label
                else:
                    full_path = f"{parent_full_path}/{label}"

                if full_path and full_path != "root":
                    paths.add(full_path)

    collect_paths(data.get("sidebar", []), "root")
    return paths


def main():
    csv_file = "map.csv"
    yaml_file = "map.yaml"

    print("=" * 60)
    print("Verifying CSV <-> YAML Nesting Structure")
    print("=" * 60)

    # Check path counts
    csv_paths = get_all_paths_csv(csv_file)
    yaml_paths = get_all_paths_yaml(yaml_file)

    print(f"\nUnique paths in CSV:  {len(csv_paths)}")
    print(f"Unique paths in YAML: {len(yaml_paths)}")

    # Paths in CSV but not YAML
    missing = csv_paths - yaml_paths
    if missing:
        print(f"\n⚠️  Paths in CSV but MISSING from YAML ({len(missing)}):")
        for p in sorted(missing)[:10]:
            print(f"   - {p}")
        if len(missing) > 10:
            print(f"   ... and {len(missing) - 10} more")

    # Paths in YAML but not CSV (structural parents are OK)
    extra = yaml_paths - csv_paths
    if extra:
        print(f"\n📁 Structural paths in YAML (auto-created, not in CSV): {len(extra)}")
        for p in sorted(extra):
            print(f"   - {p}")

    # Compare nesting structures
    print("\n" + "-" * 60)
    print("Comparing parent-child relationships...")
    print("-" * 60)

    csv_struct, duplicate_paths = build_csv_structure(csv_file)
    yaml_struct = build_yaml_structure(yaml_file)

    if duplicate_paths:
        print(
            f"\n📋 Paths appearing multiple times in CSV (grouped in YAML): {len(duplicate_paths)}"
        )
        for p in sorted(duplicate_paths)[:5]:
            print(f"   - {p}")
        if len(duplicate_paths) > 5:
            print(f"   ... and {len(duplicate_paths) - 5} more")

    # Structural paths are those in YAML but not in CSV (auto-created parents)
    structural_paths = yaml_paths - csv_paths

    is_match, differences = compare_structures(
        csv_struct, yaml_struct, structural_paths
    )

    if is_match:
        print("\n✅ Nesting structure matches perfectly!")
    else:
        print(f"\n❌ Found {len(differences)} nesting differences:")
        for diff in differences[:20]:
            print(f"   {diff}")
        if len(differences) > 20:
            print(f"   ... and {len(differences) - 20} more")

    # Summary
    print("\n" + "=" * 60)
    if is_match and not missing:
        print("✅ VALIDATION PASSED: CSV and YAML structures match!")
    else:
        print("❌ VALIDATION FAILED: Structures do not match")
    print("=" * 60)

    return 0 if (is_match and not missing) else 1


if __name__ == "__main__":
    exit(main())
