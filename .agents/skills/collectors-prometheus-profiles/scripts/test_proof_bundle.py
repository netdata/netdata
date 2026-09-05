#!/usr/bin/env python3
"""Tests for the Prometheus profile proof launcher."""

from __future__ import annotations

import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


def load_launcher():
    path = Path(__file__).with_name("proof-bundle.py")
    spec = importlib.util.spec_from_file_location("proof_bundle_launcher", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load launcher: {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


launcher = load_launcher()


class ProofBundleLauncherTest(unittest.TestCase):
    def test_repo_root_walks_from_nested_launcher_path(self) -> None:
        root = launcher.repo_root(launcher.__file__)
        nested_launcher = Path(launcher.__file__).parent / "nested" / "launcher.py"
        self.assertEqual(launcher.repo_root(str(nested_launcher)), root)

    def test_normalizes_testdata_root_and_injects_repo_root(self) -> None:
        caller = Path.cwd() / "caller"
        self.assertEqual(
            launcher.normalize_arguments(
                ["verify", "--testdata-root", "testdata", "--testdata-root=/absolute/testdata"],
                caller,
            ),
            [
                "verify",
                "--repo-root",
                str(launcher.repo_root(launcher.__file__)),
                "--testdata-root",
                str(caller / "testdata"),
                "--testdata-root=/absolute/testdata",
            ],
        )

    def test_main_runs_proof_tool_from_go_root(self) -> None:
        launcher_path = Path(__file__).with_name("proof-bundle.py").resolve()
        root = launcher.repo_root(str(launcher_path))
        go_root = root / "src" / "go"

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
                [sys.executable, str(launcher_path), "verify", "--testdata-root", "evidence"],
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
            "./tools/prometheus-profile-proof",
            "verify",
            "--repo-root",
            str(root),
            "--testdata-root",
            str(caller / "evidence"),
        ]:
            self.assertIn(f"arg={argument}\n", result.stdout)


if __name__ == "__main__":
    unittest.main()
