#!/usr/bin/env python3
"""Tests for the profile table-of-contents design helper."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest

import yaml

SCRIPT = Path(__file__).with_name("profile-toc.py")


def load_module():
    spec = importlib.util.spec_from_file_location("profile_toc", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


profile_toc = load_module()


def tree_from_yaml(text: str):
    with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as stream:
        stream.write(text)
        path = Path(stream.name)
    try:
        return profile_toc.build_tree(yaml.safe_load(path.read_text()))
    finally:
        path.unlink()


class ProfileTocWarningsTest(unittest.TestCase):
    def test_common_prefix_warning(self) -> None:
        root = tree_from_yaml(
            """
app: service
template:
  family: Service
  groups:
  - family: Service API
    charts: [{title: a, context: a, units: x, dimensions: []}]
  - family: Service Workers
    charts: [{title: b, context: b, units: x, dimensions: []}]
"""
        )
        warnings = profile_toc.warnings(root, "service")
        self.assertIn("all top-level families share prefix 'Service'", "\n".join(warnings))

    def test_one_character_segment_warning(self) -> None:
        root = profile_toc.Node("I/O")
        child = profile_toc.Node("O")
        root.children["O"] = child
        child.charts.append(profile_toc.Chart("ctx", 1))
        self.assertTrue(any("one-character family segment" in warning for warning in profile_toc.warnings(root)))

    def test_large_leaf_warning(self) -> None:
        root = profile_toc.Node("(root)")
        node = profile_toc.Node("Large")
        root.children["Large"] = node
        for index in range(16):
            node.charts.append(profile_toc.Chart(f"context{index}", index))
        self.assertTrue(any("contains 16 contexts" in warning for warning in profile_toc.warnings(root)))

    def test_single_context_leaf_warning(self) -> None:
        root = profile_toc.Node("(root)")
        node = profile_toc.Node("Single")
        root.children["Single"] = node
        node.charts.append(profile_toc.Chart("context", 1))
        self.assertTrue(any("contains one context" in warning for warning in profile_toc.warnings(root)))


class ProfileTocOrderingTest(unittest.TestCase):
    def test_parent_sort_uses_minimum_descendant_priority(self) -> None:
        root = profile_toc.Node("App")
        overview = profile_toc.Node("Overview")
        overview_child = profile_toc.Node("Health")
        detail = profile_toc.Node("Details")
        overview_child.charts.append(profile_toc.Chart("health", 100))
        overview_child.charts.append(profile_toc.Chart("status", 300))
        overview.children["Health"] = overview_child
        detail.charts.append(profile_toc.Chart("detail", 200))
        root.children["Overview"] = overview
        root.children["Details"] = detail

        ordered = [name for name, _ in sorted(root.children.items(), key=profile_toc.sort_key)]

        self.assertEqual(["Overview", "Details"], ordered)

    def test_profile_priority_uses_runtime_default_for_non_positive_values(self) -> None:
        root = tree_from_yaml(
            """
template:
  family: App
  groups:
  - family: Overview
    charts:
    - title: a
      context: a
      units: x
      priority: -1
      dimensions: []
"""
        )

        self.assertEqual("App", root.name)
        self.assertEqual(["Overview"], list(root.children))
        overview = root.children["Overview"]
        self.assertEqual(70000, profile_toc.node_priority(overview))
        self.assertEqual(70000, overview.charts[0].priority)


if __name__ == "__main__":
    unittest.main()
