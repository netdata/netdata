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


if __name__ == "__main__":
    unittest.main()
