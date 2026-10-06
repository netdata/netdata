#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the real installer parser and CMake forwarding without installing."""

import os
import sys

import build
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[2]


@unittest.skipUnless(shutil.which("cmake") and shutil.which("make"), "requires CMake and Make")
class InstallerTest(unittest.TestCase):
    def run_installer(self, *flags, go_available=True):
        with tempfile.TemporaryDirectory(prefix="otelcolpoc-installer-") as tmp:
            root = Path(tmp)
            packaging = root / "packaging"
            (packaging / "installer").mkdir(parents=True)
            shutil.copy2(REPO / "netdata-installer.sh", root)
            shutil.copy2(REPO / "packaging/installer/functions.sh", packaging / "installer")
            # Isolate toolchain provisioning: never download or install system tools.
            (packaging / "check-for-go-toolchain.sh").write_text(
                'GOLANG_MIN_VERSION=1.27.0\n'
                'GOLANG_FAILURE_REASON="synthetic missing toolchain"\n'
                'ensure_go_toolchain() {\n'
                '  echo checked > go-probe\n'
                f'  return {0 if go_available else 1}\n'
                '}\n'
            )
            env = os.environ.copy()
            env.update(NETDATA_BUILD_DIR=str(root / "build"), DISABLE_TELEMETRY="1")
            env.pop("NETDATA_CMAKE_OPTIONS", None)
            result = subprocess.run(
                ["sh", "./netdata-installer.sh", "--prepare-only", "--dont-start-it",
                 "--dont-wait", "--dev", "--disable-plugin-go", "--disable-plugin-scripts",
                 "--disable-plugin-otel", "--disable-ebpf", "--disable-ml",
                 "--install-prefix", str(root / "install"), *flags],
                cwd=root, env=env, text=True, stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT, timeout=60,
            )
            return result.returncode, result.stdout, (root / "go-probe").exists()

    def test_cmake_out_of_source_manifest(self):
        with tempfile.TemporaryDirectory(prefix="otelcolpoc cmake ") as tmp:
            root = Path(tmp)
            source = REPO / "tests/otel-orchestrator-poc"
            (root / "CMakeLists.txt").write_text(
                'cmake_minimum_required(VERSION 3.18)\nproject(otelcolpoc_fixture NONE)\n'
                'set(GO_EXECUTABLE "/synthetic/go path/go")\n'
                f'add_subdirectory("{source}" collector-build)\n'
            )
            result = subprocess.run(["cmake", "-S", str(root), "-B", str(root / "build")],
                                    text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            manifest = (root / "build/collector-build/builder-config.yaml").read_text()
            self.assertIn("  output_path: collector", manifest)
            self.assertIn("  go: '/synthetic/go path/go'", manifest)
            self.assertNotIn("    path: .", manifest)
            self.assertIn(f' => "{source}"', manifest)
            self.assertEqual(manifest.count("replaces:"), 1)
            install = (root / "build/collector-build/cmake_install.cmake").read_text()
            self.assertIn("collector/otel-worker", install)
            self.assertNotIn("otel-worker.plugin", install)

    def test_help_documents_opt_in(self):
        result = subprocess.run(["sh", str(REPO / "netdata-installer.sh"), "--help"],
                                cwd=REPO, text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 1, result.stderr)  # Existing installer help convention.
        self.assertIn("--enable-plugin-otelcolpoc", result.stdout)
        self.assertIn("--disable-plugin-otelcolpoc", result.stdout)

    def test_default_off_without_go_probe(self):
        code, output, probed = self.run_installer()
        self.assertEqual(code, 0, output)
        self.assertIn("-DENABLE_PLUGIN_OTELCOLPOC=Off", output)
        self.assertFalse(probed)

    def test_enable_independently_of_go_plugin_and_sink(self):
        code, output, probed = self.run_installer("--enable-plugin-otelcolpoc")
        self.assertEqual(code, 0, output)
        self.assertIn("-DENABLE_PLUGIN_OTELCOLPOC=On", output)
        self.assertIn("-DENABLE_PLUGIN_GO=Off", output)
        self.assertIn("-DENABLE_PLUGIN_OTEL=Off", output)
        self.assertTrue(probed)

    def test_disable_overrides_enable(self):
        code, output, probed = self.run_installer(
            "--enable-plugin-otelcolpoc", "--disable-plugin-otelcolpoc"
        )
        self.assertEqual(code, 0, output)
        self.assertIn("-DENABLE_PLUGIN_OTELCOLPOC=Off", output)
        self.assertFalse(probed)

    def test_explicit_enable_fails_without_go(self):
        code, output, probed = self.run_installer("--enable-plugin-otelcolpoc", go_available=False)
        self.assertNotEqual(code, 0, output)
        self.assertIn("OTel Collector POC was explicitly enabled", output)
        self.assertIn("synthetic missing toolchain", output)
        self.assertNotIn("Would have used the following CMake command", output)
        self.assertTrue(probed)


class BuildCommandTest(unittest.TestCase):
    def test_argument_boundaries_and_success(self):
        argument = "spaces ; $HOME `false`"
        build.run([sys.executable, "-c", "import sys; assert sys.argv[1] == " + repr(argument), argument])

    def test_original_failure_status(self):
        with self.assertRaises(SystemExit) as raised:
            build.run([sys.executable, "-c", "raise SystemExit(37)"])
        self.assertEqual(raised.exception.code, 37)

    def test_display_redacts_secrets(self):
        command = ["tool", "--token", "synthetic-value", "PASSWORD=synthetic-password",
                   "https://user:synthetic-password@example.invalid/path?token=synthetic-value"]
        rendered = build.display(command)
        self.assertNotIn("synthetic-value", rendered)
        self.assertNotIn("synthetic-password", rendered)
        self.assertEqual(command[2], "synthetic-value")


if __name__ == "__main__":
    unittest.main()
