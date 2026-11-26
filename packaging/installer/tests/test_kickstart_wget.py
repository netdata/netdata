# SPDX-License-Identifier: GPL-3.0-or-later
"""Offline kickstart wget result tests; requests only reach loopback servers started by the tests."""
import http.server
import os
from pathlib import Path
import re
import shlex
import shutil
import socket
import subprocess
import tempfile
import threading
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'kickstart.sh'
SOURCE = SCRIPT.read_text()
FUNCTION_NAMES = ('handle_http_status', 'handle_wget_result', 'wget_http_status', 'check_for_remote_file', 'download',
                  'get_actual_version')
FUNCTIONS = '\n'.join(re.search(r'^' + name + r'\(\) \{\n.*?^}', SOURCE, re.M | re.S).group()
                      for name in FUNCTION_NAMES)
# The run wrappers only add logging and dry-run handling around the command.
STUBS = '''
fatal() { echo "FATAL $*" >&2; exit 1; }
warning() { echo "WARNING $*" >&2; }
progress() { :; }
set_tmpdir() { :; }
run() { "$@"; }
always_run() { "$@"; }
'''

CHECK = 'checking for remote file at'
DOWNLOAD = 'downloading'

GNU_LOG_404 = '''--2026-11-26 10:00:00--  https://artifacts.netdata.cloud/stable/2/Version
Resolving artifacts.netdata.cloud (artifacts.netdata.cloud)... 104.20.22.2
Connecting to artifacts.netdata.cloud (artifacts.netdata.cloud)|104.20.22.2|:443... connected.
HTTP request sent, awaiting response...
  HTTP/1.1 404 Not Found
  Date: Thu, 26 Nov 2026 10:00:00 GMT
  Content-Type: text/plain
2026-11-26 10:00:00 ERROR 404: Not Found.
'''
GNU_LOG_503 = GNU_LOG_404.replace('404 Not Found', '503 Service Unavailable').replace(
    'ERROR 404: Not Found', 'ERROR 503: Service Unavailable')
GNU_LOG_403 = GNU_LOG_404.replace('404 Not Found', '403 Forbidden').replace(
    'ERROR 404: Not Found', 'ERROR 403: Forbidden')
GNU_LOG_REDIRECT_404 = GNU_LOG_404.replace(
    '  HTTP/1.1 404 Not Found', '  HTTP/1.1 301 Moved Permanently\n  Location: /other\n  HTTP/1.1 404 Not Found')
BUSYBOX_LOG_404 = '''Connecting to artifacts.netdata.cloud (104.20.22.2:443)
  HTTP/1.1 404 Not Found
wget: server returned error: HTTP/1.1 404 Not Found
'''
BUSYBOX_LOG_503 = BUSYBOX_LOG_404.replace('404 Not Found', '503 Service Unavailable')
BUSYBOX_LOG_NO_STATUS = "wget: bad address 'nonexistent.invalid'\n"

PROXY_VARIABLES = ('http_proxy', 'https_proxy', 'ftp_proxy', 'all_proxy', 'no_proxy')


def clean_env():
    # BusyBox wget honors http_proxy, which would divert the loopback requests.
    return {k: v for k, v in os.environ.items() if not k.startswith('NETDATA_') and k.lower() not in PROXY_VARIABLES}


def closed_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


class Handler(http.server.BaseHTTPRequestHandler):
    # HTTP/1.1 matches the status line of the real artifact server.
    protocol_version = 'HTTP/1.1'

    def do_GET(self):
        self.server.requested.append(self.path)
        status, body = self.server.routes.get(self.path, (404, b'not found\n'))
        self.send_response(status)
        self.send_header('Content-Type', 'text/plain')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_HEAD = do_GET

    def log_message(self, *args):
        pass


class KickstartWgetResult(unittest.TestCase):
    """handle_wget_result on log texts, for the GNU wget and BusyBox wget formats."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='kickstart-wget-result-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def result(self, ret, log, dest=''):
        (self.root / 'wget.log').write_text(log)
        setup = f'''
{FUNCTIONS}
{STUBS}
handle_wget_result {ret} https://example.test/file wget.log {shlex.quote(CHECK)} {shlex.quote(dest)}
echo "RET=$?"
'''
        return subprocess.run(['/bin/sh', '-c', setup], cwd=self.root, env=clean_env(), text=True,
                              capture_output=True)

    def test_exit_status_decoding(self):
        not_found = f'WARNING File not found when {CHECK} https://example.test/file\n'
        cases = {
            'GNU 404': (8, GNU_LOG_404, 'RET=1', not_found),
            'GNU 403': (8, GNU_LOG_403, 'RET=5',
                        f'WARNING Server returned 403 when {CHECK} https://example.test/file\n'),
            'GNU 503': (8, GNU_LOG_503, 'RET=6',
                        f'WARNING Server returned 503 when {CHECK} https://example.test/file\n'),
            'GNU redirect then 404': (8, GNU_LOG_REDIRECT_404, 'RET=1', not_found),
            'GNU HTTP/2 status': (8, GNU_LOG_404.replace('HTTP/1.1', 'HTTP/2'), 'RET=1', not_found),
            'GNU without status line': (8, 'wget: error\n', 'RET=4',
                                        f'WARNING Server returned  when {CHECK} https://example.test/file\n'),
            'GNU connection failure': (4, GNU_LOG_404, 'RET=2',
                                       f'WARNING Failed to connect to remote host when {CHECK} '
                                       'https://example.test/file\n'),
            'GNU TLS failure': (5, GNU_LOG_404, 'RET=3',
                                f'WARNING TLS error while connecting to remote host when {CHECK} '
                                'https://example.test/file\n'),
            'BusyBox 404': (1, BUSYBOX_LOG_404, 'RET=1', not_found),
            'BusyBox 403': (1, BUSYBOX_LOG_404.replace('404 Not Found', '403 Forbidden'), 'RET=5',
                            f'WARNING Server returned 403 when {CHECK} https://example.test/file\n'),
            'BusyBox 503': (1, BUSYBOX_LOG_503, 'RET=6',
                            f'WARNING Server returned 503 when {CHECK} https://example.test/file\n'),
            'BusyBox other HTTP status': (
                1, BUSYBOX_LOG_404.replace('404 Not Found', '302 Found'), 'RET=4',
                f'WARNING Server returned 302 when {CHECK} https://example.test/file\n'),
        }
        for name, (ret, log, expected_stdout, expected_stderr) in cases.items():
            with self.subTest(name):
                result = self.result(ret, log)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, expected_stdout + '\n')
                self.assertEqual(result.stderr, expected_stderr)

    def test_failures_without_http_status_are_fatal(self):
        cases = {
            'BusyBox without status line': (1, BUSYBOX_LOG_NO_STATUS),
            'BusyBox empty log': (1, ''),
            'unknown GNU exit status': (2, GNU_LOG_404),
        }
        for name, (ret, log) in cases.items():
            with self.subTest(name):
                result = self.result(ret, log)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, '')
                self.assertEqual(result.stderr, f'FATAL Unknown error when {CHECK} https://example.test/file F0520\n')

    def test_destination_is_removed_on_http_errors(self):
        dest = self.root / 'partial'
        for name, ret, log in (('GNU', 8, GNU_LOG_404), ('BusyBox', 1, BUSYBOX_LOG_404)):
            with self.subTest(name):
                dest.write_text('partial')
                self.assertEqual(self.result(ret, log, str(dest)).stdout, 'RET=1\n')
                self.assertFalse(dest.exists())


@unittest.skipUnless(shutil.which('busybox'), 'busybox is not installed')
class KickstartBusyBoxWget(unittest.TestCase):
    """The kickstart download helpers with the real BusyBox wget against a loopback HTTP server."""

    @classmethod
    def setUpClass(cls):
        cls.server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        cls.server.routes = {}
        cls.server.requested = []
        cls.base = f'http://127.0.0.1:{cls.server.server_port}'
        thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        thread.start()
        cls.addClassCleanup(thread.join)
        cls.addClassCleanup(cls.server.server_close)
        cls.addClassCleanup(cls.server.shutdown)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='kickstart-busybox-wget-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.wget = self.root / 'wget-busybox'
        self.wget.write_text('#!/bin/sh\nexec busybox wget "$@"\n')
        self.wget.chmod(0o755)
        self.server.routes = {}
        self.server.requested = []

    def shell(self, body, env=None):
        setup = f'''
{FUNCTIONS}
{STUBS}
WGET={shlex.quote(str(self.wget))}
CURL=
tmpdir="$PWD"
RELEASE_ARTIFACT_URL_PREFIX={self.base}
{body}
'''
        return subprocess.run(['/bin/sh', '-c', setup], cwd=self.root, env={**clean_env(), **(env or {})}, text=True,
                              capture_output=True, timeout=60)

    def helper(self, function, url):
        if function == 'download':
            body = f'download {shlex.quote(url)} "$PWD/file" force'
        else:
            body = f'check_for_remote_file {shlex.quote(url)}'
        return self.shell(body + '\necho "RET=$?"')

    def test_busybox_wget_failure_logs(self):
        # The behavior handle_wget_result relies on: exit status 1 plus a logged HTTP status line.
        self.server.routes = {'/ok': (200, b'body\n'), '/missing': (404, b'no\n'), '/down': (503, b'no\n')}
        cases = {
            '/ok': (0, 'HTTP/1.1 200 OK'),
            '/missing': (1, 'HTTP/1.1 404 Not Found'),
            '/down': (1, 'HTTP/1.1 503 Service Unavailable'),
        }
        for path, (expected_ret, expected_log) in cases.items():
            with self.subTest(path):
                log = self.root / 'wget.log'
                result = subprocess.run(['busybox', 'wget', '-S', '-o', str(log), '--spider', self.base + path],
                                        env=clean_env(), capture_output=True, timeout=60)
                self.assertEqual(result.returncode, expected_ret)
                log_text = log.read_text()
                self.assertIn(f'  {expected_log}\n', log_text)
                if expected_ret:
                    self.assertIn(f'wget: server returned error: {expected_log}\n', log_text)

    def test_helpers_decode_http_status(self):
        self.server.routes = {'/ok': (200, b'body\n'), '/missing': (404, b'no\n'), '/down': (503, b'no\n')}
        cases = {
            'found': ('/ok', 'RET=0', ''),
            'not found': ('/missing', 'RET=1', 'File not found when {action} {url}'),
            'server error': ('/down', 'RET=6', 'Server returned 503 when {action} {url}'),
        }
        for function, action in (('check_for_remote_file', CHECK), ('download', DOWNLOAD)):
            for name, (path, expected_stdout, expected_warning) in cases.items():
                with self.subTest(function=function, case=name):
                    url = self.base + path
                    result = self.helper(function, url)
                    self.assertEqual(result.stdout, expected_stdout + '\n')
                    expected_stderr = f'WARNING {expected_warning.format(action=action, url=url)}\n' \
                        if expected_warning else ''
                    self.assertEqual(result.stderr, expected_stderr)
                    if function == 'download':
                        self.assertEqual((self.root / 'file').exists(), path == '/ok')
                        (self.root / 'file').unlink(missing_ok=True)

    def test_downloaded_content_is_kept(self):
        self.server.routes = {'/ok': (200, b'body\n')}
        self.assertEqual(self.helper('download', self.base + '/ok').stdout, 'RET=0\n')
        self.assertEqual((self.root / 'file').read_bytes(), b'body\n')

    # BusyBox wget exits 1 for a refused connection and logs no HTTP status line, so it stays an unknown error.
    def test_connection_refused_is_still_a_failure(self):
        base = f'http://127.0.0.1:{closed_port()}'
        for function, action in (('check_for_remote_file', CHECK), ('download', DOWNLOAD)):
            with self.subTest(function):
                url = f'{base}/Version'
                result = self.helper(function, url)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, '')
                self.assertIn(f'FATAL Unknown error when {action} {url} F0520', result.stderr)

    def actual_version(self, routes, assume_present=None):
        self.server.routes = routes
        self.server.requested = []
        env = {} if assume_present is None else {'NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT': assume_present}
        return self.shell('get_actual_version 2 nightly', env)

    def test_actual_version_falls_back_to_latest_on_missing_major_file(self):
        routes = {'/nightly/latest/Version': (200, b'v2.13.0-54-nightly\n')}
        for name, assume_present in (('checked', None), ('assumed present', '1')):
            with self.subTest(name):
                result = self.actual_version(routes, assume_present)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, '2.13.0-54-nightly\n')
                self.assertEqual(result.stderr, '')
                expected = ['/nightly/2/Version', '/nightly/latest/Version', '/nightly/latest/Version']
                if assume_present:
                    expected = ['/nightly/2/Version', '/nightly/latest/Version']
                self.assertEqual(self.server.requested, expected)

    def test_actual_version_server_error_is_a_lookup_failure(self):
        routes = {'/nightly/2/Version': (503, b'down\n'), '/nightly/latest/Version': (200, b'v2.13.0-54-nightly\n')}
        for name, assume_present, action in (('checked', None, CHECK), ('assumed present', '1', DOWNLOAD)):
            with self.subTest(name):
                result = self.actual_version(routes, assume_present)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, '\n')
                self.assertEqual(result.stderr,
                                 f'WARNING Server returned 503 when {action} {self.base}/nightly/2/Version\n')
                self.assertEqual(self.server.requested, ['/nightly/2/Version'])


if __name__ == '__main__':
    unittest.main()
