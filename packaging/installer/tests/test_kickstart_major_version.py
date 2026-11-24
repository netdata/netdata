# SPDX-License-Identifier: GPL-3.0-or-later
"""Offline kickstart --install-major-version tests; remote lookups are served from a stubbed URL table."""
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'kickstart.sh'
SOURCE = SCRIPT.read_text()
FUNCTION_NAMES = ('get_actual_version', 'handle_major_version')
FUNCTIONS = '\n'.join(re.search(r'^' + name + r'\(\) \{\n.*?^}', SOURCE, re.M | re.S).group()
                      for name in FUNCTION_NAMES)
CONSTANTS = '\n'.join(re.findall(r'^RELEASE_ARTIFACT_URL_PREFIX=.*$', SOURCE, re.M))


def version_url(channel, release):
    return f'https://artifacts.netdata.cloud/{channel}/{release}/Version'


def url_key(url):
    # Must match url_key() in the shell stubs below.
    return re.sub(r'[^A-Za-z0-9]', '_', url)


class KickstartMajorVersion(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='kickstart-major-version-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.runs = 0

    def run_major(self, table, major, selected='stable', release_channel='default', version='', success=True,
                  assume_present=None, download_status=1):
        """Run handle_major_version with a {url: (check_status, content)} table.

        check_status is the return code of the stubbed check_for_remote_file; content None means that
        downloading the URL fails with download_status. Returns the process result and the list of requested URLs.
        """
        self.runs += 1
        work = self.root / f'run{self.runs}'
        (work / 'table').mkdir(parents=True)
        for url, (status, content) in table.items():
            (work / 'table' / f'{url_key(url)}.status').write_text(str(status))
            if content is not None:
                (work / 'table' / f'{url_key(url)}.content').write_text(content)

        setup = f'''
{CONSTANTS}
{FUNCTIONS}
fatal() {{ echo "FATAL $*" >&2; exit 1; }}
warning() {{ echo "WARNING $*" >&2; }}
progress() {{ :; }}
confirm() {{ echo "CONFIRM $*" >&2; return 1; }}
set_tmpdir() {{ :; }}
url_key() {{ printf '%s' "$1" | tr -c 'A-Za-z0-9' '_'; }}
check_for_remote_file() {{
  echo "check $1" >> urls
  key="$(url_key "$1")"
  [ -f "table/$key.status" ] || return 99
  return "$(cat "table/$key.status")"
}}
download() {{
  echo "download $1 $3" >> urls
  key="$(url_key "$1")"
  [ -f "table/$key.content" ] || return {download_status}
  cp "table/$key.content" "$2"
}}
INTERACTIVE=0
DRY_RUN=0
tmpdir="$PWD"
RELEASE_CHANNEL={shlex.quote(release_channel)}
SELECTED_RELEASE_CHANNEL={shlex.quote(selected)}
INSTALL_MAJOR_VERSION={shlex.quote(major)}
INSTALL_VERSION={shlex.quote(version)}
handle_major_version
printf 'INSTALL_VERSION=%s\\n' "$INSTALL_VERSION"
'''
        env = {k: v for k, v in os.environ.items() if not k.startswith('NETDATA_')}
        if assume_present is not None:
            env['NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT'] = assume_present
        result = subprocess.run(['/bin/sh', '-c', setup], cwd=work, env=env, text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        urls_file = work / 'urls'
        urls = urls_file.read_text().splitlines() if urls_file.exists() else []
        return result, urls

    def test_stable_uses_major_version_file(self):
        url = version_url('stable', '2')
        result, urls = self.run_major({url: (0, 'v2.12.3\n')}, '2')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.3\n')
        self.assertEqual(urls, [f'check {url}', f'download {url} force'])

    def test_nightly_falls_back_to_latest(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (1, None), latest: (0, 'v2.12.0-653-nightly\n')}, '2', selected='nightly')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.0-653-nightly\n')
        self.assertEqual(urls, [f'check {major}', f'check {latest}', f'download {latest} force'])

    def test_latest_of_another_major_continues_with_latest_version(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (1, None), latest: (0, 'v3.0.0-1-nightly\n')}, '2', selected='nightly')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=\n')
        self.assertIn("WARNING No releases with major version '2' have been published", result.stderr)
        self.assertEqual(urls, [f'check {major}', f'check {latest}', f'download {latest} force'])

    def test_major_version_file_of_another_major_is_rejected(self):
        major, latest = version_url('stable', '2'), version_url('stable', 'latest')
        result, urls = self.run_major({major: (0, 'v3.0.0\n'), latest: (0, 'v2.12.3\n')}, '2')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.3\n')
        self.assertEqual(urls, [f'check {major}', f'download {major} force',
                                f'check {latest}', f'download {latest} force'])

    def test_preview_falls_back_to_latest(self):
        major, latest = version_url('preview', '2'), version_url('preview', 'latest')
        result, urls = self.run_major({major: (1, None), latest: (0, 'v2.12.2\n')}, '2', selected='preview')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.2\n')
        self.assertEqual(urls, [f'check {major}', f'check {latest}', f'download {latest} force'])

    def test_lookup_failure_is_fatal_when_not_interactive(self):
        major, latest = version_url('stable', '2'), version_url('stable', 'latest')
        cases = {
            'major file check fails': ({major: (2, None)}, [f'check {major}']),
            'latest check fails': ({major: (1, None), latest: (2, None)}, [f'check {major}', f'check {latest}']),
            'major download fails': ({major: (0, None)}, [f'check {major}', f'download {major} force']),
            'latest download fails': (
                {major: (1, None), latest: (0, None)}, [f'check {major}', f'check {latest}', f'download {latest} force']),
        }
        for name, (table, expected_urls) in cases.items():
            with self.subTest(name):
                result, urls = self.run_major(table, '2', success=False)
                self.assertIn('F0517', result.stderr)
                self.assertIn("channel 'stable'", result.stderr)
                self.assertEqual(urls, expected_urls)

    # With NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT, check_for_remote_file returns 0 without probing, so a
    # missing per-major file only shows up as a failed download (status 0, no content in the stub table).
    def test_assumed_present_major_download_failure_falls_back_to_latest(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (0, None), latest: (0, 'v2.12.0-653-nightly\n')}, '2',
                                      selected='nightly', assume_present='1')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.0-653-nightly\n')
        self.assertEqual(urls, [f'check {major}', f'download {major} force',
                                f'check {latest}', f'download {latest} force'])

    def test_assumed_present_major_and_latest_download_failures_are_fatal(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (0, None), latest: (0, None)}, '2', selected='nightly',
                                      success=False, assume_present='1')
        self.assertIn('F0517', result.stderr)
        self.assertIn("channel 'nightly'", result.stderr)
        self.assertEqual(urls, [f'check {major}', f'download {major} force',
                                f'check {latest}', f'download {latest} force'])

    def test_assumed_present_major_download_failure_with_latest_of_another_major(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (0, None), latest: (0, 'v3.0.0-1-nightly\n')}, '2',
                                      selected='nightly', assume_present='1')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=\n')
        self.assertIn("WARNING No releases with major version '2' have been published", result.stderr)
        self.assertEqual(urls, [f'check {major}', f'download {major} force',
                                f'check {latest}', f'download {latest} force'])

    def test_assumed_present_major_download_errors_are_fatal(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        cases = {
            'connection error': 2,
            'TLS error': 3,
            'HTTP client error': 5,
            'HTTP server error': 6,
        }
        for name, status in cases.items():
            with self.subTest(name):
                result, urls = self.run_major({major: (0, None), latest: (0, 'v3.0.0-1-nightly\n')}, '2',
                                              selected='nightly', success=False, assume_present='1',
                                              download_status=status)
                self.assertIn('F0517', result.stderr)
                self.assertIn("channel 'nightly'", result.stderr)
                self.assertEqual(urls, [f'check {major}', f'download {major} force'])

    def test_empty_assume_present_major_download_not_found_is_fatal(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (0, None), latest: (0, 'v3.0.0-1-nightly\n')}, '2',
                                      selected='nightly', success=False, assume_present='', download_status=1)
        self.assertIn('F0517', result.stderr)
        self.assertIn("channel 'nightly'", result.stderr)
        self.assertEqual(urls, [f'check {major}', f'download {major} force'])

    def test_resolved_channel_is_used(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        result, urls = self.run_major({major: (1, None), latest: (0, 'v2.12.0-653-nightly\n')}, '2',
                                      selected='nightly', release_channel='default')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=2.12.0-653-nightly\n')
        self.assertTrue(urls)
        for url in urls:
            self.assertIn('/nightly/', url)
            self.assertNotIn('/default/', url)

    def test_leading_v_in_major_version_is_accepted(self):
        url = version_url('stable', '2')
        table = {url: (0, 'v2.12.3\n')}
        plain, plain_urls = self.run_major(table, '2')
        prefixed, prefixed_urls = self.run_major(table, 'v2')
        self.assertEqual(prefixed.stdout, 'INSTALL_VERSION=2.12.3\n')
        self.assertEqual(prefixed.stdout, plain.stdout)
        self.assertEqual(prefixed_urls, plain_urls)
        self.assertEqual(prefixed_urls, [f'check {url}', f'download {url} force'])

    def test_invalid_major_version_is_fatal(self):
        for major in ('2/../x', 'v', 'vv2', '2.1', 'abc', '2 '):
            with self.subTest(major=major):
                result, urls = self.run_major({}, major, success=False)
                self.assertIn('F0534', result.stderr)
                self.assertEqual(urls, [])

    def test_empty_major_version_does_nothing(self):
        result, urls = self.run_major({}, '', version='keep')
        self.assertEqual(result.stdout, 'INSTALL_VERSION=keep\n')
        self.assertEqual(urls, [])

    def test_version_file_content_is_not_mangled(self):
        major, latest = version_url('nightly', '2'), version_url('nightly', 'latest')
        contents = {
            'v inside the version': ('v2.12.0-1-dev\n', '2.12.0-1-dev'),
            'surrounding whitespace': ('  v2.12.0-1-dev \t\n', '2.12.0-1-dev'),
            'no trailing newline': ('v2.12.0-1-dev', '2.12.0-1-dev'),
            'no leading v': ('2.12.0-1-dev\n', '2.12.0-1-dev'),
        }
        for name, (content, expected) in contents.items():
            with self.subTest(name):
                result, _ = self.run_major({major: (1, None), latest: (0, content)}, '2', selected='nightly')
                self.assertEqual(result.stdout, f'INSTALL_VERSION={expected}\n')


if __name__ == '__main__':
    unittest.main()
