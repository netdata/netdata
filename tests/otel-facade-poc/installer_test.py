#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the real installer parser and CMake forwarding without installing."""

import os
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


if __name__ == "__main__":
    unittest.main()
