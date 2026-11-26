# SPDX-License-Identifier: GPL-3.0-or-later
"""Static caller contracts with downloads and privilege changes replaced by fixtures."""
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1]


def shell_function(source, name):
    return re.search(r'^' + re.escape(name) + r'\(\) \{\n.*?^}', source, re.M | re.S).group()


class StaticCallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-static-callers-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.prefix = self.root / 'prefix'
        self.scratch = self.root / 'selected scratch'
        self.scratch.mkdir()
        self.record = self.root / 'child-record'
        self.kickstart = (INSTALLER / 'kickstart.sh').read_text()
        self.updater = (INSTALLER / 'netdata-updater.sh').read_text()

    def run_shell(self, script):
        environment = os.environ.copy()
        environment.pop('TMPDIR', None)
        return subprocess.run(['sh', '-c', script], cwd=self.scratch, env=environment,
                              text=True, capture_output=True)

    def test_kickstart_summarizes_failed_curl_probe(self):
        curl = self.root / 'curl'
        curl.write_text('#!/bin/sh\nexit 60\n')
        curl.chmod(0o755)
        prefix = 'https://example.invalid/static%20build'
        env = {'PATH': f'{self.root}{os.pathsep}{os.environ["PATH"]}', 'TMPDIR': str(self.root),
               'ROOTCMD': 'env', 'NETDATA_ARTIFACT_URL_PREFIX': prefix}
        result = subprocess.run(
            ['sh', str(INSTALLER / 'kickstart.sh'), '--disable-telemetry', '--no-cleanup', '--static-only',
             '--prepare-offline-install-source', 'bundle'],
            cwd=self.root, env=env, text=True, capture_output=True)

        self.assertEqual(result.returncode, 1, result.stderr)
        header = 'The following non-fatal warnings or errors were encountered:\n'
        self.assertIn(header, result.stderr)
        summary = result.stderr.split(header, 1)[1]
        url = f'{prefix}/Manifest'
        self.assertIn(f'\n  - Command "{curl} --write-out %{{http_code}} ', summary)
        self.assertIn(f'{url}" failed with exit code 60.\n'
                      f'  - TLS error while connecting to remote host when checking for remote file at {url}\n',
                      summary)

    def archive(self, status=37, create_prefix=False):
        archive = self.root / 'fixture.run'
        body = '#!/bin/sh\n'
        if create_prefix:
            body += 'mkdir -p ' + shlex.quote(str(self.prefix / 'etc/netdata')) + '\n'
            body += 'printf partial > ' + shlex.quote(str(self.prefix / 'etc/netdata/netdata.conf')) + '\n'
        body += ('printf "%s\\n" "${TMPDIR-unset}" "${NETDATA_CERT_TEST_URL-unset}" '
                 '"${NETDATA_CERT_MODE-unset}" "$@" > ' + shlex.quote(str(self.record)) + '\n')
        body += 'printf "%s\\n" "matching companion unavailable; live files unchanged" >&2\n'
        body += f'exit {status}\n'
        archive.write_text(body)
        return archive

    def kickstart_script(self, archive, offline=False, claim_url='https://example.invalid/claim'):
        function = shell_function(self.kickstart, 'try_static_install').replace('/opt/netdata', str(self.prefix))
        return function + '\n' + '''
set_static_archive_urls() { :; }; check_for_remote_file() { :; }; verify_manifest() { :; }
progress() { :; }; warning() { printf '%s\\n' "$*" >&2; }; run() { "$@"; }
# Model a privilege boundary clearing the caller's environment.
run_as_root() { env -i PATH="$PATH" "$@"; }
DRY_RUN=0; NETDATA_MANIFEST_INSTALL=0; INTERACTIVE=0; SYSARCH=x86_64
NETDATA_STATIC_ARCHIVE_NAME=fixture.run
NETDATA_INSTALLER_OPTIONS='--disable-telemetry --dont-start-it'
''' + 'NETDATA_CLAIM_URL=' + shlex.quote(claim_url) + '\n' + \
            'tmpdir=' + shlex.quote(str(self.scratch)) + '\n' + \
            'NETDATA_OFFLINE_INSTALL_SOURCE=' + ('offline' if offline else '') + '\n' + \
            'download() { cp ' + shlex.quote(str(archive)) + ' "$2"; };\ntry_static_install\n'

    def test_failed_reinstall_preserves_configuration_history_and_runtime_proof(self):
        files = ['etc/netdata/netdata.conf', 'var/cache/netdata/history', 'var/lib/netdata/dem/ownership-proof']
        for name in files:
            path = self.prefix / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('preexisting state\n')
        result = self.run_shell(self.kickstart_script(self.archive()))
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn('matching companion unavailable; live files unchanged', result.stderr)
        for name in files:
            self.assertEqual((self.prefix / name).read_text(), 'preexisting state\n')
        self.assertIn('status 37', result.stderr)

    def test_failed_reinstall_preserves_a_preexisting_dangling_prefix_symlink(self):
        self.prefix.symlink_to(self.root / 'missing-target', target_is_directory=True)
        result = self.run_shell(self.kickstart_script(self.archive()))
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertTrue(self.prefix.is_symlink())
        self.assertEqual(self.prefix.readlink(), self.root / 'missing-target')

    def test_failed_fresh_install_removes_partial_prefix_before_fallback(self):
        result = self.run_shell(self.kickstart_script(self.archive(create_prefix=True)))
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertFalse(self.prefix.exists())

    def test_kickstart_static_child_receives_selected_scratch_and_certificate_options(self):
        for offline, mode in [(False, 'check'), (True, 'auto')]:
            with self.subTest(offline=offline):
                script = self.kickstart_script(self.archive(), offline=offline,
                                               claim_url='https://example.invalid/claim?query=a b')
                result = self.run_shell(script)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertEqual(self.record.read_text().splitlines(), [
                    str(self.scratch), 'https://example.invalid/claim?query=a b', mode,
                    '--accept', '--', '--disable-telemetry', '--dont-start-it'])

    def test_kickstart_retains_success_fatal_and_unavailable_return_protocol(self):
        script = self.kickstart_script(self.archive(status=0))
        self.assertEqual(self.run_shell(script).returncode, 0)
        for stub, expected in [('set_static_archive_urls() { return 1; }', 1),
                               ('check_for_remote_file() { return 1; }', 2)]:
            result = self.run_shell(script.replace('try_static_install\n', stub + '\ntry_static_install\n'))
            self.assertEqual(result.returncode, expected, result.stderr)

    def test_kickstart_mktemp_uses_unexported_selected_directory(self):
        function = shell_function(self.kickstart, 'create_tmp_directory')
        for selected in ['explicit', 'fallback']:
            with self.subTest(selected=selected):
                script = function + '\nfatal() { exit 99; };\n'
                # Model /tmp being noexec to exercise the current-directory fallback.
                script += '_cannot_use_tmpdir() { [ "$1" = /tmp ]; };\n'
                script += 'unset TMPDIR\n'
                if selected == 'explicit':
                    script += 'TMPDIR=' + shlex.quote(str(self.scratch)) + '\n'
                result = self.run_shell(script + 'create_tmp_directory\n')
                self.assertEqual(result.returncode, 0, result.stderr)
                created = Path(result.stdout.strip()).resolve()
                self.addCleanup(shutil.rmtree, created)
                self.assertEqual(created.parent, self.scratch)

    def test_updater_static_child_uses_actual_tmpdir_path_and_keeps_failure_inputs(self):
        (self.prefix / 'bin').mkdir(parents=True)
        (self.prefix / 'bin/netdatacli').write_text('#!/bin/sh\nexit 255\n')
        (self.prefix / 'bin/netdatacli').chmod(0o755)
        install_type = self.prefix / 'etc/netdata/.install-type'
        install_type.parent.mkdir(parents=True)
        install_type.write_text("INSTALL_TYPE='fixture-static'\n")
        config = self.prefix / 'etc/netdata/netdata.conf'
        config.write_text('preexisting configuration\n')
        archive = self.archive()
        script = shell_function(self.updater, 'create_exec_tmp_directory') + '\n'
        script += shell_function(self.updater, 'update_static').replace('/opt/netdata', str(self.prefix)) + '\n'
        script += '''
info() { :; }; error() { printf '%s\\n' "$*" >&2; }; fatal() { exit 99; }
update_available() { return 0; }; fetch_release_metadata() { :; }; verify_manifest() { :; }
restore_preview_channel() { exit 91; }
NETDATA_MANIFEST_INSTALL=0; archive_base_name=fixture.run; NETDATA_TARBALL_URL_PREFIX=fixture
REINSTALL_OPTIONS=--disable-telemetry; ndtmpdir=; TMPDIR=/unselected-parent
exec 3>&2
'''
        script += 'NETDATA_TMPDIR_PATH=' + shlex.quote(str(self.scratch)) + '\n'
        script += 'download() { cp ' + shlex.quote(str(archive)) + ' "$2"; };\nupdate_static\n'
        result = self.run_shell(script)
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertIn('status 37', result.stderr)
        self.assertEqual(self.record.read_text().splitlines(), [
            str(self.scratch), 'unset', 'unset', '--accept', '--', '--disable-telemetry', '--dont-start-it'])
        self.assertTrue((self.scratch / 'fixture.run').is_file())
        self.assertEqual(install_type.read_text(), "INSTALL_TYPE='fixture-static'\n")
        self.assertEqual(config.read_text(), 'preexisting configuration\n')


if __name__ == '__main__':
    unittest.main()
