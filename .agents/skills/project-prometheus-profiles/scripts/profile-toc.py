#!/usr/bin/env python3
"""Render the operator-facing chart-family table of contents for a Prometheus profile.

This is a design-review helper, not a validator gate. It intentionally reports UX warnings
that require human judgement.
"""

from __future__ import annotations

import argparse
from dataclasses import dataclass, field
from pathlib import Path
import sys

import yaml

DEFAULT_PRIORITY = 70000
LARGE_LEAF_CONTEXTS = 15


@dataclass
class Chart:
    context: str
    priority: int


@dataclass
class Node:
    name: str
    charts: list[Chart] = field(default_factory=list)
    children: dict[str, "Node"] = field(default_factory=dict)

    @property
    def context_count(self) -> int:
        return len({chart.context for chart in self.charts})


def normalize(value: str | None) -> str:
    return (value or "").strip()


def build_tree(profile: dict) -> Node:
    template = profile.get("template")
    if not isinstance(template, dict):
        raise ValueError("profile has no template mapping")

    root = Node(normalize(template.get("family")) or "(root)")
    groups = template.get("groups")
    if not isinstance(groups, list):
        return root

    def add_group(group: object, parent: Node, context_parts: list[str]) -> None:
        if not isinstance(group, dict):
            return

        family = normalize(group.get("family"))
        node = parent.children.setdefault(family, Node(family))
        namespace = normalize(group.get("context_namespace"))
        child_context_parts = context_parts + ([namespace] if namespace else [])

        for chart in group.get("charts") or []:
            if not isinstance(chart, dict):
                continue
            context = ".".join(part for part in (*child_context_parts, normalize(chart.get("context"))) if part)
            if not context:
                context = "(no context)"
            node.charts.append(Chart(context, int(chart.get("priority") or DEFAULT_PRIORITY)))

        for child in group.get("groups") or []:
            add_group(child, node, child_context_parts)

    for group in groups:
        add_group(group, root, [])
    return root


def sort_key(item: tuple[str, Node]) -> tuple[int, int, str]:
    name, node = item
    priorities = [chart.priority for chart in node.charts]
    minimum = min(priorities, default=DEFAULT_PRIORITY)
    return (minimum, -len(name), name)


def render(node: Node, depth: int = 0) -> list[str]:
    lines: list[str] = []
    indent = "  " * depth
    if depth == 0:
        display_name = node.name or "(application root)"
    else:
        display_name = node.name or "(unnamed)"
    leaf_contexts = node.context_count

    if depth == 0:
        title = f"{display_name} [contexts={leaf_contexts}]"
    elif node.children:
        title = f"{indent}{display_name}/ [leaf-contexts={leaf_contexts}]"
    else:
        title = f"{indent}{display_name} [contexts={leaf_contexts}]"
    lines.append(title)

    for chart in sorted(node.charts, key=lambda chart: (chart.priority, chart.context)):
        lines.append(f"{indent}  . {chart.context} @{chart.priority}")

    for _, child in sorted(node.children.items(), key=sort_key):
        lines.extend(render(child, depth + 1))
    return lines


def warnings(root: Node, app_name: str | None = None) -> list[str]:
    results: list[str] = []
    top_names = [name.strip("/") for name in root.children]
    application = (app_name or "").strip()

    # One-character family segments, including the accidental split of names such as I/O.
    def walk(node: Node, path: tuple[str, ...] = ()) -> None:
        current = (*path, node.name) if depth_visible(node, path) else path
        for segment in (node.name.split("/") if node.name else []):
            if len(segment) == 1:
                rendered = "/".join(part for part in current if part and part != "(root)")
                results.append(
                    f"one-character family segment {segment!r} in {rendered or '(root)'}: "
                    "a slash-containing label such as I/O may have split into I/O path segments"
                )
        for child in node.children.values():
            walk(child, current)

    def depth_visible(node: Node, path: tuple[str, ...]) -> bool:
        return bool(path) or bool(node.name)

    walk(root)

    # Common prefix on all top-level families.
    if len(top_names) >= 2:
        first, *rest = top_names
        common: list[str] = []
        for characters in zip(*(name for name in top_names if name)):
            if len(set(characters)) == 1:
                common.append(characters[0])
            else:
                break
        prefix = "".join(common).strip()
        words = first.split()
        usable_prefix = ""
        for index in range(1, len(words) + 1):
            candidate = " ".join(words[:index]).strip()
            if candidate and all(name == candidate or name.startswith(candidate + " ") for name in top_names):
                usable_prefix = candidate
        selected = max((prefix, usable_prefix), key=len)
        if selected and len(selected) >= 2:
            results.append(
                f"all top-level families share prefix {selected!r}: remove the common/application prefix if it is redundant"
            )

    if application:
        lowered = application.casefold()
        exact_names = {name.casefold() for name in top_names}
        for name in top_names:
            # CephFS-style product names legitimately begin with the same letters as the
            # application; only flag an exact application token, not a longer product word.
            exact_application_prefix = name.casefold() == lowered or name.casefold().startswith(lowered + " ")
            if exact_application_prefix and name.casefold() not in exact_names:
                results.append(
                    f"top-level family {name!r} starts with application name {application!r}: remove the redundant prefix"
                )

    # Oversized and single-context leaves.
    def leaves(node: Node, path: tuple[str, ...] = ()) -> None:
        current = path + ((node.name,) if node.name else ())
        if not node.children and current:
            rendered = "/".join(part for part in current if part and part != "(root)")
            if node.context_count > LARGE_LEAF_CONTEXTS:
                results.append(
                    f"leaf {rendered!r} contains {node.context_count} contexts (> {LARGE_LEAF_CONTEXTS}): add structure"
                )
            elif node.context_count == 1:
                results.append(f"leaf {rendered!r} contains one context: remove unnecessary structure or merge siblings")
        for child in node.children.values():
            leaves(child, current)

    leaves(root)

    return sorted(dict.fromkeys(results))


def parse_arguments(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile", type=Path, help="profile YAML path")
    parser.add_argument("--app", help="application name used to detect redundant application prefixes")
    parser.add_argument("--quiet", action="store_true", help="print ToC without warnings")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_arguments(argv)
    try:
        document = yaml.safe_load(args.profile.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as error:
        print(f"error: cannot read profile {args.profile}: {error}", file=sys.stderr)
        return 2
    if not isinstance(document, dict):
        print(f"error: profile {args.profile} is not a YAML mapping", file=sys.stderr)
        return 2

    try:
        tree = build_tree(document)
    except ValueError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2

    print("\n".join(render(tree)))
    if not args.quiet:
        design_warnings = warnings(tree, args.app or normalize(document.get("app")))
        print("\nUX warnings (not validation failures):")
        if design_warnings:
            for index, warning in enumerate(design_warnings, 1):
                print(f"{index}. {warning}")
        else:
            print("none")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
