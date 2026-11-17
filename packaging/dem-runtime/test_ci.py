# SPDX-License-Identifier: GPL-3.0-or-later
"""Shared runner selection has no independent native publication authority."""
from contextlib import redirect_stderr
import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

import ci


class CITests(unittest.TestCase):
    def test_build_runners_follow_shared_catalog(self):
        catalog = {'arch_data': {'amd64': {'runner': 'next-amd64'}, 'arm64': {'runner': 'next-arm64'}}}
        self.assertEqual(ci.build_matrix(catalog), {'include': [
            {'arch': 'amd64', 'runner': 'next-amd64'},
            {'arch': 'arm64', 'runner': 'next-arm64'}]})

    def test_native_submission_is_not_a_runtime_command(self):
        with patch.object(sys, 'argv', ['ci.py', 'upload-native', '--artifacts', '/unused']), \
                patch.object(ci, 'read_catalog', side_effect=AssertionError('catalog read')), \
                redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as failure:
                ci.main()
        self.assertEqual(failure.exception.code, 2)
        self.assertEqual(ci.ROOT / '.github/data/distros.yml',
                         Path(__file__).resolve().parents[2] / '.github/data/distros.yml')


class NativeArtifactTests(unittest.TestCase):
    def fixture(self, root, arch='amd64'):
        import release
        artifacts = root / 'artifacts'
        artifacts.mkdir()
        agent = {'version': 'v2.12.0-607-nightly', 'channel': 'nightly', 'source': 'a' * 40}
        info = ''.join(k + '=' + agent[k] + '\n' for k in ('version', 'channel', 'source')).encode()
        expected = root / 'release-info'
        expected.write_bytes(info)
        manifest = {'format': 1, 'contract': 2, 'platform': 'linux/' + arch, 'version': agent['version']}
        payload = root / 'stage'
        payload.mkdir()
        (payload / 'release-info').write_bytes(info)
        release.write_json(payload / 'native.json', {'format': 1, 'path': '/usr/lib/netdata/dem-runtime',
                                                     'manifest': manifest})
        (payload / 'program').write_bytes(b'bound bytes')
        (payload / 'program').chmod(0o755)
        release.archive_native(payload, artifacts, 12345)
        (artifacts / 'release-info').write_bytes(info)
        release.write_json(artifacts / 'manifest.json', manifest)
        release.write_json(artifacts / 'provenance.json', {'agent': agent, 'platform': 'linux/' + arch})
        inventory = {'format': 2, 'architecture': arch, 'agent': agent,
                     'files': {p.name: {'sha256': release.sha256(p), 'bytes': p.stat().st_size}
                               for p in artifacts.iterdir()}}
        release.write_json(artifacts / 'artifacts.json', inventory)
        receipt = root / 'receipt'
        receipt.mkdir()
        (receipt / 'release-info').write_bytes(info)
        for item in ci.ARCHES:
            release.write_json(receipt / (item + '.json'), dict(inventory, architecture=item))
        from types import SimpleNamespace
        return SimpleNamespace(artifacts=artifacts, arch=arch, release_info=expected,
                               output=root / 'output', receipt=receipt)

    def test_publication_lane_requires_both_exact_companion_receipts(self):
        import tempfile
        import shutil
        from types import SimpleNamespace
        for fault in ('none', 'missing', 'wrong-pair', 'wrong-arch', 'wrong-format'):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                for arch in ci.ARCHES:
                    work = root / arch
                    work.mkdir()
                    fixture = self.fixture(work, arch)
                    shutil.copyfile(fixture.artifacts / 'artifacts.json', root / (arch + '.json'))
                shutil.copyfile(fixture.release_info, root / 'release-info')
                args = SimpleNamespace(receipt=root, release_info=fixture.release_info)
                if fault == 'missing':
                    (root / 'arm64.json').rename(root / 'unavailable')
                elif fault == 'wrong-pair':
                    args.release_info.write_text(args.release_info.read_text().replace('a' * 40, 'b' * 40))
                elif fault == 'wrong-arch':
                    shutil.copyfile(root / 'amd64.json', root / 'arm64.json')
                elif fault == 'wrong-format':
                    path = root / 'arm64.json'
                    path.write_text(path.read_text().replace('"format": 2', '"format": 1'))
                if fault == 'none':
                    ci.verify_publication_receipt(args)
                else:
                    with self.assertRaises((ValueError, FileNotFoundError)):
                        ci.verify_publication_receipt(args)

    def test_exact_parent_artifact_preserves_prebound_bytes_and_modes(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temporary:
            args = self.fixture(Path(temporary))
            ci.prepare_native(args)
            self.assertEqual((args.output / 'release-info').read_bytes(), args.release_info.read_bytes())
            self.assertEqual((args.output / 'program').read_bytes(), b'bound bytes')
            self.assertEqual((args.output / 'program').stat().st_mode & 0o777, 0o755)

    def test_later_attempt_cannot_replace_a_successfully_published_native_artifact(self):
        import json
        import release
        import tempfile
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            args = self.fixture(root)
            inventory = json.loads((args.artifacts / 'artifacts.json').read_text())
            # A later build attempt has valid inventory and the same Agent tuple,
            # but differs from the bytes attested by successful publication.
            extra = args.artifacts / 'later-attempt'
            extra.write_bytes(b'different build output')
            inventory['files'][extra.name] = {'sha256': release.sha256(extra), 'bytes': extra.stat().st_size}
            release.write_json(args.artifacts / 'artifacts.json', inventory)
            with self.assertRaisesRegex(ValueError, 'publication receipt'):
                ci.prepare_native(args)
            self.assertFalse(args.output.exists())

    def test_rejects_tampered_wrong_pair_wrong_arch_and_existing_output(self):
        import tempfile
        for fault in ('archive', 'source', 'architecture', 'output'):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as temporary:
                args = self.fixture(Path(temporary))
                if fault == 'archive':
                    (args.artifacts / 'native-payload.tar.zst').write_bytes(b'altered')
                elif fault == 'source':
                    args.release_info.write_text(args.release_info.read_text().replace('a' * 40, 'b' * 40))
                elif fault == 'architecture':
                    args.arch = 'arm64'
                else:
                    args.output.mkdir()
                    (args.output / 'keep').write_bytes(b'unrelated')
                with self.assertRaises(ValueError):
                    ci.prepare_native(args)
                if fault == 'output':
                    self.assertEqual((args.output / 'keep').read_bytes(), b'unrelated')
                else:
                    self.assertFalse(args.output.exists())

    def test_filtered_extraction_and_inner_identity_are_required(self):
        import json
        import release
        import shutil
        import tarfile
        import tempfile
        for fault in ('release-info', 'native-platform', 'escape'):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                args = self.fixture(root)
                stage = root / 'stage'
                if fault == 'release-info':
                    (stage / 'release-info').write_text(
                        (stage / 'release-info').read_text().replace('a' * 40, 'b' * 40))
                elif fault == 'native-platform':
                    native = json.loads((stage / 'native.json').read_text())
                    native['manifest']['platform'] = 'linux/arm64'
                    release.write_json(stage / 'native.json', native)
                else:
                    (stage / 'escape').symlink_to('/outside')
                rebuilt = root / 'rebuilt'
                rebuilt.mkdir()
                release.archive_native(stage, rebuilt, 12345)
                target = args.artifacts / 'native-payload.tar.zst'
                shutil.copyfile(rebuilt / target.name, target)
                inventory = json.loads((args.artifacts / 'artifacts.json').read_text())
                inventory['files'][target.name] = {'sha256': release.sha256(target), 'bytes': target.stat().st_size}
                release.write_json(args.artifacts / 'artifacts.json', inventory)
                # Preserve successful publication provenance here so this test
                # reaches extraction and the inner identity checks.
                release.write_json(args.receipt / (args.arch + '.json'), inventory)
                with self.assertRaises((ValueError, tarfile.FilterError)):
                    ci.prepare_native(args)
                self.assertFalse((root / 'outside').exists())


if __name__ == '__main__':
    unittest.main()
