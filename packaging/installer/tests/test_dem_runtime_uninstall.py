# SPDX-License-Identifier: GPL-3.0-or-later
"""Run the uninstaller's actual teardown sequence with inert file-removal callouts."""
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


SOURCE = Path(__file__).resolve().parents[1] / 'netdata-uninstaller.sh'


def option_parser(source):
    return source[:source.index('if [ -n "${script_source}" ]; then')]


class RuntimeUninstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="dem remove 'literal ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.prefix = self.root / 'netdata'
        libexec = self.prefix / 'usr/libexec/netdata'
        (libexec / 'plugins.d').mkdir(parents=True)
        self.helper = libexec / 'netdata-dem-runtime.sh'
        self.helper.write_text('''#!/bin/sh
[ "$1" = agent-remove ] && [ "$2" = --agent ] &&
[ "$3" = "$EXPECTED_AGENT" ] || exit 99
if [ "$#" = 4 ] && [ "$4" = --deployments-stopped ]; then
  printf 'explicit-retirement\\n' >> "$EVENTS"
elif [ "$#" != 3 ]; then exit 99; fi
if [ "$REQUIRE_RETIREMENT" = 1 ] && [ "$#" != 4 ]; then exit 41; fi
printf 'runtime-drain\\n' >> "$EVENTS"
exit "$DRAIN_STATUS"
''')
        self.helper.chmod(0o755)
        self.plugin = libexec / 'plugins.d/dem.plugin'
        self.plugin.write_text('fixture')
        self.events = self.root / 'events'
        source = SOURCE.read_text()
        functions = re.findall(r'^remove_dem_runtime(?:_or_exit)?\(\) \{\n.*?^\}\n', source, re.M | re.S)
        sequence_start = source.index('#### STOP NETDATA')
        sequence_end = source.index('if [ -n "${tmpdir}" ]; then', sequence_start)
        sequence = source[sequence_start:sequence_end]
        self.script = self.root / 'teardown.sh'
        self.script.write_text('''
info() { :; }
error() { printf '%s\\n' "$*" >&2; }
run() { "$@"; }
stop_all_netdata() { printf 'stop\\n' >> "$EVENTS"; }
uname() { printf 'Linux\\n'; }
issystemd() { return 1; }
rm_file() { printf 'erase-file\\n' >> "$EVENTS"; }
rm_dir() { printf 'erase-dir\\n' >> "$EVENTS"; }
''' + '\n'.join(functions) + sequence)

    def invoke(self, status=0, prefix=None, **env):
        return subprocess.run(['sh', str(self.script)], env=dict(
            os.environ, NETDATA_PREFIX=str(self.prefix) if prefix is None else prefix,
            EXPECTED_AGENT=str(self.prefix / "usr/sbin/netdata"),
            EVENTS=str(self.events), DRAIN_STATUS=str(status), **env),
            capture_output=True, text=True)

    def recorded(self):
        return self.events.read_text().splitlines() if self.events.exists() else []

    def test_runtime_drains_before_any_file_is_erased(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded()[:2], ['stop', 'runtime-drain'])
        self.assertIn('erase-dir', self.recorded()[2:])

    def test_trailing_prefix_slashes_use_the_canonical_agent_path(self):
        for suffix in ('/', '///'):
            with self.subTest(suffix=suffix):
                result = self.invoke(prefix=str(self.prefix) + suffix)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_refused_or_failed_drain_preserves_all_files_and_exit_status(self):
        result = self.invoke(37)
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(self.recorded(), ['stop', 'runtime-drain'])

    def test_missing_helper_does_not_bypass_installed_dem(self):
        self.helper.unlink()
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.recorded(), ['stop'])

    def test_installation_without_dem_keeps_existing_removal(self):
        self.helper.unlink()
        self.plugin.unlink()
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('erase-dir', self.recorded())

    def test_preload_retirement_is_explicitly_forwarded(self):
        self.assertEqual(self.invoke(REQUIRE_RETIREMENT='1').returncode, 41)
        result = self.invoke(REQUIRE_RETIREMENT='1', DEM_DEPLOYMENTS_STOPPED='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('explicit-retirement', self.recorded())
        self.assertIn('erase-dir', self.recorded())

    def test_force_does_not_imply_retirement(self):
        result = self.invoke(REQUIRE_RETIREMENT='1', YES='1', INTERACTIVITY='-f')
        self.assertEqual(result.returncode, 41, result.stderr)
        self.assertNotIn('erase-dir', self.recorded())

    def test_missing_parser_boundary_fails_before_shell_execution(self):
        source = SOURCE.read_text().replace('if [ -n "${script_source}" ]; then', '# changed boundary')
        with self.assertRaises(ValueError):
            option_parser(source)

    def test_retirement_option_requires_its_own_command_line_flag(self):
        parser = option_parser(SOURCE.read_text())
        script = self.root / 'options.sh'
        script.write_text(parser + '\nprintf "%s" "$DEM_DEPLOYMENTS_STOPPED"\n')
        for flags, expected in ((['--yes', '--force'], '0'),
                                (['--yes', '--dem-deployments-stopped'], '1')):
            result = subprocess.run(['sh', str(script), *flags], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, expected)

    def native(self, manager='apt-get', drain_status=0, package_status=0, consent='0'):
        source = SOURCE.read_text()
        functions = re.findall(r'^(?:remove_dem_runtime(?:_or_exit)?|prepare_native_dem_removal)\(\) \{\n.*?^\}\n',
                               source, re.M | re.S)
        native_function = re.findall(r'^remove_native_install\(\) \{\n.*?^\}\n', source, re.M | re.S)
        if native_function:
            dispatch = native_function[0] + '\nremove_native_install\n'
        else:
            # Execute the original early dispatch too, so the regression fails
            # against the previous implementation rather than skipping that path.
            start = source.index('if [ -x "$(command -v apt-get)" ]')
            dispatch = source[start:source.index(
                '\n# -----------------------------------------------------------------------------', start)]
        pm = self.root / manager
        pm.write_text('''#!/bin/sh
if [ "$1" = search ]; then [ "$3" = netdata ]; exit $?; fi
printf 'package-remove\\n' >> "$EVENTS"
exit "$PACKAGE_STATUS"
''')
        pm.chmod(0o755)
        body = '''
error() { printf '%s\\n' "$*" >&2; }
run() { "$@"; }
service() { printf 'service-stop\\n' >> "$EVENTS"; }
user_input() { return "$CONSENT"; }
command() { if [ "$1:$2" = "-v:$PACKAGE_MANAGER" ]; then printf '%s\\n' "$PM_STUB"; fi; }
dpkg() { [ "$2" = netdata ]; }
rpm() { [ "$2" = netdata ]; }
cleanup_data_and_config() { printf 'erase-dir\\n' >> "$EVENTS"; }
''' + '\n'.join(functions) + dispatch
        native_script = self.root / 'native.sh'
        native_script.write_text(body)
        return subprocess.run(['sh', str(native_script)], env=dict(
            os.environ, PATH=str(self.root) + os.pathsep + os.environ['PATH'],
            NETDATA_PREFIX=str(self.prefix), ndprefix=str(self.prefix), EVENTS=str(self.events),
            EXPECTED_AGENT=str(self.prefix / "usr/sbin/netdata"),
            INSTALL_TYPE='binpkg-deb' if manager == 'apt-get' else 'binpkg-rpm',
            PACKAGE_MANAGER=manager, PM_STUB=str(pm), PACKAGE_STATUS=str(package_status),
            DRAIN_STATUS=str(drain_status), CONSENT=consent), capture_output=True, text=True)

    def test_native_removal_checks_runtime_before_packages_or_state(self):
        for manager in ('apt-get', 'dnf', 'yum', 'zypper'):
            with self.subTest(manager=manager):
                self.events.write_text('')
                result = self.native(manager)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.recorded(), ['service-stop', 'runtime-drain', 'package-remove', 'erase-dir'])

    def test_native_failed_drain_preserves_packages_and_state(self):
        result = self.native(drain_status=37)
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(self.recorded(), ['service-stop', 'runtime-drain'])

    def test_native_declined_or_failed_package_removal_preserves_state(self):
        result = self.native(consent='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded(), [])
        result = self.native(package_status=39)
        self.assertEqual(result.returncode, 39, result.stderr)
        self.assertNotIn('erase-dir', self.recorded())


if __name__ == '__main__':
    unittest.main()
