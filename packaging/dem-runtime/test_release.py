# SPDX-License-Identifier: GPL-3.0-or-later
"""Publisher control flow and real Go/GPG signature integration regressions."""
import copy
import base64
import hashlib
import io
import json
from pathlib import Path
import os
import shutil
import subprocess
import tempfile
import tarfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import release


def candidate(arch, sequence=2, version='2.0'):
    return {'sequence': sequence, 'manifest': {'version': version, 'platform': 'linux/' + arch}}


class PublicationTests(unittest.TestCase):
    def test_canonical_key_acquisition_is_bounded_and_has_no_failure_fallback(self):
        with tempfile.TemporaryDirectory() as temporary:
            args = SimpleNamespace(output=Path(temporary) / 'public.gpg')
            for body in (b'public keyring', b'', b'x' * (release.PUBLIC_KEY_LIMIT + 1)):
                args.output.write_bytes(b'prior keyring')
                with patch.object(release.HTTPS_OPENER, 'open', return_value=io.BytesIO(body)) as opened:
                    if body == b'public keyring':
                        release.fetch_public_key(args)
                        self.assertEqual(args.output.read_bytes(), body)
                    else:
                        with self.assertRaises(ValueError):
                            release.fetch_public_key(args)
                        self.assertEqual(args.output.read_bytes(), b'prior keyring')
                    opened.assert_called_once_with(release.PUBLIC_KEY_URL,
                                                   timeout=release.DOWNLOAD_TIMEOUT_SECONDS)
            with patch.object(release.HTTPS_OPENER, 'open', side_effect=OSError('unavailable')):
                with self.assertRaises(OSError):
                    release.fetch_public_key(args)

    def test_sequence_guard_and_partial_promotion_retry(self):
        proposed = {arch: candidate(arch) for arch in release.ARCHITECTURES}
        release.guard_sequence({}, proposed)
        release.guard_sequence(copy.deepcopy(proposed), proposed)
        release.guard_sequence({'amd64': candidate('amd64'), 'arm64': candidate('arm64', 1, '1.0')}, proposed)
        bad = [
            {'arm64': candidate('arm64', 3, '3.0')},
            {'amd64': candidate('amd64', 2, 'other')},
            {'amd64': candidate('amd64', 1, '2.0')},
            {'amd64': candidate('arm64', 1, '1.0')},
        ]
        for current in bad:
            with self.subTest(current=current), self.assertRaises(ValueError):
                release.guard_sequence(current, proposed)

    def test_current_metadata_reads_storage_and_fails_closed(self):
        remote = 's3:bucket/dem-runtime/latest/amd64.json'
        with patch.object(release, 'run', side_effect=[json.dumps({'IsDir': False, 'Size': 2}), '{}']) as run:
            self.assertEqual(release.read_latest(remote), b'{}')
            self.assertEqual(run.call_args_list[0].args[0], ['rclone', 'lsjson', '--stat', remote])
            self.assertIn('--count', run.call_args_list[1].args[0])
        for status in (3, 4, 1, 5, 7):
            with self.subTest(status=status), \
                    patch.object(release, 'run', side_effect=subprocess.CalledProcessError(status, ['rclone'])):
                if status in (3, 4):
                    self.assertIsNone(release.read_latest(remote))
                else:
                    with self.assertRaises(subprocess.CalledProcessError):
                        release.read_latest(remote)
        for stat, body in (({'IsDir': True, 'Size': 2}, '{}'),
                           ({'IsDir': False, 'Size': release.METADATA_LIMIT + 1}, '{}'),
                           ({'IsDir': False, 'Size': 3}, '{}')):
            with patch.object(release, 'run', side_effect=[json.dumps(stat), body]), self.assertRaises(ValueError):
                release.read_latest(remote)

    def test_public_object_exact_size_digest_and_streaming_bound(self):
        original = b'archive fixture'
        digest = hashlib.sha256(original).hexdigest()
        for data, ok in ((original, True), (original[:-1], False),
                         (original + b'x', False), (b'x' * len(original), False)):
            with self.subTest(data=data), patch.object(release.HTTPS_OPENER, 'open', return_value=io.BytesIO(data)):
                if ok:
                    release.public_identity('https://example.invalid/runtime', digest, len(original))
                else:
                    with self.assertRaises(ValueError):
                        release.public_identity('https://example.invalid/runtime', digest, len(original))

    def test_immutable_upload_never_syncs_or_mutates_prior_objects(self):
        args = SimpleNamespace(remote='s3:bucket')
        with patch.object(release, 'run') as run:
            release.upload(args, Path('/tmp/archive'), 'dem-runtime/1/amd64/hash.tar.zst')
            command = run.call_args.args[0]
            self.assertEqual(command[:2], ['rclone', 'copyto'])
            self.assertIn('--immutable', command)
            self.assertIn('--checksum', command)
            self.assertNotIn('sync', command)
            release.upload(args, Path('/tmp/release'), 'dem-runtime/latest/amd64.json', mutable=True)
            self.assertNotIn('--immutable', run.call_args.args[0])
            self.assertIn('Cache-Control: no-cache, no-transform', run.call_args.args[0])

    def test_inventory_rejects_changes_extras_links_and_path_escape(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifact = root / 'payload'
            artifact.write_bytes(b'abc')
            record = {'format': 1, 'architecture': 'amd64', 'version': '1.0',
                      'files': {'payload': {'sha256': release.sha256(artifact), 'bytes': 3}}}
            inventory = root / 'artifacts.json'
            inventory.write_text(json.dumps(record))
            release.validate_inventory(root, 'amd64')
            artifact.write_bytes(b'xyz')
            with self.assertRaises(ValueError):
                release.validate_inventory(root, 'amd64')
            artifact.write_bytes(b'abc')
            extra = root / 'unexpected'
            extra.write_bytes(b'abc')
            with self.assertRaises(ValueError):
                release.validate_inventory(root, 'amd64')
            extra.unlink()
            artifact.unlink()
            artifact.symlink_to('artifacts.json')
            with self.assertRaises(ValueError):
                release.validate_inventory(root, 'amd64')
            record['files'] = {'../outside': {'sha256': 'x', 'bytes': 3}}
            inventory.write_text(json.dumps(record))
            with self.assertRaises(ValueError):
                release.validate_inventory(root, 'amd64')

    def publication_fixture(self, root):
        proposed, directories = {}, {}
        for arch in release.ARCHITECTURES:
            directory = root / 'dem-runtime' / '2.0' / arch
            directory.mkdir(parents=True)
            (directory / 'release.json').write_bytes(b'signed fixture')
            data = {'url': 'https://example.invalid/' + arch, 'sha256': 'a' * 64, 'bytes': 12}
            proposed[arch] = candidate(arch)
            proposed[arch]['manifest']['archive'] = data
            proposed[arch]['sources'] = dict(data, url=data['url'] + '/sources')
            directories[arch] = directory
        return proposed, directories

    def test_promotion_verifies_all_platforms_before_first_pointer_write(self):
        with tempfile.TemporaryDirectory() as temporary:
            args = SimpleNamespace(publication=Path(temporary), public_base='https://example.invalid')
            proposed, directories = self.publication_fixture(args.publication)
            events = []
            with patch.object(release, 'publication', return_value=(proposed, directories)), \
                    patch.object(release, 'current_releases', return_value={}), \
                    patch.object(release, 'canonical_descriptors',
                                 return_value=({a: d / 'release.json' for a, d in directories.items()}, set())), \
                    patch.object(release, 'public_identity', side_effect=lambda *a: events.append(('verify', a[0]))), \
                    patch.object(release, 'upload', side_effect=lambda *a, **k: events.append(('upload', a[2]))):
                release.promote(args)
            self.assertEqual([e[0] for e in events], ['verify'] * 6 + ['upload', 'verify', 'upload', 'verify'])
            self.assertEqual(events[6][1], 'dem-runtime/latest/amd64.json')
            self.assertEqual(events[8][1], 'dem-runtime/latest/arm64.json')

    def test_missing_source_or_current_signature_failure_prevents_promotion(self):
        with tempfile.TemporaryDirectory() as temporary:
            args = SimpleNamespace(publication=Path(temporary), public_base='https://example.invalid')
            proposed, directories = self.publication_fixture(args.publication)
            for source_failure in (True, False):
                with self.subTest(source_failure=source_failure), \
                        patch.object(release, 'publication', return_value=(proposed, directories)), \
                        patch.object(release, 'current_releases',
                                     **({'return_value': {}} if source_failure else
                                        {'side_effect': ValueError('invalid signature')})), \
                        patch.object(release, 'canonical_descriptors',
                                     return_value=({a: d / 'release.json' for a, d in directories.items()}, set())), \
                        patch.object(release, 'public_identity', side_effect=[None, ValueError('missing sources')]), \
                        patch.object(release, 'upload') as upload:
                    with self.assertRaises(ValueError):
                        release.promote(args)
                    upload.assert_not_called()


@unittest.skipUnless(os.environ.get('DEM_RELEASE_TEST_TOOL') and shutil.which('gpg') and shutil.which('zstd'),
                     'requires compiled DEM_RELEASE_TEST_TOOL, gpg and zstd')
class SignedPublicationTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='dem-publisher-openpgp-')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        home = self.root / 'gnupg'
        home.mkdir(mode=0o700)
        environment = patch.dict(os.environ, {'GNUPGHOME': str(home)})
        environment.start()
        self.addCleanup(environment.stop)

        def gpg(*arguments):
            result = subprocess.run(['gpg', '--batch', '--pinentry-mode', 'loopback', *arguments],
                                    capture_output=True)
            if result.returncode:
                # Fixture setup failures must retain the actual GPG diagnostics/status.
                raise RuntimeError(f'gpg fixture failed with status {result.returncode}: '
                                   + result.stderr.decode(errors='replace'))
            return result.stdout

        gpg('--passphrase', '', '--quick-generate-key', 'DEM release test <dem@example.invalid>',
            'rsa2048', 'sign', '1d')
        listing = gpg('--with-colons', '--list-secret-keys').decode()
        self.fingerprint = next(line.split(':')[9] for line in listing.splitlines() if line.startswith('fpr:'))
        self.public = self.root / 'public.gpg'
        self.public.write_bytes(gpg('--export', self.fingerprint))

    def fixture(self):
        root = self.root
        tool = Path(os.environ['DEM_RELEASE_TEST_TOOL']).resolve()
        # Synthetic archive and sources test the real publisher/signature
        # boundary. They do not claim browser execution or source completeness.
        slot = b'/unprepared/loader' + b'\0' * 110
        entries = {'node': slot, 'browser': slot, 'lib/loader': b'loader',
                   'runner/run.mjs': b'export {}', 'dependencies/package.json': b'{}', 'fonts/font.ttf': b'font'}
        raw = io.BytesIO()
        with tarfile.open(fileobj=raw, mode='w') as archive:
            for name, data in entries.items():
                member = tarfile.TarInfo(name)
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))
        data = subprocess.run(['zstd', '-19', '--single-thread', '--zstd=wlog=23', '--check', '-c'],
                              input=raw.getvalue(), capture_output=True, check=True).stdout
        digest = hashlib.sha256(data).hexdigest()
        source_data = b'synthetic source fixture'
        source_hash = hashlib.sha256(source_data).hexdigest()
        for arch in release.ARCHITECTURES:
            directory = root / 'artifacts' / ('dem-runtime-' + arch)
            directory.mkdir(parents=True)
            manifest = {'format': 1, 'contract': 1, 'version': '1.2.3', 'platform': 'linux/' + arch,
                        'archive': {'sha256': digest, 'bytes': len(data), 'tar_bytes': len(raw.getvalue()),
                                    'expanded_bytes': sum(map(len, entries.values())), 'entries': len(entries)},
                        'layout': {'node': 'node', 'browser': 'browser', 'loader': 'lib/loader',
                                   'entrypoint': 'runner/run.mjs', 'dependencies': 'dependencies',
                                   'fonts': 'fonts'},
                        'bindings': [{'path': p, 'offset': 0, 'expected': base64.b64encode(slot).decode()}
                                     for p in ('node', 'browser')]}
            (directory / (digest + '.tar.zst')).write_bytes(data)
            source_name = source_hash + '.sources.tar.zst'
            (directory / source_name).write_bytes(source_data)
            release.write_json(directory / 'manifest.json', manifest)
            release.write_json(directory / 'source-archive.json', {'format': 1, 'runtime_archive': digest,
                               'file': source_name, 'sha256': source_hash, 'bytes': len(source_data)})
            for name in ('provenance.json', 'sbom.cdx.json', 'sources-required.json'):
                (directory / name).write_text('{}')
            release.write_json(directory / 'artifacts.json', {'format': 1, 'architecture': arch, 'version': '1.2.3',
                               'files': {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                                         for p in directory.iterdir()}})
        args = SimpleNamespace(output=root / 'publication', artifacts=root / 'artifacts',
                               signing_key=self.fingerprint, public_key=self.public, release_tool=tool,
                               public_base='https://example.invalid', sequence=17, publication=root / 'publication',
                               remote='s3:bucket')
        return args, digest, source_hash

    def test_two_platform_staging_authenticates_every_field_and_rejects_tampering(self):
        args, digest, source_hash = self.fixture()
        release.stage(args)
        published, directories = release.publication(args)
        self.assertEqual(set(published), set(release.ARCHITECTURES))
        for arch, item in published.items():
            self.assertEqual(item['sequence'], 17)
            self.assertEqual(item['manifest']['archive']['sha256'], digest)
            self.assertEqual(item['sources']['sha256'], source_hash)
            self.assertIn('/1.2.3/' + arch + '/', item['sources']['url'])
        path = directories['arm64'] / 'release.json'
        envelope = json.loads(path.read_bytes())
        payload = json.loads(base64.b64decode(envelope['payload']))
        payload['sequence'] = 18
        envelope['payload'] = base64.b64encode(json.dumps(payload).encode()).decode()
        path.write_text(json.dumps(envelope))
        with self.assertRaises(subprocess.CalledProcessError):
            release.publication(args)

    def test_retry_reuses_authenticated_immutable_bytes_and_promotes_them(self):
        args, _, _ = self.fixture()
        release.stage(args)
        first = {a: (args.output / 'dem-runtime' / '1.2.3' / a / 'release.json').read_bytes()
                 for a in release.ARCHITECTURES}
        storage, writes = {}, []

        def upload(arguments, path, relative, *, mutable=False):
            body = path.read_bytes()
            if not mutable and relative in storage and storage[relative] != body:
                raise ValueError('immutable object differs')
            storage[relative] = body
            writes.append(relative)

        def identity(url, digest, size):
            body = storage[url.removeprefix(args.public_base + '/')]
            self.assertEqual((hashlib.sha256(body).hexdigest(), len(body)), (digest, size))

        with patch.object(release, 'read_latest', side_effect=lambda remote: storage.get(
                remote.removeprefix(args.remote + '/'))), \
                patch.object(release, 'upload', side_effect=upload), \
                patch.object(release, 'public_identity', side_effect=identity):
            release.publish(args)
            time.sleep(1.1)  # Real signing timestamps differ; no synthetic signing time.
            args.output = args.publication = self.root / 'retry'
            release.stage(args)
            second = {a: (args.output / 'dem-runtime' / '1.2.3' / a / 'release.json').read_bytes()
                      for a in release.ARCHITECTURES}
            for arch in release.ARCHITECTURES:
                self.assertNotEqual(first[arch], second[arch])
            writes.clear()
            release.publish(args)
            self.assertFalse(any(p.endswith('/release.json') for p in writes))
            release.promote(args)
            for arch in release.ARCHITECTURES:
                self.assertEqual(storage['dem-runtime/latest/' + arch + '.json'], first[arch])
                self.assertEqual((args.output / 'dem-runtime' / '1.2.3' / arch / 'release.json').read_bytes(),
                                 second[arch])

            # A same-version retry with different signed content fails before writes.
            args.output = args.publication = self.root / 'changed'
            args.sequence = 18
            release.stage(args)
            writes.clear()
            with patch.object(release, 'current_releases', return_value={}), self.assertRaisesRegex(
                    ValueError, 'immutable release payload differs'):
                release.publish(args)
            self.assertEqual(writes, [])

            # An untrusted immutable descriptor also fails before writes/promotion.
            args.output = args.publication = self.root / 'retry'
            args.sequence = 17
            storage['dem-runtime/1.2.3/arm64/release.json'] = b'{}'
            for operation in (release.publish, release.promote):
                with self.assertRaises(subprocess.CalledProcessError):
                    operation(args)
                self.assertEqual(writes, [])
            storage.pop('dem-runtime/1.2.3/arm64/release.json')
            with self.assertRaisesRegex(ValueError, 'immutable release descriptor is missing'):
                release.promote(args)
            self.assertEqual(writes, [])


if __name__ == '__main__':
    unittest.main()
