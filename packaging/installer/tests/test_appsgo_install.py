#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Run the real appsgo installer sections without installing host services."""

from pathlib import Path
import os
import shlex
import shutil
import subprocess
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[3]
INSTALLER = (REPO / "netdata-installer.sh").read_text()
FUNCTIONS = (REPO / "packaging/installer/functions.sh").read_text()


def function(name):
    start = FUNCTIONS.index(f"{name}() {{\n")
    return FUNCTIONS[start:FUNCTIONS.index("\n}\n", start) + 3]


class AppsgoInstallTest(unittest.TestCase):
    def run_shell(self, script, args=(), env=None):
        result = subprocess.run(["sh", "-c", script, "installer-test", *args],
                                env={**os.environ, **(env or {})}, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout, result.stderr

    def test_flags_reach_cmake_independently_of_go_d(self):
        # Execute production option defaults/parser and CMake translation. Other
        # dependency probes are irrelevant here and cannot install anything.
        start = INSTALLER.index("DONOTSTART=0\n")
        end = INSTALLER.index('\nif [ ! "${DISABLE_TELEMETRY', start)
        script = INSTALLER[start:end] + "\n"
        script += "\n".join(function(name) for name in ("enable_feature", "check_for_feature", "prepare_cmake_options"))
        script += '''
check_for_module() { return 1; }
FORCE_LEGACY_CXX=1
NETDATA_BUILD_DIR=test-build
prepare_cmake_options
printf '%s\\n' "${NETDATA_CMAKE_OPTIONS}"
'''
        cases = [
            ([], "Off", "On"),
            (["--enable-plugin-appsgo", "--disable-plugin-go"], "On", "Off"),
            (["--enable-plugin-appsgo", "--disable-plugin-appsgo"], "Off", "On"),
            (["--disable-plugin-appsgo", "--enable-plugin-appsgo"], "On", "On"),
        ]
        for args, appsgo, go_d in cases:
            with self.subTest(args=args):
                output, _ = self.run_shell(script, args)
                options = shlex.split(output)
                self.assertIn(f"-DENABLE_PLUGIN_APPSGO={appsgo}", options)
                self.assertIn(f"-DENABLE_PLUGIN_GO={go_d}", options)
                self.assertEqual(sum(x.startswith("-DENABLE_PLUGIN_APPSGO=") for x in options), 1)

    def test_capability_setup_and_unprivileged_fallback(self):
        start = INSTALLER.index('  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/appsgo.plugin" ]; then')
        end = INSTALLER.index('  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin" ]; then', start)
        block = INSTALLER[start:end]
        script = '''
NETDATA_GROUP=netdata
iscontainer() { return "${TEST_CONTAINER}"; }
setcap() { :; }
run() {
    printf '%s\\n' "$*"
    if [ "$1" = setcap ]; then return "${TEST_SETCAP_STATUS}"; fi
    return 0
}
warning() { printf '%s\\n' "$1" >&2; }
''' + block
        cases = [
            ("capabilities", 1, 0, "/bin/true", True, False, False),
            ("container", 0, 0, "/bin/true", False, False, True),
            ("setcap fails", 1, 1, "/bin/true", True, False, True),
            ("exec denied", 1, 0, "/bin/false", True, True, True),
        ]
        for label, container, status, executable, grant, remove, warn in cases:
            with self.subTest(case=label), tempfile.TemporaryDirectory() as tmp:
                binary = Path(tmp) / "usr/libexec/netdata/plugins.d/appsgo.plugin"
                binary.parent.mkdir(parents=True)
                binary.symlink_to(shutil.which(Path(executable).name))
                output, errors = self.run_shell(script, env={
                    "NETDATA_PREFIX": tmp, "TEST_CONTAINER": str(container),
                    "TEST_SETCAP_STATUS": str(status),
                })
                self.assertIn(f"chown root:netdata {binary}", output)
                self.assertIn(f"chmod 0750 {binary}", output)
                self.assertEqual("setcap cap_dac_read_search,cap_sys_ptrace+ep" in output, grant)
                self.assertEqual("setcap -r" in output, remove)
                self.assertEqual("without elevated capabilities" in errors, warn)
                self.assertNotIn("4750", output)


if __name__ == "__main__":
    unittest.main()
