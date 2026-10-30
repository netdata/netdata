# SPDX-License-Identifier: GPL-3.0-or-later
"""Publisher control-flow regressions; signing/verification belongs to Go tests."""
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
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import release


def candidate(arch, sequence=2, version='2.0'):
    return {'sequence': sequence, 'manifest': {'version': version, 'platform': 'linux/' + arch}}


class PublicationTests(unittest.TestCase):
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
                        patch.object(release, 'public_identity', side_effect=[None, ValueError('missing sources')]), \
                        patch.object(release, 'upload') as upload:
                    with self.assertRaises(ValueError):
                        release.promote(args)
                    upload.assert_not_called()


@unittest.skipUnless(os.environ.get('DEM_RELEASE_TEST_TOOL') and shutil.which('openssl'),
                     'requires compiled DEM_RELEASE_TEST_TOOL and openssl')
class SignedPublicationTests(unittest.TestCase):
    def test_two_platform_staging_authenticates_every_field_and_rejects_tampering(self):
        tool = Path(os.environ['DEM_RELEASE_TEST_TOOL']).resolve()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            private, public = root / 'private.pem', root / 'public.pem'
            subprocess.run(['openssl', 'genpkey', '-algorithm', 'ED25519', '-out', str(private)], check=True,
                           capture_output=True)
            subprocess.run(['openssl', 'pkey', '-in', str(private), '-pubout', '-out', str(public)], check=True,
                           capture_output=True)
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
                                   private_key=private, public_key=public, release_tool=tool,
                                   public_base='https://example.invalid', sequence=17, publication=root / 'publication')
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


if __name__ == '__main__':
    unittest.main()
