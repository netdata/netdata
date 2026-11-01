# SPDX-License-Identifier: GPL-3.0-or-later
from concurrent.futures import ThreadPoolExecutor
import hashlib
import io
import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch

import locked
import sources


class LockedInputTests(unittest.TestCase):
    def response(self, data, url='https://example.invalid/archive'):
        response = io.BytesIO(data)
        response.url = url
        return response

    def test_download_requires_exact_hash_size_and_https(self):
        data = b'exact archived bytes'
        record = {'name': 'input.deb', 'url': 'https://example.invalid/input.deb',
                  'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data)}
        cases = {'valid': (data, record, None), 'corrupt': (b'wrong', record, ValueError),
                 'wrong_size': (data, dict(record, bytes=len(data) + 1), ValueError),
                 'credentials': (data, dict(record, url='https://user:secret@example.invalid/input.deb'), ValueError),
                 'empty_user': (data, dict(record, url='https://@example.invalid/input.deb'), ValueError),
                 'empty_password': (data, dict(record, url='https://:@example.invalid/input.deb'), ValueError),
                 'http': (data, dict(record, url='http://example.invalid/input.deb'), ValueError),
                 'unsafe_name': (data, dict(record, name='../outside'), ValueError)}
        for name, (payload, selected, error) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                with patch('locked.HTTPS_OPENER.open', return_value=self.response(payload)):
                    if error:
                        with self.assertRaises(error):
                            locked.download(selected, root)
                        self.assertEqual(list(root.iterdir()), [],
                                         'failed acquisition must clean its private temporary files')
                    else:
                        self.assertEqual(locked.download(selected, root).read_bytes(), data)
                if not error:
                    # Cache reuse still validates bytes and must not perform I/O.
                    with patch('locked.HTTPS_OPENER.open', side_effect=AssertionError('network')):
                        self.assertEqual(locked.download(selected, root).read_bytes(), data)
                    (root / 'input.deb').write_bytes(b'corrupt cache')
                    with self.assertRaisesRegex(ValueError, 'cached input'):
                        locked.download(selected, root)
        with tempfile.TemporaryDirectory() as directory, \
                patch('locked.HTTPS_OPENER.open', return_value=self.response(data, 'http://example.invalid/archive')):
            with self.assertRaisesRegex(ValueError, 'redirected'):
                locked.download(record, Path(directory))

    def test_concurrent_downloads_share_only_verified_cache_entry(self):
        data = b'shared all-architecture package bytes'
        record = {'name': 'shared.deb', 'url': 'https://example.invalid/shared.deb',
                  'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data)}
        opened = threading.Barrier(2)

        class OverlappingResponse(io.BytesIO):
            url = record['url']

            def read(self, size=-1):
                if self.tell() == 0:
                    # Both calls have opened their output before either writes.
                    opened.wait(timeout=5)
                return super().read(size)

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch('locked.HTTPS_OPENER.open', side_effect=lambda url, **kwargs: OverlappingResponse(data)):
                with ThreadPoolExecutor(max_workers=2) as workers:
                    futures = [workers.submit(locked.download, record, root) for _ in range(2)]
                    paths = [future.result(timeout=10) for future in futures]
            self.assertEqual(paths, [root / 'shared.deb'] * 2)
            self.assertEqual(paths[0].read_bytes(), data)
            self.assertEqual(sorted(p.name for p in root.iterdir()), ['shared.deb'])

    def test_exact_inventory_including_binary_source_versions(self):
        packages = [{'name': 'libexample1', 'version': '1:2.0-3+b2', 'architecture': 'arm64',
                     'source': 'example', 'source_version': '1:2.0-3'}]
        with patch('locked.inventory', return_value=packages):
            locked.check_inventory({'installed': packages})
            with self.assertRaisesRegex(ValueError, 'differ'):
                locked.check_inventory({'installed': [dict(packages[0], version='1:2.0-4')]})
        self.assertEqual(locked.source_key(packages[0]['source'], packages[0]['source_version']), 'example@1:2.0-3')

    def test_source_descriptor_preserves_all_companions(self):
        companions = [{'name': name, 'sha256': hashlib.sha256(name.encode()).hexdigest(), 'bytes': len(name)}
                      for name in ('example_2.0.orig.tar.xz', 'example_2.0.orig-pre-gen.tar.xz',
                                   'example_2.0-3.debian.tar.xz')]
        body = ('-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA512\n\nFormat: 3.0 (quilt)\n'
                'Source: example\nVersion: 1:2.0-3\nChecksums-Sha256:\n')
        body += ''.join(f' {p["sha256"]} {p["bytes"]} {p["name"]}\n' for p in companions)
        body += '\n-----BEGIN PGP SIGNATURE-----\nsynthetic\n'
        record = {'name': 'example', 'version': '1:2.0-3', 'files': companions + [{'name': 'example_2.0-3.dsc'}]}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'example.dsc'
            path.write_text(body)
            sources.verify_descriptor(path, record)
            with self.assertRaisesRegex(ValueError, 'companions'):
                sources.verify_descriptor(path, dict(record, files=companions[1:]))
            with self.assertRaisesRegex(ValueError, 'identity'):
                sources.verify_descriptor(path, dict(record, version='1:2.0-3+b2'))

    def test_committed_locks_reconstruct_complete_inventory(self):
        for architecture in ('arm64', 'amd64'):
            with self.subTest(architecture=architecture):
                path = Path(__file__).parent / 'locks' / (architecture + '.json')
                lock = json.loads(path.read_text())
                state = {(p['name'], p['architecture']): p for p in lock['base']}
                installed = {(p['name'], p['architecture']): p for p in lock['installed']}
                self.assertEqual(len(installed), len(lock['installed']))
                self.assertEqual(lock['architecture'], architecture)
                for package in lock['binary_packages']:
                    key = package['name'], package['architecture']
                    state[key] = {field: package[field]
                                  for field in ('name', 'architecture', 'version', 'source', 'source_version')}
                    self.assertRegex(package['sha256'], '^[a-f0-9]{64}$')
                    self.assertGreater(package['bytes'], 0)
                    self.assertTrue(package['url'].startswith('https://'))
                    self.assertEqual(Path(package['url']).name, package['file'])
                self.assertEqual(state, installed)
                for name in ('chromium', 'chromium-common'):
                    package = installed[(name, architecture)]
                    self.assertEqual(package['version'], lock['chromium']['package_version'])
                    self.assertEqual(package['source'], 'chromium')
                for key, source in lock['sources'].items():
                    self.assertEqual(key, locked.source_key(source['name'], source['version']))
                    self.assertEqual(sum(p['name'].endswith('.dsc') for p in source['files']), 1)
                    self.assertGreater(len(source['files']), 1)
                    for item in source['files']:
                        self.assertRegex(item['sha256'], '^[a-f0-9]{64}$')
                        self.assertGreater(item['bytes'], 0)
                        self.assertEqual(Path(item['name']).name, item['name'])

    def test_source_lock_checksums_reject_unsafe_names(self):
        with self.assertRaisesRegex(ValueError, 'checksum'):
            locked.checksums({'Checksums-Sha256': '0' * 64 + ' 1 ../escape'})


if __name__ == '__main__':
    unittest.main()
