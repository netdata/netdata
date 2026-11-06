#!/usr/bin/env python3
"""Offline trust-policy and loopback TLS regression tests for standalone scripts."""
import http.server
import os
from pathlib import Path
import shutil
import ssl
import sys
import subprocess
import tempfile
import threading
import unittest

ROOT = Path(__file__).resolve().parents[3]
SCRIPTS = [ROOT / 'packaging/installer' / name for name in
           ('kickstart.sh', 'netdata-updater.sh')]
START = '# Keep this block identical in kickstart.sh and netdata-updater.sh:'
END = '# End standalone CA filtering block.'


def helper(path):
    text = path.read_text()
    return text[text.index(START):text.index(END) + len(END)]


@unittest.skipUnless(shutil.which('openssl'), 'openssl required')
class RejectCertificatesTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='netdata-ca-tests-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.base = Path(cls.temp.name)
        cls.certificates = []
        cls.fingerprints = []
        for number in range(3):
            cert = cls.base / f'ca{number}.pem'
            key = cls.base / f'ca{number}.key'
            subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
                            '-keyout', str(key), '-out', str(cert), '-days', '1',
                            '-subj', '/CN=localhost', '-addext', 'subjectAltName=DNS:localhost',
                            '-addext', 'basicConstraints=critical,CA:TRUE'],
                           check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            cls.certificates.append(cert)
            cls.fingerprints.append(subprocess.check_output(
                ['openssl', 'x509', '-in', str(cert), '-noout', '-fingerprint', '-sha256'],
                text=True).strip().split('=', 1)[1])
        subprocess.run(['openssl', 'rehash', str(cls.base)], check=True, capture_output=True)
        cls.bundle = cls.base / 'system.pem'
        cls.bundle.write_bytes(b''.join(p.read_bytes() for p in cls.certificates))

    def run_shell(self, body, fingerprints=None, source=None, shell=None, extra_env=None):
        with tempfile.TemporaryDirectory(prefix='netdata-ca-run-') as temp:
            env = dict(os.environ, TMPDIR=temp, SSL_CERT_FILE=str(source or self.bundle),
                       NETDATA_REJECT_CERT_FINGERPRINTS=(fingerprints if fingerprints is not None
                                                        else self.fingerprints[0]))
            # Keep user configuration and proxies out of the local fixtures.
            for key in list(env):
                if key.lower().endswith('_proxy'):
                    del env[key]
            env.update(extra_env or {})
            env['HOME'] = temp
            result = subprocess.run((shell or ['sh']) + ['-c', helper(SCRIPTS[0]) + '''
trap 'test -z "$NETDATA_CA_DIR" || rm -rf "$NETDATA_CA_DIR"' 0
''' + body], env=env, capture_output=True, text=True)
            return result

    def test_standalone_copies_match(self):
        self.assertEqual(helper(SCRIPTS[0]), helper(SCRIPTS[1]))

    def test_normalization_and_multiple_hash_algorithms(self):
        sha1 = subprocess.check_output(['openssl', 'x509', '-in', str(self.certificates[1]),
                                       '-noout', '-fingerprint', '-sha1'], text=True).strip().split('=')[1]
        fingerprints = self.fingerprints[0].replace(':', '').lower() + ',\n' + sha1
        shells = [['sh'], ['bash']]
        if shutil.which('busybox'):
            shells.append(['busybox', 'ash'])
        for shell in shells:
            with self.subTest(shell=shell):
                result = self.run_shell('setup_filtered_ca_bundle || exit 1\ncat "$NETDATA_CA_BUNDLE"',
                                        fingerprints, shell=shell)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, self.certificates[2].read_text())

    def test_invalid_policy_and_source_fail_closed(self):
        for policy in ('wrong', '*', ' ', ',,,', '12:34', 'a' * 63, 'g' * 64):
            with self.subTest(policy=policy):
                result = self.run_shell('setup_filtered_ca_bundle', policy)
                self.assertNotEqual(result.returncode, 0)
        for contents in ('', 'not a certificate', '-----BEGIN CERTIFICATE-----\nbroken',
                         self.bundle.read_text() + '\n-----BEGIN CERTIFICATE-----\n'):
            with tempfile.NamedTemporaryFile(mode='w') as source:
                source.write(contents)
                source.flush()
                self.assertNotEqual(self.run_shell('setup_filtered_ca_bundle', source=source.name).returncode, 0)
        self.assertNotEqual(self.run_shell('setup_filtered_ca_bundle', source='/does/not/exist').returncode, 0)

    def test_rejecting_all_roots_fails_closed(self):
        result = self.run_shell('setup_filtered_ca_bundle', ','.join(self.fingerprints))
        self.assertNotEqual(result.returncode, 0)

    def test_setup_failure_cleanup_and_reinitialization(self):
        result = self.run_shell('''
setup_filtered_ca_bundle || exit 1
first="$NETDATA_CA_DIR"
[ "$(ls -ld "$first" | cut -c2-10)" = rwx------ ] || exit 2
setup_filtered_ca_bundle || exit 3
[ "$first" = "$NETDATA_CA_DIR" ] || exit 4
cleanup_filtered_ca_bundle
[ ! -e "$first" ] && [ -z "$NETDATA_CA_BUNDLE" ] || exit 5
setup_filtered_ca_bundle || exit 6
cleanup_filtered_ca_bundle
cleanup_filtered_ca_bundle
''')
        self.assertEqual(result.returncode, 0, result.stderr)
        result = self.run_shell('openssl() { return 1; }; setup_filtered_ca_bundle')
        self.assertNotEqual(result.returncode, 0)

    def test_unset_policy_preserves_client_arguments(self):
        result = self.run_shell('''
setup_filtered_ca_bundle || exit 1
[ -z "$NETDATA_CA_DIR" ] || exit 2
netdata_curl printf '%s\\n' 'one argument' --example
netdata_wget printf '%s\\n' 'one argument' --example
''', '')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, 'one argument\n--example\none argument\n--example\n')

    def test_clients_cannot_run_before_successful_setup(self):
        result = self.run_shell('netdata_curl echo bypass; netdata_wget echo bypass')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')

    def test_all_direct_requests_use_helpers(self):
        import re
        for script in SCRIPTS:
            text = script.read_text()
            for line in text.splitlines():
                if re.search(r'"\$\{(?:CURL|WGET|curl|wget)\}"\s+(?:--?|"\$\{url\}")', line):
                    if '--help' in line:
                        continue
                    self.assertRegex(line, r'netdata_(curl|wget) ', (script.name, line))
            self.assertFalse(any('--no-check-certificate' in line and not line.lstrip().startswith('#')
                                 for line in text.splitlines()), script.name)
            self.assertNotIn("always_run sh -c", text)

    def test_real_tls_clients_accept_retained_and_reject_removed_root(self):
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200)
                self.send_header('Content-Length', '3')
                self.end_headers()
                self.wfile.write(b'ok\n')

            def log_message(self, *_args):
                pass

        class Server(http.server.ThreadingHTTPServer):
            def handle_error(self, request, client_address):
                # Clients may close immediately after rejecting a certificate.
                if not isinstance(sys.exc_info()[1], (ConnectionResetError, BrokenPipeError)):
                    super().handle_error(request, client_address)

        for number in (0, 1):
            server = Server(('127.0.0.1', 0), Handler)
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain(str(self.certificates[number]), str(self.base / f'ca{number}.key'))
            server.socket = context.wrap_socket(server.socket, server_side=True)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                for client in ('curl', 'wget'):
                    if not shutil.which(client):
                        continue
                    with self.subTest(root=number, client=client):
                        args = '--show-error --max-time 3' if client == 'curl' else '-q -O - -T 3 --tries=1'
                        url = f'https://localhost:{server.server_port}/'
                        result = self.run_shell(f'''setup_filtered_ca_bundle || exit 90
# The rejected root remains in an externally configured CA directory.
# Explicit empty --capath/--ca-directory must prevent that fallback.
SSL_CERT_DIR="{self.base}"
export SSL_CERT_DIR
netdata_{client} {shutil.which(client)} {args} '{url}'
''')
                        if number == 0:
                            self.assertNotEqual(result.returncode, 0, result.stdout)
                        else:
                            self.assertEqual(result.returncode, 0, result.stderr)
                            self.assertEqual(result.stdout, 'ok\n')
                if shutil.which('curl'):
                    with self.subTest(root=number, client='curl HTTPS proxy'):
                        result = self.run_shell(
                            'setup_filtered_ca_bundle || exit 90\n'
                            'netdata_curl curl --show-error --max-time 3 http://example.invalid/',
                            extra_env={'http_proxy': '', 'HTTPS_PROXY': '',
                                       'ALL_PROXY': f'https://localhost:{server.server_port}'})
                        if number == 0:
                            self.assertNotEqual(result.returncode, 0, result.stdout)
                        else:
                            self.assertEqual(result.returncode, 0, result.stderr)
                            self.assertEqual(result.stdout, 'ok\n')
            finally:
                server.shutdown()
                thread.join()
                server.server_close()


if __name__ == '__main__':
    unittest.main()
