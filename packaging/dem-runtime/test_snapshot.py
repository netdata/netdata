# SPDX-License-Identifier: GPL-3.0-or-later
"""Snapshot locators never replace the lock's authenticated content identity."""
import copy
import hashlib
import io
import importlib.util
import json
from pathlib import Path
import tempfile
from unittest.mock import patch
import unittest

import snapshot


class SnapshotTests(unittest.TestCase):
    def fixture(self):
        binary, source = b'binary bytes', b'source bytes'
        binary_hash = hashlib.sha1(binary).hexdigest()
        source_hash = hashlib.sha1(source).hexdigest()

        def record(name, data):
            return {'name': name, 'sha256': hashlib.sha256(data).hexdigest(),
                    'bytes': len(data), 'url': 'https://deb.debian.org/pool/' + name}
        package = dict(record('example', binary), file='example_2.0_all.deb',
                       version='1:2.0-3+b1', architecture='all', source='example', source_version='1:2.0-3')
        lock = {'format': 1, 'architecture': 'amd64', 'binary_packages': [package],
                'sources': {'example@1:2.0-3': {'name': 'example', 'version': '1:2.0-3',
                            'files': [record('example_2.0-3.dsc', source)]}},
                'index_sha256': {'InRelease': 'retained'}, 'installed': ['unchanged']}
        metadata = {'package': 'example', 'version': '1:2.0-3',
                    'result': {'source': [{'hash': source_hash}], 'binaries': [
                        {'name': 'example', 'version': '1:2.0-3+b1',
                         'files': [{'architecture': 'all', 'hash': binary_hash}]}]},
                    'fileinfo': {binary_hash: [{'name': package['file'], 'size': len(binary)}],
                                 source_hash: [{'name': 'example_2.0-3.dsc', 'size': len(source)}]}}
        return lock, metadata, {binary_hash: binary, source_hash: source}

    def convert(self, locks, metadata, bodies):
        urls = []

        def opened(url, **kwargs):
            urls.append(url)
            data = json.dumps(metadata).encode() if '/mr/' in url else bodies[url.rsplit('/', 1)[1]]
            response = io.BytesIO(data)
            response.url = url
            return response
        with patch.object(snapshot.HTTPS_OPENER, 'open', side_effect=opened):
            result = snapshot.resolve_locks(locks)
        return result, urls

    def test_complete_conversion_preserves_identity_and_deduplicates_across_architectures(self):
        lock, metadata, bodies = self.fixture()
        other = copy.deepcopy(lock)
        other['architecture'] = 'arm64'
        originals = copy.deepcopy([lock, other])
        converted, urls = self.convert([lock, other], metadata, bodies)
        self.assertEqual([lock, other], originals)
        self.assertEqual(sum('/mr/' in url for url in urls), 1)
        self.assertIn('/example/1%3A2.0-3/allfiles?fileinfo=1', urls[0])
        self.assertEqual(len(urls), 3)
        for old, new in zip(originals, converted):
            self.assertNotEqual(old['binary_packages'][0]['url'], new['binary_packages'][0]['url'])
            self.assertTrue(new['binary_packages'][0]['url'].startswith('https://snapshot.debian.org/file/'))
            new['binary_packages'][0]['url'] = old['binary_packages'][0]['url']
            new['sources']['example@1:2.0-3']['files'][0]['url'] = old['sources']['example@1:2.0-3']['files'][0]['url']
            self.assertEqual(old, new)

    def test_ambiguous_name_and_size_resolves_only_by_authenticated_bytes(self):
        lock, metadata, bodies = self.fixture()
        wrong = '0' * 40
        metadata['result']['binaries'][0]['files'].insert(0, {'architecture': 'all', 'hash': wrong})
        metadata['fileinfo'][wrong] = [{'name': lock['binary_packages'][0]['file'], 'size': 12}]
        bodies[wrong] = b'wrong! bytes'
        converted, urls = self.convert([lock], metadata, bodies)
        self.assertNotIn(wrong, converted[0]['binary_packages'][0]['url'])
        self.assertIn('https://snapshot.debian.org/file/' + wrong, urls)

    def test_missing_mismatched_and_wrong_identity_fail_without_mutating_locks(self):
        for fault in ('missing', 'bytes', 'architecture', 'version', 'source', 'filename', 'size'):
            with self.subTest(fault=fault):
                lock, metadata, bodies = self.fixture()
                original = copy.deepcopy(lock)
                candidate = metadata['result']['binaries'][0]['files'][0]
                if fault == 'missing':
                    metadata['result']['binaries'] = []
                elif fault == 'bytes':
                    bodies[candidate['hash']] = b'wrong! bytes'
                elif fault == 'architecture':
                    candidate['architecture'] = 'arm64'
                elif fault == 'version':
                    metadata['result']['binaries'][0]['version'] = '2.0-4'
                elif fault == 'source':
                    metadata['package'] = 'other'
                elif fault == 'filename':
                    metadata['fileinfo'][candidate['hash']][0]['name'] = 'other.deb'
                else:
                    metadata['fileinfo'][candidate['hash']][0]['size'] = 13
                with self.assertRaises(ValueError):
                    self.convert([lock], metadata, bodies)
                self.assertEqual(lock, original)

    def test_conversion_command_never_resolves_packages_or_publishes_partial_locks(self):
        spec = importlib.util.spec_from_file_location('refresh_lock', Path(__file__).with_name('refresh-lock.py'))
        refresh = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(refresh)
        for missing in (False, True):
            with self.subTest(missing=missing), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                lock, metadata, bodies = self.fixture()
                original = root / 'existing.json'
                original.write_text(json.dumps(lock))
                output = root / 'converted'
                if missing:
                    metadata['result']['source'] = []

                def opened(url, **kwargs):
                    data = json.dumps(metadata).encode() if '/mr/' in url else bodies[url.rsplit('/', 1)[1]]
                    response = io.BytesIO(data)
                    response.url = url
                    return response
                with patch('sys.argv', ['refresh-lock.py', '--snapshot-lock', str(original),
                                        '--output', str(output)]), \
                        patch.object(refresh, 'run', side_effect=AssertionError('apt or Docker invoked')), \
                        patch.object(snapshot.HTTPS_OPENER, 'open', side_effect=opened):
                    if missing:
                        with self.assertRaises(ValueError):
                            refresh.main()
                        self.assertEqual(list(output.iterdir()), [])
                    else:
                        refresh.main()
                        converted = json.loads((output / 'amd64.json').read_text())
                        self.assertTrue(converted['binary_packages'][0]['url'].startswith(snapshot.BASE))
                self.assertEqual(json.loads(original.read_text()), lock)


if __name__ == '__main__':
    unittest.main()
