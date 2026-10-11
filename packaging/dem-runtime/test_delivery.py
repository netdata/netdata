# SPDX-License-Identifier: GPL-3.0-or-later
"""Observable builder and source-assembly boundaries."""
from contextlib import redirect_stderr
import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import urllib.request

import build
import bundle
import locked
import sources


class DeliveryTests(unittest.TestCase):
    def test_invalid_prefix_fails_before_acquisition(self):
        for prefix in ('https://:443', 'https://host:bad', 'https://host:99999',
                       'https://host/base?', 'https://host/base#', 'https://[broken'):
            with self.subTest(prefix=prefix), tempfile.TemporaryDirectory() as directory:
                argv = ['build.py', '--arch', 'arm64', '--output', directory, '--url-prefix', prefix]
                with patch.object(sys, 'argv', argv), redirect_stderr(io.StringIO()), \
                        patch.object(build, 'download', side_effect=AssertionError('acquisition started')):
                    with self.assertRaises(SystemExit) as error:
                        build.main()
                    self.assertEqual(error.exception.code, 2)

    def test_prefix_contract_is_shared_with_bundle_writer(self):
        for value in ('', 'https://example.invalid/releases', 'https://example.invalid:8443/path/'):
            bundle.validate_url_prefix(value)
        with self.assertRaises(ValueError):
            bundle.prepare(type('Arguments', (), {'url_prefix': 'https://example.invalid/base?'})())

    def test_donor_binaries_use_verified_acquisition(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            recipe, assets, output, work = (base / name for name in ('recipe', 'assets', 'output', 'work'))
            recipe.mkdir()
            (recipe / 'locks').mkdir()
            assets.mkdir()
            output.mkdir()
            archive = io.BytesIO()
            with tarfile.open(fileobj=archive, mode='w'):
                pass
            payload = archive.getvalue()
            digest = hashlib.sha256(payload).hexdigest()
            chromium = {'source': 'debian', 'version': 'fixture'}
            inputs = {'node_version': '24.0.0', 'patchelf_version': '0.15.0',
                      'architectures': {'arm64': {'node_arch': 'arm64', 'node_sha256': digest,
                                                  'patchelf_arch': 'aarch64', 'patchelf_sha256': digest}}}
            (recipe / 'inputs.json').write_text(json.dumps(inputs))
            (recipe / 'locks/arm64.json').write_text(json.dumps({'chromium': chromium}))
            (assets / 'manifest.json').write_text(json.dumps({'chromium': chromium, 'node_major_min': 24}))
            paths = {'/recipe': recipe, '/assets': assets, '/out': output, '/tmp/dem-build': work}

            def response(url, **kwargs):
                result = io.BytesIO(payload)
                result.url = url
                return result

            with patch.object(bundle, 'Path', side_effect=lambda value: paths.get(str(value), Path(value))), \
                    patch.object(locked, 'check_inventory'), \
                    patch.object(tarfile.TarFile, 'extractall'), \
                    patch.object(locked, 'download', wraps=locked.download) as download, \
                    patch.object(locked.HTTPS_OPENER, 'open', side_effect=response) as opened, \
                    patch.object(bundle, 'run', side_effect=RuntimeError('execution boundary')):
                with self.assertRaisesRegex(RuntimeError, 'execution boundary'):
                    bundle.prepare(type('Arguments', (), {'url_prefix': '', 'arch': 'arm64'})())
            self.assertEqual(download.call_count, 2)
            self.assertEqual([call.args[0]['name'] for call in download.call_args_list],
                             ['node.tar.xz', 'patchelf.tar.gz'])
            self.assertEqual([call.args[0]['sha256'] for call in download.call_args_list], [digest, digest])
            self.assertEqual([call.args[0] for call in opened.call_args_list], [
                'https://nodejs.org/dist/v24.0.0/node-v24.0.0-linux-arm64.tar.xz',
                'https://github.com/NixOS/patchelf/releases/download/0.15.0/patchelf-0.15.0-aarch64.tar.gz'])
            for name in ('node.tar.xz', 'patchelf.tar.gz'):
                self.assertEqual((work / name).read_bytes(), payload)

    def test_acquisition_timeout_cleans_unpublished_file(self):
        record = {'name': 'input', 'url': 'https://example.invalid/input',
                  'sha256': hashlib.sha256(b'x').hexdigest(), 'bytes': 1}
        with tempfile.TemporaryDirectory() as directory, \
                patch('urllib.request.OpenerDirector.open', side_effect=TimeoutError('stalled')) as opened:
            with self.assertRaises(TimeoutError):
                locked.download(record, Path(directory))
            self.assertGreater(opened.call_args.kwargs.get('timeout', 0), 0)
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_oversized_download_stops_before_consuming_body(self):
        class Response(io.BytesIO):
            url = 'https://example.invalid/input'
            consumed = 0

            def read(self, size=-1):
                result = super().read(size)
                self.consumed += len(result)
                return result

        response = Response(b'x' * (3 * 1024 * 1024))
        record = {'name': 'input', 'url': response.url,
                  'sha256': hashlib.sha256(b'x').hexdigest(), 'bytes': 1}
        with tempfile.TemporaryDirectory() as directory, \
                patch('urllib.request.OpenerDirector.open', return_value=response):
            with self.assertRaises(ValueError):
                locked.download(record, Path(directory))
            self.assertLess(response.consumed, 3 * 1024 * 1024)
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_redirect_rejects_downgrade_before_opening_target(self):
        record = {'name': 'input', 'url': 'https://example.invalid/input',
                  'sha256': hashlib.sha256(b'x').hexdigest(), 'bytes': 1}
        # Drive the real redirect handler with an HTTP response, without network.
        for target in ('http://example.invalid/clear', 'ftp://example.invalid/file'):
            with self.subTest(target=target):
                opener = urllib.request.build_opener(locked.HTTPSRedirectHandler())
                with patch.object(opener, 'open', side_effect=AssertionError('target opened')):
                    handler = next(h for h in opener.handlers if isinstance(h, locked.HTTPSRedirectHandler))
                    with self.assertRaises(ValueError):
                        handler.http_error_302(urllib.request.Request(record['url']), io.BytesIO(),
                                               302, 'redirect', {'location': target})

    def fixture(self, base):
        recipe = base / 'packaging/dem-runtime'
        recipe.mkdir(parents=True)
        (recipe / 'build.py').write_text('# exact recipe')
        assets = base / 'src/go/plugin/dem/synthetic/runner/assets'
        assets.mkdir(parents=True)
        for name, data in [('manifest.json', {}), ('package.json', {}),
                           ('package-lock.json', {'packages': {}})]:
            (assets / name).write_text(json.dumps(data))
        runtime = base / 'runtime'
        runtime.mkdir()
        provenance = {'recipe_sha256': {'build.py': hashlib.sha256(b'# exact recipe').hexdigest()},
                      'source_sha256': {}, 'debian_sources': {}, 'inputs': {'sources': {}}}
        for name, data in [('manifest.json', {'archive': {'sha256': 'fixture'}}),
                           ('provenance.json', provenance), ('sbom.cdx.json', {}),
                           ('sources-required.json', {})]:
            (runtime / name).write_text(json.dumps(data))
        return recipe, runtime

    def assemble(self, recipe, runtime, output):
        argv = ['sources.py', '--runtime-output', str(runtime), '--output', str(output)]
        with patch.object(sources, '__file__', str(recipe / 'sources.py')), patch.object(sys, 'argv', argv):
            sources.main()

    def test_nested_output_rejected_before_mutation(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            recipe, runtime = self.fixture(base)
            alias = base / 'alias'
            alias.symlink_to(recipe, target_is_directory=True)
            for output in (recipe / 'output', alias / 'output'):
                with self.subTest(output=output), redirect_stderr(io.StringIO()), \
                        patch.object(sources.shutil, 'copyfile', side_effect=AssertionError('copy started')):
                    with self.assertRaises(SystemExit):
                        self.assemble(recipe, runtime, output)
                    self.assertFalse(output.exists())

    def test_source_rerun_and_unexpected_file_rejection(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            recipe, runtime = self.fixture(base)
            output = base / 'output'
            self.assemble(recipe, runtime, output)
            first = (output / 'source-assembly.json').read_bytes()
            self.assemble(recipe, runtime, output)
            self.assertEqual(first, (output / 'source-assembly.json').read_bytes())
            stale = output / 'other-architecture.deb'
            stale.write_bytes(b'unverified')
            with self.assertRaisesRegex(ValueError, 'unexpected'):
                self.assemble(recipe, runtime, output)
            self.assertEqual(stale.read_bytes(), b'unverified')
            self.assertEqual(first, (output / 'source-assembly.json').read_bytes())


if __name__ == '__main__':
    unittest.main()
