# SPDX-License-Identifier: GPL-3.0-or-later
"""Observable builder and source-assembly boundaries."""
from contextlib import redirect_stderr
import hashlib
import io
import json
from pathlib import Path
import sys
import subprocess
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
                       'https://host/base?', 'https://host/base#', 'https://[broken',
                       'https://@host/base', 'https://:@host/base'):
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
            inputs = {'version': 'fixture', 'node_version': '24.0.0', 'patchelf_version': '0.15.0',
                      'architectures': {'arm64': {'node_arch': 'arm64', 'node_sha256': digest,
                                                  'patchelf_arch': 'aarch64', 'patchelf_sha256': digest}}}
            (recipe / 'inputs.json').write_text(json.dumps(inputs))
            (recipe / 'locks/arm64.json').write_text(json.dumps({'chromium': chromium}))
            (assets / 'manifest.json').write_text(json.dumps({'chromium': chromium, 'node_major_min': 24,
                                                             'node_version': '24.0.0'}))
            paths = {'/recipe': recipe, '/assets': assets, '/out': output, '/tmp/dem-build': work}

            def response(url, **kwargs):
                result = io.BytesIO(payload)
                result.url = url
                return result

            with patch.object(bundle, 'Path', side_effect=lambda value: paths.get(str(value), Path(value))), \
                    patch.object(locked, 'check_inventory'), \
                    patch.object(bundle, 'install_runner'), \
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
        for target in ('http://example.invalid/clear', 'ftp://example.invalid/file',
                       'https://@example.invalid/file', 'https://:@example.invalid/file'):
            with self.subTest(target=target):
                opener = urllib.request.build_opener(locked.HTTPSRedirectHandler())
                with patch.object(opener, 'open', side_effect=AssertionError('target opened')):
                    handler = next(h for h in opener.handlers if isinstance(h, locked.HTTPSRedirectHandler))
                    with self.assertRaises(ValueError):
                        handler.http_error_302(urllib.request.Request(record['url']), io.BytesIO(),
                                               302, 'redirect', {'location': target})

    def test_recipe_inventory_covers_native_policy(self):
        self.assertIn('native/netdata-dem-browser', bundle.recipe_sources())

    def fixture(self, base):
        recipe = base / 'packaging/dem-runtime'
        recipe.mkdir(parents=True)
        for name in bundle.recipe_sources():
            path = recipe / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('# exact recipe')
        assets = base / 'src/dem-runtime'
        assets.mkdir(parents=True)
        for name, data in [('manifest.json', {}), ('runtime.json', {}), ('README.md', {}), ('package.json', {}),
                           ('package-lock.json', {'packages': {}})]:
            (assets / name).write_text(json.dumps(data))
        (assets / 'runner').mkdir()
        (assets / 'runner/run.mjs').write_text('// exact runtime adapter')
        runtime = base / 'runtime'
        runtime.mkdir()
        provenance = {'recipe_sha256': {name: bundle.sha256(recipe / name) for name in bundle.recipe_sources()},
                      'source_sha256': {path.relative_to(assets).as_posix(): bundle.sha256(path)
                                        for path in assets.rglob('*') if path.is_file()},
                      'debian_sources': {}, 'inputs': {'sources': {}}}
        for name, data in [('manifest.json', {'archive': {'sha256': 'fixture'}}),
                           ('provenance.json', provenance), ('sbom.cdx.json', {}),
                           ('sources-required.json', {})]:
            (runtime / name).write_text(json.dumps(data))
        module = base / 'src/go'
        module.mkdir()
        for name in ('go.mod', 'go.sum', 'main.go'):
            (module / name).write_text(name)
        return recipe, runtime

    def assemble(self, recipe, runtime, output):
        argv = ['sources.py', '--runtime-output', str(runtime), '--output', str(output)]
        module = recipe.parent.parent / 'src/go'
        package = {'Dir': str(module), 'Module': {'Main': True, 'Dir': str(module)},
                   'GoFiles': ['main.go']}
        with patch.object(sources, '__file__', str(recipe / 'sources.py')), patch.object(sys, 'argv', argv), \
                patch.object(sources, 'run', return_value=json.dumps(package)):
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
            self.assertEqual((output / 'runtime-source/runner/run.mjs').read_text(), '// exact runtime adapter')
            first = (output / 'source-assembly.json').read_bytes()
            self.assemble(recipe, runtime, output)
            self.assertEqual(first, (output / 'source-assembly.json').read_bytes())
            stale = output / 'other-architecture.deb'
            stale.write_bytes(b'unverified')
            with self.assertRaisesRegex(ValueError, 'unexpected'):
                self.assemble(recipe, runtime, output)
            self.assertEqual(stale.read_bytes(), b'unverified')
            self.assertEqual(first, (output / 'source-assembly.json').read_bytes())

    def test_source_assembly_rejects_changed_runtime_adapter(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            recipe, runtime = self.fixture(base)
            (base / 'src/dem-runtime/runner/run.mjs').write_text('// changed after assembly')
            with self.assertRaisesRegex(ValueError, 'source input differs'):
                self.assemble(recipe, runtime, base / 'output')
            self.assertFalse((base / 'output').exists())

    def test_source_archive_contains_complete_inventory_and_is_repeatable(self):
        import tarfile
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            recipe, runtime = self.fixture(base)
            output = base / 'sources'
            self.assemble(recipe, runtime, output)
            first = sources.archive_sources(output, base / 'first', 'fixture', 12345)
            second = sources.archive_sources(output, base / 'second', 'fixture', 12345)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            identity = json.loads((base / 'first/source-archive.json').read_text())
            self.assertEqual(identity['sha256'], bundle.sha256(first))
            self.assertEqual(identity['bytes'], first.stat().st_size)
            inventory = json.loads((output / 'source-assembly.json').read_text())
            with tarfile.open(fileobj=io.BytesIO(subprocess.run(
                    ['zstd', '-d', '-c', first], capture_output=True, check=True).stdout)) as archive:
                files = {m.name for m in archive.getmembers() if m.isfile()}
                self.assertEqual(files, set(inventory['files']) | {'source-assembly.json'})
                self.assertEqual(archive.extractfile('runtime-source/runner/run.mjs').read(),
                                 b'// exact runtime adapter')
            with self.assertRaisesRegex(ValueError, 'empty'):
                sources.archive_sources(output, base / 'first', 'fixture', 12345)
            self.assertEqual(first.read_bytes(), second.read_bytes())

    def test_source_archive_rejects_incomplete_or_substituted_assembly(self):
        for fault in ('changed', 'missing', 'extra', 'symlink', 'identity'):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as directory:
                base = Path(directory).resolve()
                recipe, runtime = self.fixture(base)
                output = base / 'sources'
                self.assemble(recipe, runtime, output)
                target = output / 'runtime-source/runner/run.mjs'
                if fault == 'changed':
                    target.write_text('// changed')
                elif fault == 'missing':
                    target.unlink()
                elif fault == 'extra':
                    (output / 'unverified').write_text('extra')
                elif fault == 'symlink':
                    (output / 'alias').symlink_to(target)
                with self.assertRaises(ValueError):
                    sources.archive_sources(output, base / 'archives',
                                            'other' if fault == 'identity' else 'fixture', 12345)
                self.assertFalse((base / 'archives').exists())

    def test_source_archive_output_cannot_contaminate_assembly(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            recipe, runtime = self.fixture(base)
            output = base / 'sources'
            self.assemble(recipe, runtime, output)
            for destination in (output, output / 'archives', base):
                with self.subTest(destination=destination), self.assertRaisesRegex(ValueError, 'separate'):
                    sources.archive_sources(output, destination, 'fixture', 12345)


class ReleaseToolSourcesTests(unittest.TestCase):
    def fixture(self, root):
        for name in ('go.mod', 'go.sum', 'tool/main.go', 'tool/arm64.s', 'tool/data/file.txt'):
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(name)
        (root / 'tool/local-secret').write_text('excluded fixture')
        return {'Dir': str(root / 'tool'), 'Module': {'Main': True, 'Dir': str(root)},
                'GoFiles': ['main.go'], 'EmbedFiles': ['data/file.txt']}

    def test_union_preserves_build_inputs_and_excludes_unlisted_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            package = self.fixture(root)

            def listing(command, **kwargs):
                self.assertEqual(command, ['go', 'list', '-mod=readonly', '-deps', '-json',
                                           './tools/dem-runtime-release'])
                env = kwargs['env']
                self.assertEqual((env['GOOS'], env['GOWORK'], env['GOFLAGS']), ('linux', 'off', ''))
                selected = dict(package)
                if env['GOARCH'] == 'arm64':
                    selected['SFiles'] = ['arm64.s']
                return json.dumps({'Standard': True}) + '\n' + json.dumps(selected)
            with patch.object(sources, 'run', side_effect=listing) as run:
                result = sources.release_tool_sources(root)
            self.assertEqual(run.call_count, 4)
            self.assertEqual(set(result), {'go.mod', 'go.sum', 'tool/main.go', 'tool/arm64.s',
                                           'tool/data/file.txt'})
            for name, digest in result.items():
                self.assertEqual(digest, bundle.sha256(root / name))

    def test_rejects_external_local_modules_and_escaped_inputs(self):
        for fault in ('replacement', 'outside', 'parent', 'symlink'):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as directory:
                root = Path(directory).resolve()
                package = self.fixture(root)
                if fault == 'replacement':
                    package['Module'] = {'Replace': {'Dir': str(root)}, 'Main': False}
                elif fault == 'outside':
                    package['Dir'] = str(root.parent)
                elif fault == 'parent':
                    package['GoFiles'] = ['../go.mod']
                else:
                    (root / 'tool/link.go').symlink_to('main.go')
                    package['GoFiles'] = ['link.go']
                with patch.object(sources, 'run', return_value=json.dumps(package)):
                    with self.assertRaises(ValueError):
                        sources.release_tool_sources(root)

    def test_native_recipe_is_verified_and_archived(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            helper = DeliveryTests()
            recipe, runtime = helper.fixture(base)
            helper.assemble(recipe, runtime, base / 'sources')
            record = json.loads((base / 'sources/source-assembly.json').read_text())
            self.assertIn('recipe/native/CMakeLists.txt', record['files'])
            self.assertIn('recipe/native/lifecycle.sh', record['files'])
            for name in ('go.mod', 'go.sum', 'main.go'):
                self.assertEqual(record['files']['release-tool/' + name]['sha256'],
                                 record['release_tool']['source_sha256'][name])
            (recipe / 'native/lifecycle.sh').write_text('changed')
            with self.assertRaisesRegex(ValueError, 'source input differs'):
                helper.assemble(recipe, runtime, base / 'sources')


if __name__ == '__main__':
    unittest.main()
