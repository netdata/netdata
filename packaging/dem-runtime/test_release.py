# SPDX-License-Identifier: GPL-3.0-or-later
"""Publisher control flow and real Go/GPG signature integration regressions."""
import base64
from contextlib import redirect_stderr
import hashlib
import io
import json
from pathlib import Path
import os
import runpy
import shutil
import subprocess
import sys
import tempfile
import tarfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from urllib.error import HTTPError
from urllib.request import Request

import release


def candidate(arch, version='v2.12.0-2-nightly', channel='nightly', source='a' * 40):
    return {'format': 2, 'agent': {'version': version, 'channel': channel, 'source': source},
            'manifest': {'version': version, 'platform': 'linux/' + arch}}


class PublicationTests(unittest.TestCase):
    def test_cli_reports_http_status_without_untrusted_details(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / 'public.gpg'
            argv = [release.__file__, 'fetch-public-key', '--output', str(output)]
            for status in (403, 503):
                with self.subTest(status=status):
                    error = HTTPError('https://example.invalid/?token=secret', status,
                                      'private response reason', {'Secret': 'private header'}, None)
                    stderr = io.StringIO()
                    with patch.object(release.HTTPS_OPENER, 'open', side_effect=error), \
                            patch.object(sys, 'argv', argv), \
                            redirect_stderr(stderr), self.assertRaises(SystemExit) as raised:
                        runpy.run_path(release.__file__, run_name='__main__')
                    self.assertEqual(raised.exception.code, 1)
                    self.assertEqual(stderr.getvalue().strip(), f'runtime release operation failed: HTTP {status}')
                    self.assertFalse(output.exists())

    def test_cli_reports_sanitized_failure_without_secondary_exception(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run([
                sys.executable, str(Path(release.__file__)), 'publish',
                '--publication', str(Path(temporary) / 'missing'),
                '--release-tool', 'unused', '--public-key', 'unused',
                '--public-base', 'https://example.invalid', '--remote', 'unused:prefix',
            ], capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stderr.strip(), 'runtime release operation failed: FileNotFoundError')

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
                    opened.assert_called_once()
                    request = opened.call_args.args[0]
                    self.assertIsInstance(request, Request)
                    self.assertEqual(request.full_url, release.PUBLIC_KEY_URL)
                    self.assertEqual(request.get_header('User-agent'), 'Netdata-DEM-release/1')
                    self.assertEqual(opened.call_args.kwargs, {'timeout': release.DOWNLOAD_TIMEOUT_SECONDS})
            with patch.object(release.HTTPS_OPENER, 'open', side_effect=OSError('unavailable')):
                with self.assertRaises(OSError):
                    release.fetch_public_key(args)

    def test_immutable_metadata_reads_storage_and_fails_closed(self):
        remote = 's3:bucket/dem-runtime/nightly/v2.12.0-2-nightly/amd64/release.json'
        entry = {'Path': 'release.json', 'IsDir': False, 'Size': 2}
        with patch.object(release, 'run', side_effect=[json.dumps([entry]), '{}']) as run:
            self.assertEqual(release.read_stored_descriptor(remote), b'{}')
            self.assertIn('--count', run.call_args_list[1].args[0])
        with patch.object(release, 'run', return_value='[]') as run:
            self.assertIsNone(release.read_stored_descriptor(remote))
            self.assertEqual(run.call_count, 1)
        for status in (3, 4, 1, 5, 7):
            with self.subTest(status=status), \
                    patch.object(release, 'run', side_effect=subprocess.CalledProcessError(status, ['rclone'])):
                with self.assertRaises(subprocess.CalledProcessError):
                    release.read_stored_descriptor(remote)
        for stat, body in ((dict(entry, IsDir=True), '{}'),
                           (dict(entry, Size=release.METADATA_LIMIT + 1), '{}'),
                           (dict(entry, Size=3), '{}')):
            with patch.object(release, 'run', side_effect=[json.dumps([stat]), body]), self.assertRaises(ValueError):
                release.read_stored_descriptor(remote)

    def test_public_object_exact_size_digest_and_streaming_bound(self):
        original = b'archive fixture'
        digest = hashlib.sha256(original).hexdigest()
        for data, ok in ((original, True), (original[:-1], False),
                         (original + b'x', False), (b'x' * len(original), False)):
            with self.subTest(data=data), \
                    patch.object(release.HTTPS_OPENER, 'open', return_value=io.BytesIO(data)) as opened:
                if ok:
                    release.public_identity('https://example.invalid/runtime', digest, len(original))
                else:
                    with self.assertRaises(ValueError):
                        release.public_identity('https://example.invalid/runtime', digest, len(original))
                request = opened.call_args.args[0]
                self.assertIsInstance(request, Request)
                self.assertEqual(request.full_url, 'https://example.invalid/runtime')
                self.assertEqual(request.get_header('User-agent'), 'Netdata-DEM-release/1')

    def test_immutable_upload_never_syncs_or_mutates_prior_objects(self):
        args = SimpleNamespace(remote='s3:bucket')
        with patch.object(release, 'run') as run:
            release.upload(args, Path('/tmp/archive'), 'dem-runtime/stable/v1.0.0/amd64/hash.tar.zst')
            command = run.call_args.args[0]
            self.assertEqual(command[:2], ['rclone', 'copyto'])
            self.assertIn('--immutable', command)
            self.assertIn('--checksum', command)
            self.assertNotIn('sync', command)

    def test_inventory_rejects_changes_extras_links_and_path_escape(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            artifact = root / 'payload'
            artifact.write_bytes(b'abc')
            agent = candidate('amd64')['agent']
            (root / 'release-info').write_text(''.join(k + '=' +
                                                       agent[k] + '\n' for k in ('version', 'channel', 'source')))
            release.write_json(root / 'manifest.json', candidate('amd64')['manifest'])
            release.write_json(root / 'provenance.json', {'agent': agent, 'platform': 'linux/amd64'})
            record = {'format': 2, 'architecture': 'amd64', 'agent': agent,
                      'files': {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                                for p in root.iterdir()}}
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
            directory = root / 'dem-runtime' / 'nightly' / 'v2.12.0-2-nightly' / arch
            directory.mkdir(parents=True)
            (directory / 'release.json').write_bytes(b'signed fixture')
            data = {'url': 'https://example.invalid/' + arch, 'sha256': 'a' * 64, 'bytes': 12}
            proposed[arch] = candidate(arch)
            proposed[arch]['manifest']['archive'] = data
            proposed[arch]['sources'] = dict(data, url=data['url'] + '/sources')
            directories[arch] = directory
        return proposed, directories


class PairedAssemblyTests(unittest.TestCase):
    def artifacts(self, root, arch, agent):
        directory = root / ('dem-runtime-' + arch)
        directory.mkdir(parents=True)
        archive = directory / (hashlib.sha256(b'runtime').hexdigest() + '.tar.zst')
        archive.write_bytes(b'runtime')
        source = directory / (hashlib.sha256(b'sources').hexdigest() + '.sources.tar.zst')
        source.write_bytes(b'sources')
        manifest = candidate(arch, **{'version': agent['version'], 'channel': agent['channel'],
                                      'source': agent['source']})['manifest']
        manifest.update({'format': 1, 'contract': 2,
                         'archive': {'sha256': release.sha256(archive), 'bytes': archive.stat().st_size}})
        release.write_json(directory / 'manifest.json', manifest)
        release.write_json(directory / 'source-archive.json', {'format': 1, 'runtime_archive': release.sha256(archive),
                           'sha256': release.sha256(source), 'bytes': source.stat().st_size, 'file': source.name})
        release.write_json(directory / 'provenance.json', {'agent': agent, 'platform': 'linux/' + arch,
                           'inputs': {'source_date_epoch': 12345}})
        for name in ('sbom.cdx.json', 'sources-required.json'):
            release.write_json(directory / name, {})
        (directory / 'release-info').write_text(''.join(k + '=' +
                                                        agent[k] + '\n' for k in ('version', 'channel', 'source')))
        (directory / 'native-payload.tar.zst').write_bytes(b'native transport only')
        release.write_json(directory / 'artifacts.json', {'format': 2, 'agent': agent, 'architecture': arch,
                           'files': {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                                     for p in directory.iterdir()}})
        return directory

    def test_stage_uses_exact_agent_and_versioned_paths_for_both_channels(self):
        for channel, version in [('stable', 'v2.12.0'), ('nightly', 'v2.12.0-607-nightly')]:
            with self.subTest(channel=channel), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                agent = {'version': version, 'channel': channel, 'source': 'a' * 40}
                for arch in release.ARCHITECTURES:
                    self.artifacts(root / 'artifacts', arch, agent)
                args = SimpleNamespace(artifacts=root / 'artifacts', output=root / 'publication',
                                       release_tool=Path('/test-tool'), signing_key='fixture',
                                       public_key=root / 'public', public_base='https://artifacts.netdata.cloud')
                calls = []

                def sign(command):
                    calls.append(command)
                    self.assertEqual(command[1], 'sign')
                    self.assertNotIn('--sequence', command)
                    m = json.loads(Path(command[command.index('--manifest') + 1]).read_text())
                    m['archive']['url'] = command[command.index('--archive-url') + 1]
                    source = Path(command[command.index('--sources') + 1])
                    item = {'format': 2, 'agent': agent, 'manifest': m,
                            'sources': {'url': command[command.index('--sources-url') + 1],
                                        'sha256': release.sha256(source), 'bytes': source.stat().st_size}}
                    release.write_json(Path(command[command.index('--output') + 1]), item)

                with patch.object(release, 'run', side_effect=sign), \
                        patch.object(release, 'verified_release',
                                     side_effect=lambda tool, key, path: json.loads(path.read_text())):
                    release.stage(args)
                    args.publication = args.output
                    proposed, directories = release.publication(args)
                self.assertEqual(len(calls), 2)
                for arch, directory in directories.items():
                    self.assertEqual(directory, args.output / 'dem-runtime' / channel / version / arch)
                    self.assertNotIn('native-payload.tar.zst', {p.name for p in directory.iterdir()})
                    self.assertEqual(proposed[arch]['agent'], agent)
                    self.assertIn('--agent-source', calls[release.ARCHITECTURES.index(arch)])

    def test_cross_arch_source_mismatch_refuses_signing(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for arch, source in [('amd64', 'a' * 40), ('arm64', 'b' * 40)]:
                self.artifacts(root / 'artifacts', arch,
                               {'version': 'v2.12.0', 'channel': 'stable', 'source': source})
            args = SimpleNamespace(artifacts=root / 'artifacts', output=root / 'publication')
            with patch.object(release, 'run', side_effect=AssertionError('signing started')):
                with self.assertRaisesRegex(ValueError, 'same Agent release'):
                    release.stage(args)

    def test_payloads_precede_descriptors_and_retry_completes_partial_publish(self):
        with tempfile.TemporaryDirectory() as temporary:
            args = SimpleNamespace(publication=Path(temporary), public_base='https://example.invalid')
            proposed, directories = PublicationTests().publication_fixture(args.publication)
            for directory in directories.values():
                (directory / 'runtime.tar.zst').write_bytes(b'runtime')
                (directory / 'sources.tar.zst').write_bytes(b'sources')
            stored, events = set(), []

            def canonical(*arguments, **kwargs):
                return {a: d / 'release.json' for a, d in directories.items()}, set(stored)

            def upload(arguments, path, relative):
                events.append(relative)
                if relative.endswith('arm64/release.json') and 'amd64' in stored:
                    raise RuntimeError('second architecture failed')
                if relative.endswith('/release.json'):
                    stored.add(relative.split('/')[-2])
            with patch.object(release, 'publication', return_value=(proposed, directories)), \
                    patch.object(release, 'canonical_descriptors', side_effect=canonical), \
                    patch.object(release, 'public_identity'), patch.object(release, 'upload', side_effect=upload):
                with self.assertRaisesRegex(RuntimeError, 'second architecture'):
                    release.publish(args)
                self.assertEqual(stored, {'amd64'})
                first_descriptor = next(i for i, name in enumerate(events) if name.endswith('/release.json'))
                self.assertEqual(first_descriptor, 4, 'both runtime/source pairs precede every descriptor')
                events.clear()
                with patch.object(release, 'upload', side_effect=lambda args, path, relative: events.append(relative)):
                    release.publish(args)
                self.assertEqual([p for p in events if p.endswith('/release.json')],
                                 ['dem-runtime/nightly/v2.12.0-2-nightly/arm64/release.json'])

    def test_failed_public_source_readback_prevents_all_descriptors(self):
        with tempfile.TemporaryDirectory() as temporary:
            args = SimpleNamespace(publication=Path(temporary), public_base='https://example.invalid')
            proposed, directories = PublicationTests().publication_fixture(args.publication)
            for directory in directories.values():
                (directory / 'runtime.tar.zst').write_bytes(b'runtime')
                (directory / 'sources.tar.zst').write_bytes(b'sources')
            uploaded = []

            def public_identity(url, digest, size):
                if url.endswith('arm64/sources.tar.zst'):
                    raise ValueError('public source bytes differ')
            with patch.object(release, 'publication', return_value=(proposed, directories)), \
                    patch.object(release, 'canonical_descriptors', return_value=(
                        {arch: directory / 'release.json' for arch, directory in directories.items()}, set())), \
                    patch.object(release, 'upload', side_effect=lambda args, path, relative:
                                 uploaded.append(relative)), \
                    patch.object(release, 'public_identity', side_effect=public_identity):
                with self.assertRaisesRegex(ValueError, 'public source bytes differ'):
                    release.publish(args)
            self.assertTrue(uploaded)
            self.assertFalse(any(path.endswith('/release.json') for path in uploaded))

    @unittest.skipUnless(os.environ.get('DEM_RELEASE_TEST_TOOL') and shutil.which('zstd'),
                         'requires compiled DEM_RELEASE_TEST_TOOL and zstd')
    def test_assemble_transports_existing_go_native_stage_to_normal_agent_packaging(self):
        import ci
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            agent = {'version': 'v2.12.0-607-nightly', 'channel': 'nightly', 'source': 'a' * 40}
            builder = self.artifacts(root / 'builder', 'amd64', agent)
            slot = b'/unprepared/loader' + b'\0' * 110
            entries = {'node': slot, 'browser': slot, 'lib/loader': b'loader',
                       'runner/run.mjs': b'export {}', 'dependencies/package.json': b'{}',
                       'fonts/font.ttf': b'font', 'release-info': (builder / 'release-info').read_bytes()}
            raw = io.BytesIO()
            with tarfile.open(fileobj=raw, mode='w') as archive:
                for name, data in entries.items():
                    member = tarfile.TarInfo(name)
                    member.size = len(data)
                    member.mode = 0o755 if name in ('node', 'browser', 'lib/loader') else 0o644
                    archive.addfile(member, io.BytesIO(data))
            compressed = subprocess.run(['zstd', '-19', '--single-thread', '--zstd=wlog=23', '--check', '-c'],
                                        input=raw.getvalue(), capture_output=True, check=True).stdout
            digest = hashlib.sha256(compressed).hexdigest()
            (builder / (digest + '.tar.zst')).write_bytes(compressed)
            manifest = {'format': 1, 'contract': 2, 'version': agent['version'], 'platform': 'linux/amd64',
                        'archive': {'url': '', 'sha256': digest, 'bytes': len(compressed),
                                    'tar_bytes': len(raw.getvalue()),
                                    'expanded_bytes': sum(map(len, entries.values())), 'entries': len(entries)},
                        'layout': {'node': 'node', 'browser': 'browser', 'loader': 'lib/loader',
                                   'entrypoint': 'runner/run.mjs', 'dependencies': 'dependencies', 'fonts': 'fonts'},
                        'bindings': [{'path': p, 'offset': 0, 'expected': base64.b64encode(slot).decode()}
                                     for p in ('node', 'browser')]}
            release.write_json(builder / 'manifest.json', manifest)
            source = json.loads((builder / 'source-archive.json').read_text())
            source['runtime_archive'] = digest
            release.write_json(builder / 'source-archive.json', source)
            args = SimpleNamespace(runtime_output=builder, source_archive=builder, arch='amd64',
                                   release_tool=Path(os.environ['DEM_RELEASE_TEST_TOOL']).resolve(),
                                   output=root / 'assembled')
            release.assemble(args)
            inventory = release.validate_inventory(args.output, 'amd64')
            self.assertEqual(inventory['agent'], agent)
            self.assertFalse(any(p.suffix in ('.deb', '.rpm') for p in args.output.iterdir()))
            receipt = root / 'publication-receipt'
            receipt.mkdir()
            shutil.copyfile(builder / 'release-info', receipt / 'release-info')
            shutil.copyfile(args.output / 'artifacts.json', receipt / 'amd64.json')
            other = self.artifacts(root / 'other-builder', 'arm64', agent)
            shutil.copyfile(other / 'artifacts.json', receipt / 'arm64.json')
            output = root / 'admitted'
            ci.prepare_native(SimpleNamespace(artifacts=args.output, arch='amd64',
                              release_info=builder / 'release-info', output=output, receipt=receipt))
            self.assertEqual((output / 'release-info').read_bytes(), (builder / 'release-info').read_bytes())
            self.assertTrue((output / 'node').read_bytes().startswith(b'/usr/lib/netdata/dem-runtime/lib/loader\0'))
            self.assertEqual(json.loads((output / 'native.json').read_text())['manifest'], manifest)


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
                raise RuntimeError(f'gpg fixture failed with status {result.returncode}: ' +
                                   result.stderr.decode(errors='replace'))
            return result.stdout

        gpg('--passphrase', '', '--quick-generate-key', 'DEM release test <dem@example.invalid>',
            'rsa2048', 'sign', '1d')
        listing = gpg('--with-colons', '--list-secret-keys').decode()
        self.fingerprint = next(line.split(':')[9] for line in listing.splitlines() if line.startswith('fpr:'))
        self.public = self.root / 'public.gpg'
        self.public.write_bytes(gpg('--export', self.fingerprint))

    def fixture(self, channel='stable', version='v1.2.3'):
        root = self.root
        agent = {'version': version, 'channel': channel, 'source': 'a' * 40}
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
            directory = root / ('artifacts-' + channel) / ('dem-runtime-' + arch)
            directory.mkdir(parents=True)
            manifest = {'format': 1, 'contract': 2, 'version': version, 'platform': 'linux/' + arch,
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
            for name in ('sbom.cdx.json', 'sources-required.json'):
                (directory / name).write_text('{}')
            release.write_json(directory / 'provenance.json', {'agent': agent, 'platform': 'linux/' + arch,
                               'inputs': {'source_date_epoch': 12345}})
            (directory / 'release-info').write_text(''.join(k + '=' +
                                                            agent[k] + '\n' for k in ('version', 'channel', 'source')))
            release.write_json(directory / 'artifacts.json', {'format': 2, 'architecture': arch, 'agent': agent,
                               'files': {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                                         for p in directory.iterdir()}})
        args = SimpleNamespace(output=root / ('publication-' + channel), artifacts=root / ('artifacts-' + channel),
                               signing_key=self.fingerprint, public_key=self.public, release_tool=tool,
                               public_base='https://example.invalid', publication=root / ('publication-' + channel),
                               remote='s3:bucket')
        return args, digest, source_hash

    def test_two_platform_staging_authenticates_every_field_and_rejects_tampering(self):
        args, digest, source_hash = self.fixture()
        release.stage(args)
        published, directories = release.publication(args)
        self.assertEqual(set(published), set(release.ARCHITECTURES))
        for arch, item in published.items():
            self.assertEqual(item['format'], 2)
            self.assertNotIn('sequence', item)
            self.assertEqual(item['agent']['version'], 'v1.2.3')
            self.assertEqual(item['manifest']['archive']['sha256'], digest)
            self.assertEqual(item['sources']['sha256'], source_hash)
            self.assertIn('/stable/v1.2.3/' + arch + '/', item['sources']['url'])
        path = directories['arm64'] / 'release.json'
        envelope = json.loads(path.read_bytes())
        payload = json.loads(base64.b64decode(envelope['payload']))
        payload['agent']['source'] = 'b' * 40
        envelope['payload'] = base64.b64encode(json.dumps(payload).encode()).decode()
        path.write_text(json.dumps(envelope))
        with self.assertRaises(subprocess.CalledProcessError):
            release.publication(args)

    def test_retry_reuses_authenticated_immutable_bytes(self):
        args, _, _ = self.fixture()
        release.stage(args)
        first = {a: (args.output / 'dem-runtime' / 'stable' / 'v1.2.3' / a / 'release.json').read_bytes()
                 for a in release.ARCHITECTURES}
        storage, writes = {}, []

        def upload(arguments, path, relative):
            body = path.read_bytes()
            if relative in storage and storage[relative] != body:
                raise ValueError('immutable object differs')
            storage[relative] = body
            writes.append(relative)

        def identity(url, digest, size):
            body = storage[url.removeprefix(args.public_base + '/')]
            self.assertEqual((hashlib.sha256(body).hexdigest(), len(body)), (digest, size))

        with patch.object(release, 'read_stored_descriptor', side_effect=lambda remote: storage.get(
                remote.removeprefix(args.remote + '/'))), \
                patch.object(release, 'upload', side_effect=upload), \
                patch.object(release, 'public_identity', side_effect=identity):
            release.publish(args)
            time.sleep(1.1)  # Real signing timestamps differ; no synthetic signing time.
            args.output = args.publication = self.root / 'retry'
            release.stage(args)
            second = {a: (args.output / 'dem-runtime' / 'stable' / 'v1.2.3' / a / 'release.json').read_bytes()
                      for a in release.ARCHITECTURES}
            for arch in release.ARCHITECTURES:
                self.assertNotEqual(first[arch], second[arch])
            writes.clear()
            release.publish(args)
            self.assertFalse(any(p.endswith('/release.json') for p in writes))
            for arch in release.ARCHITECTURES:
                self.assertEqual(storage['dem-runtime/stable/v1.2.3/' + arch + '/release.json'], first[arch])
                self.assertEqual(
                    (args.output / 'dem-runtime' / 'stable' / 'v1.2.3' /
                     arch / 'release.json').read_bytes(), second[arch])

            # A same-version retry with different signed content fails before writes.
            args.output = args.publication = self.root / 'changed'
            for arch in release.ARCHITECTURES:
                directory = args.artifacts / ('dem-runtime-' + arch)
                provenance = json.loads((directory / 'provenance.json').read_text())
                provenance['agent']['source'] = 'b' * 40
                release.write_json(directory / 'provenance.json', provenance)
                agent = provenance['agent']
                (directory / 'release-info').write_text(
                    ''.join(k + '=' + agent[k] + '\n' for k in ('version', 'channel', 'source')))
                inventory = json.loads((directory / 'artifacts.json').read_text())
                inventory['agent'] = agent
                inventory['files'] = {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                                      for p in directory.iterdir() if p.name != 'artifacts.json'}
                release.write_json(directory / 'artifacts.json', inventory)
            release.stage(args)
            writes.clear()
            with self.assertRaisesRegex(
                    ValueError, 'immutable release payload differs'):
                release.publish(args)
            self.assertEqual(writes, [])

            # An untrusted immutable descriptor also fails before writes.
            args.output = args.publication = self.root / 'retry'
            storage['dem-runtime/stable/v1.2.3/arm64/release.json'] = b'{}'
            with self.assertRaises(subprocess.CalledProcessError):
                release.publish(args)
            self.assertEqual(writes, [])


if __name__ == '__main__':
    unittest.main()
