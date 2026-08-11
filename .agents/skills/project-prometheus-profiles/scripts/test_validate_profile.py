#!/usr/bin/env python3
"""Tests for the Prometheus profile validator compatibility launcher."""

from __future__ import annotations

import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import _go_tool_launcher as go_tool_launcher


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
            "--support-profile",
            "profiles/runtime.yaml",
            "-support-profile=profiles/gc.yaml",
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
                "--support-profile",
                str(caller / "profiles/runtime.yaml"),
                f"-support-profile={caller / 'profiles/gc.yaml'}",
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

    def test_main_resolves_go_from_relative_path_before_chdir(self) -> None:
        launcher_path = Path(__file__).with_name("validate-profile.py").resolve()
        go_root = go_tool_launcher.repo_root(str(launcher_path)) / "src" / "go"

        with tempfile.TemporaryDirectory() as temporary:
            caller = Path(temporary).resolve()
            bin_dir = caller / "bin"
            bin_dir.mkdir()
            fake_go = bin_dir / "go"
            fake_go.write_text(
                "#!/bin/sh\n"
                "printf 'cwd=%s\\n' \"$PWD\"\n"
                "printf 'arg=%s\\n' \"$@\"\n",
                encoding="utf-8",
            )
            fake_go.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = "bin"
            result = subprocess.run(
                [
                    sys.executable,
                    str(launcher_path),
                    "--profile",
                    "profile.yaml",
                    "--support-profile",
                    "support.yaml",
                    "--dump=dump.prom",
                    "-job",
                    "job.yaml",
                ],
                cwd=caller,
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"cwd={go_root}\n", result.stdout)
        for argument in [
            "run",
            "./tools/prometheus-profile-validation",
            "--profile",
            str(caller / "profile.yaml"),
            "--support-profile",
            str(caller / "support.yaml"),
            f"--dump={caller / 'dump.prom'}",
            "-job",
            str(caller / "job.yaml"),
        ]:
            self.assertIn(f"arg={argument}\n", result.stdout)


if __name__ == "__main__":
    unittest.main()
