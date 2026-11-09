#!/usr/bin/env python3
"""Test kickstart release authentication without installing anything or using the network."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'kickstart.sh'
SOURCE = SCRIPT.read_text()
FUNCTIONS = SOURCE[SOURCE.index('verify_release_signature()'):SOURCE.index('verify_manifest()')]


class SignatureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='kickstart-signature-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / 'home'
        self.home.mkdir(mode=0o700)
        self.env = dict(os.environ, GNUPGHOME=str(self.home), HOME=str(self.root))
        self.manifest = self.root / 'Manifest'
        self.manifest.write_text('archive 10 hash\n')
        self.harness = self.root / 'harness.sh'
        self.harness.write_text('''
progress() { echo "$*"; }
warning() { echo "$*" >&2; }
fatal() { echo "$*" >&2; exit 1; }
set_tmpdir() { tmpdir="$TEST_ROOT"; }
confirm() { echo "$*"; [ "${TEST_CONFIRM:-n}" = y ]; }
download() {
  case "$1" in
    */Manifest.sig) cp "$TEST_ROOT/published.sig" "$2" ;;
    *) echo fetched >> "$TEST_ROOT/fetches"; cp "$TEST_ROOT/public.key" "$2" ;;
  esac
}
''' + FUNCTIONS + '\n"$@"\n')
        self.env['TEST_ROOT'] = str(self.root)

    def run_shell(self, *args, **env):
        return subprocess.run(['/bin/sh', str(self.harness), *map(str, args)],
                              cwd=self.root, env=dict(self.env, **env),
                              capture_output=True, text=True)

    def pin_fixture_key(self):
        """Pin the fixture key, since Netdata's private key is not available."""
        listing = subprocess.run(['gpg', '--batch', '--with-colons', '--list-keys'],
                                 env=self.env, check=True, capture_output=True, text=True).stdout
        fingerprint = next(line.split(':')[9] for line in listing.splitlines()
                           if line.startswith('fpr:'))
        self.harness.write_text(self.harness.read_text().replace(
            '6E155DC153906B73765A74A99DD4A74CECFA8F4F', fingerprint))
        return fingerprint

    def make_key(self):
        if not shutil.which('gpg'):
            self.skipTest('gpg needed to generate fixture')
        subprocess.run(['gpg', '--batch', '--pinentry-mode', 'loopback', '--passphrase', '',
                        '--quick-generate-key', 'Fixture <fixture@example.invalid>',
                        'rsa2048', 'sign', '1d'], env=self.env, check=True, capture_output=True)
        public = subprocess.run(['gpg', '--batch', '--armor', '--export'], env=self.env,
                                check=True, capture_output=True).stdout
        (self.root / 'public.key').write_bytes(public)
        subprocess.run(['gpg', '--batch', '--detach-sign', '--output',
                        str(self.root / 'published.sig'), str(self.manifest)],
                       env=self.env, check=True, capture_output=True)

    def test_real_backends_valid_and_tampered(self):
        self.make_key()
        for tool in ('gpg', 'gpgv', 'sq', 'sqv', 'rnp', 'enc'):
            if not shutil.which(tool):
                continue
            with self.subTest(tool=tool):
                work = self.root / tool
                work.mkdir()
                args = ('verify_openpgp_signature', tool, self.root / 'public.key',
                        self.root / 'published.sig', self.manifest, work)
                result = self.run_shell(*args)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.manifest.write_text('tampered\n')
                bad_work = self.root / (tool + '-bad')
                bad_work.mkdir()
                result = self.run_shell(*args[:-1], bad_work)
                self.assertNotEqual(result.returncode, 0)
                self.manifest.write_text('archive 10 hash\n')

    def test_policy_skip_and_dry_run(self):
        for env, message in (({'NETDATA_VERIFY_RELEASE_SIGNATURES': '0'}, 'malicious'),
                             ({'DRY_RUN': '1'}, 'Would verify')):
            result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                    **dict({'NETDATA_VERIFY_RELEASE_SIGNATURES': '1',
                                            'DRY_RUN': '0', 'NETDATA_MANIFEST_INSTALL': '1'}, **env))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(message, result.stdout + result.stderr)
            self.assertFalse((self.root / 'fetches').exists())

    def test_unsigned_release_rejected(self):
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='0')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('--no-release-signature-verification', result.stderr)

    def test_fetch_does_not_change_user_keyring(self):
        self.make_key()
        # The pinned key is absent from the keyring in use, so it must be fetched.
        self.pin_fixture_key()
        other_home = self.root / 'other-home'
        other_home.mkdir(mode=0o700)
        before = {p.relative_to(self.home): p.read_bytes() for p in self.home.rglob('*') if p.is_file()}
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1', GNUPGHOME=str(other_home))
        self.assertEqual(result.returncode, 0, result.stderr)
        after = {p.relative_to(self.home): p.read_bytes() for p in self.home.rglob('*') if p.is_file()}
        self.assertEqual(before, after)
        self.assertEqual((self.root / 'fetches').read_text(), 'fetched\n')

    def test_existing_repository_key_is_reused(self):
        self.make_key()
        self.pin_fixture_key()
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / 'fetches').exists())

    def test_policy_rejects_tampering_without_creating_user_keyring(self):
        self.make_key()
        self.manifest.write_text('tampered manifest')
        empty_home = self.root / 'new-home'
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1', GNUPGHOME=str(empty_home))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Refusing to install', result.stderr)
        self.assertFalse(empty_home.exists())

    def test_no_verifier_is_fatal(self):
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1', PATH=str(self.root / 'absent'))
        self.assertNotEqual(result.returncode, 0)
        for word in ('gpg', 'sq', '--no-release-signature-verification', 'malicious'):
            self.assertIn(word, result.stderr)
        for word in ('gpgv', 'sqv', 'rnp', 'enc'):
            self.assertNotIn(word, result.stderr)

    def test_fingerprint_check(self):
        self.make_key()
        fingerprint = self.pin_fixture_key()
        other = self.root / 'other'
        other.mkdir(mode=0o700)
        subprocess.run(['gpg', '--batch', '--pinentry-mode', 'loopback', '--passphrase', '',
                        '--quick-generate-key', 'Other <other@example.invalid>',
                        'rsa2048', 'sign', '1d'], env=dict(self.env, GNUPGHOME=str(other)),
                       check=True, capture_output=True)
        other_key = subprocess.run(['gpg', '--batch', '--armor', '--export'],
                                   env=dict(self.env, GNUPGHOME=str(other)),
                                   check=True, capture_output=True).stdout
        both = self.root / 'both.key'
        both.write_bytes((self.root / 'public.key').read_bytes() + other_key)
        (self.root / 'other.key').write_bytes(other_key)
        cases = {'pinned': (self.root / 'public.key', 0),
                 'different key': (self.root / 'other.key', 1),
                 'extra certificate': (both, 1)}
        for tool in ('gpg', 'sq'):
            if not shutil.which(tool):
                continue
            for name, (key, expected) in cases.items():
                with self.subTest(tool=tool, case=name):
                    work = Path(tempfile.mkdtemp(dir=self.root))
                    result = self.run_shell('check_release_key_fingerprint', tool, key,
                                            fingerprint, work)
                    self.assertEqual(result.returncode != 0, bool(expected), result.stderr)

    def test_mismatched_downloaded_key_rejected(self):
        self.make_key()
        other_home = self.root / 'other-home'
        other_home.mkdir(mode=0o700)
        # The fixture key does not carry Netdata's real fingerprint.
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1', GNUPGHOME=str(other_home))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('expected fingerprint', result.stderr)

    def test_unverified_key_policy(self):
        cases = {'non-interactive refuses': ({'INTERACTIVE': '0'}, False),
                 'non-interactive with flag warns': ({'INTERACTIVE': '0',
                                                      'NETDATA_ALLOW_UNVERIFIED_RELEASE_KEYS': '1'}, True),
                 'interactive with flag skips prompt': ({'INTERACTIVE': '1', 'TEST_CONFIRM': 'n',
                                                         'NETDATA_ALLOW_UNVERIFIED_RELEASE_KEYS': '1'}, True),
                 'interactive accepted': ({'INTERACTIVE': '1', 'TEST_CONFIRM': 'y'}, True),
                 'interactive declined': ({'INTERACTIVE': '1', 'TEST_CONFIRM': 'n'}, False)}
        for name, (env, ok) in cases.items():
            with self.subTest(case=name):
                result = self.run_shell('confirm_unverified_release_key', 'gpgv',
                                        **dict({'NETDATA_ALLOW_UNVERIFIED_RELEASE_KEYS': '0'}, **env))
                self.assertEqual(result.returncode == 0, ok, result.stderr)
                self.assertIn('genuine Netdata key', result.stdout + result.stderr)

    def test_missing_signature_rejected(self):
        result = self.run_shell('verify_release_signature', 'https://example.invalid',
                                NETDATA_VERIFY_RELEASE_SIGNATURES='1', DRY_RUN='0',
                                NETDATA_MANIFEST_INSTALL='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Unable to download', result.stderr)

    def test_wiring_precedes_manifest_parsing(self):
        for start, end, prefix in [('try_static_install() {', 'set_source_archive_urls() {', 'STATIC'),
                                   ('try_build_install() {', 'prepare_offline_install_source()', 'SOURCE')]:
            body = SOURCE[SOURCE.index(start):]
            if end in body:
                body = body[:body.index(end)]
            self.assertIn(f'verify_release_signature "${{NETDATA_{prefix}_ARCHIVE_URL_PREFIX}}" || fatal', body)
            marker = 'archive_info=' if prefix == 'STATIC' else 'zstd_info='
            self.assertLess(body.index('verify_release_signature'), body.index(marker))


if __name__ == '__main__':
    unittest.main()
