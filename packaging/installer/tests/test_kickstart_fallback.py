#!/usr/bin/env python3
"""Exercise kickstart selection with no network access or system installation."""
from pathlib import Path
import re
import subprocess
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'kickstart.sh'
SOURCE = SCRIPT.read_text()


def function(name):
    """Extract a top-level shell function without executing kickstart's main."""
    return re.search(r'^' + name + r'\(\) \{\n.*?^\}', SOURCE, re.M | re.S)[0]


class KickstartFallbackTests(unittest.TestCase):
    def run_shell(self, code):
        return subprocess.run(['sh', '-c', code], text=True, capture_output=True)

    def dispatch(self, native=3, static=2, args=(), system='linux', build=0):
        defaults = SOURCE.split('# Defaults for environment variables\n', 1)[1].split(
            'if [ -n "$DISABLE_TELEMETRY" ]', 1)[0]
        code = defaults + '\n' + '\n'.join(function(name) for name in (
            'parse_args', 'install_on_linux', 'install_on_macos', 'install_on_freebsd'))
        code += '''
warning() { printf 'WARNING: %s\\n' "$*"; }
fatal() { printf 'FATAL: %s\\n' "$*"; exit 1; }
validate_args() { :; }
SELECTED_RELEASE_CHANNEL=nightly
'''
        code += f'''
try_package_install() {{ echo NATIVE; return {native}; }}
try_static_install() {{ echo STATIC; return {static}; }}
try_build_install() {{ echo BUILD; return {build}; }}
parse_args {' '.join(args)}
install_on_{system}
'''
        result = self.run_shell(code)
        calls = [line for line in result.stdout.splitlines() if line in ('NATIVE', 'STATIC', 'BUILD')]
        return result, calls

    def test_linux_unavailable_binaries_allow_source(self):
        for install_type in ('auto', 'any'):
            with self.subTest(install_type=install_type):
                result, calls = self.dispatch(args=('--install-type', install_type))
                self.assertEqual(result.returncode, 0, result.stdout)
                self.assertEqual(calls, ['NATIVE', 'STATIC', 'BUILD'])
                self.assertIn('Support for local builds is best-effort', result.stdout)

    def test_available_binaries_block_source_unless_overridden(self):
        for native, static in ((3, 3), (2, 2), (2, 3)):
            for allow in (False, True):
                with self.subTest(native=native, static=static, allow=allow):
                    args = ('--install-type', 'any') + (('--allow-local-build',) if allow else ())
                    result, calls = self.dispatch(native, static, args)
                    self.assertEqual(result.returncode, 0 if allow else 1, result.stdout)
                    self.assertEqual(calls, ['NATIVE', 'STATIC'] + (['BUILD'] if allow else []))
                    self.assertIn('best-effort', result.stdout)
                    if not allow:
                        self.assertIn('--allow-local-build', result.stdout)

    def test_default_static_failure_is_blocked(self):
        result, calls = self.dispatch(static=3)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(calls, ['NATIVE', 'STATIC'])

    def test_unpublished_native_packages_are_unavailable(self):
        result, calls = self.dispatch(native=4, args=('--install-type', 'any'))
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(calls, ['NATIVE', 'STATIC', 'BUILD'])

    def test_success_stops_fallback(self):
        for native, static, expected in ((0, 2, ['NATIVE']), (3, 0, ['NATIVE', 'STATIC'])):
            result, calls = self.dispatch(native, static)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertEqual(calls, expected)
            self.assertNotIn('best-effort', result.stdout)

    def test_explicit_build(self):
        for args in (('--build-only',), ('--install-type', 'build')):
            result, calls = self.dispatch(args=args)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertEqual(calls, ['BUILD'])
            self.assertIn('best-effort', result.stdout)

    def test_strict_binary_types_do_not_build_even_with_override(self):
        for args, expected in ((('--native-only',), ['NATIVE']), (('--static-only',), ['STATIC'])):
            result, calls = self.dispatch(args=args + ('--allow-local-build',))
            self.assertEqual(result.returncode, 1)
            self.assertEqual(calls, expected)

    def test_native_auto_errors_remain_fatal(self):
        for status in (1, 2, 4):
            result, calls = self.dispatch(native=status, args=('--allow-local-build',))
            self.assertEqual(result.returncode, 1)
            self.assertEqual(calls, ['NATIVE'])

    def test_hard_static_and_source_failures_remain_fatal(self):
        for static, build, expected in ((1, 0, ['NATIVE', 'STATIC']), (2, 1, ['NATIVE', 'STATIC', 'BUILD'])):
            result, calls = self.dispatch(static=static, build=build, args=('--allow-local-build',))
            self.assertEqual(result.returncode, 1)
            self.assertEqual(calls, expected)

    def test_non_linux_selection_is_unchanged(self):
        for system in ('macos', 'freebsd'):
            for args in ((), ('--allow-local-build',), ('--build-only',), ('--install-type', 'any')):
                with self.subTest(system=system, args=args):
                    result, calls = self.dispatch(args=args, system=system)
                    self.assertEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(calls, ['BUILD'])
                    self.assertNotIn('best-effort', result.stdout)
            for option in ('--native-only', '--static-only'):
                result, calls = self.dispatch(args=(option, '--allow-local-build'), system=system)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(calls, [])

    def test_real_static_helper_distinguishes_missing_and_failed_archives(self):
        # Use the real helper, with all host/network operations replaced by stubs.
        code = function('try_static_install') + '''
set_static_archive_urls() { NETDATA_MANIFEST_INSTALL=0; }
progress() { :; }
warning() { :; }
fatal() { exit 99; }
download() { :; }
verify_manifest() { :; }
run_as_root() { return 1; }
run() { :; }
DRY_RUN=1
INTERACTIVE=0
check_for_remote_file() { return 1; }
try_static_install
printf 'missing=%s\\n' "$?"
check_for_remote_file() { return 0; }
try_static_install
printf 'failed=%s\\n' "$?"
'''
        result = self.run_shell(code)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, 'missing=2\nfailed=3\n')


if __name__ == '__main__':
    unittest.main()
