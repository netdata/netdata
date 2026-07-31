#!/usr/bin/env python3
"""Tests for the Prometheus profile validator compatibility launcher."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import unittest


def load_launcher():
    path = Path(__file__).with_name("validate-profile.py")
    spec = importlib.util.spec_from_file_location("validate_profile_launcher", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load launcher: {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


launcher = load_launcher()


class NormalizeFileArgumentsTest(unittest.TestCase):
    def test_normalizes_separated_and_equals_forms(self) -> None:
        caller = Path.cwd() / "caller"
        absolute_dump = caller / "absolute.prom"
        arguments = [
            "--profile",
            "profiles/app.yaml",
            f"--dump={absolute_dump}",
            "-job=jobs/app.yaml",
            "--output",
            "json",
        ]

        self.assertEqual(
            launcher.normalize_file_arguments(arguments, caller),
            [
                "--profile",
                str(caller / "profiles/app.yaml"),
                f"--dump={absolute_dump}",
                f"-job={caller / 'jobs/app.yaml'}",
                "--output",
                "json",
            ],
        )

    def test_preserves_empty_and_missing_values_for_go_flag_validation(self) -> None:
        caller = Path.cwd() / "caller"

        self.assertEqual(
            launcher.normalize_file_arguments(["--profile=", "--dump"], caller),
            ["--profile=", "--dump"],
        )


if __name__ == "__main__":
    unittest.main()
