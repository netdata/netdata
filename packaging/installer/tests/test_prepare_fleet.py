#!/usr/bin/env python3
"""Exercise fleet preparation with offline fixtures; never execute an installer."""
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / 'packaging/makeself/prepare-fleet.sh'


class PrepareFleetTests(unittest.TestCase):
    def setUp(self):
        scratch = Path(os.environ.get('TMPDIR', Path.home() / 'tmp'))
        scratch.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-fleet-tests-', dir=scratch)
        self.root = Path(self.temp.name)
        self.source = self.root / 'source'
        self.source.mkdir()
        self.put('bin/netdata', 'wrapper')
        self.put('bin/srv/netdata', 'daemon')
        for name in ('bin/bash', 'bin/nd-run', 'bin/curl', 'bin/netdatacli',
                     'system/post-installer.sh', 'system/functions.sh'):
            self.put(name, 'retained helper')
        self.put('system/install-or-update.sh', (ROOT / 'packaging/makeself/install-or-update.sh').read_text())
        self.put('etc/netdata/custom.conf', 'secret stays in file')
        self.put('var/lib/netdata/state', 'state')
        for plugin in ('apps.plugin', 'network-viewer.plugin', 'go.d.plugin', 'local-listeners', 'snmp-trap-profile-gen', 'ndsudo', 'loopsleepms.sh.inc'):
            self.put('usr/libexec/netdata/plugins.d/' + plugin, plugin)
        self.put('usr/lib/netdata/conf.d/go.d/custom.conf', 'stock')
        self.put('usr/lib/netdata/conf.d/go.d.conf', 'stock framework config')
        self.put('usr/share/netdata/web/index.html', 'dashboard')
        self.put('usr/share/netdata/future-feature/data', 'unknown')

    def tearDown(self):
        self.temp.cleanup()

    def put(self, name, data):
        path = self.source / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(data)
        return path

    def run_script(self, *args, success=True):
        result = subprocess.run([str(SCRIPT), *map(str, args)], cwd=self.root, capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, success, result.stderr)
        return result

    def prepare(self, keep='none', **options):
        target = self.root / 'output'
        args = ['--source', self.source, '--keep', keep, '--strip-mode', options.get('strip', 'none'),
                '--apply', '--output', target]
        if 'objcopy' in options:
            args += ['--objcopy', options['objcopy']]
        result = self.run_script(*args, success=options.get('success', True))
        return target, result

    def test_dependency_and_shared_user_unknown_preservation(self):
        target, _ = self.prepare('network')
        for name in ('apps.plugin', 'network-viewer.plugin', 'ndsudo', 'loopsleepms.sh.inc'):
            self.assertTrue((target / 'usr/libexec/netdata/plugins.d' / name).exists())
        self.assertFalse((target / 'usr/libexec/netdata/plugins.d/go.d.plugin').exists())
        self.assertFalse((target / 'usr/lib/netdata/conf.d/go.d').exists())
        self.assertEqual((target / 'etc/netdata/custom.conf').read_text(), 'secret stays in file')
        self.assertEqual((target / 'var/lib/netdata/state').read_text(), 'state')
        self.assertTrue((target / 'usr/share/netdata/future-feature/data').exists())
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertEqual(manifest['resolved'], ['apps', 'network'])
        self.assertTrue((self.source / 'usr/libexec/netdata/plugins.d/go.d.plugin').exists())

    def test_missing_capability_companion_refused(self):
        other = self.root / 'incomplete-go'
        shutil.copytree(self.source, other, symlinks=True,
                        ignore=lambda directory, names: ['local-listeners'])
        result = self.run_script('--source', other, '--keep', 'go', success=False)
        self.assertIn('incomplete capability go', result.stderr)

    def test_preview_does_not_publish(self):
        result = self.run_script('--source', self.source, '--keep', 'none', '--output', self.root / 'preview')
        self.assertIn('Preview only', result.stderr)
        self.assertFalse((self.root / 'preview').exists())
        self.assertEqual(json.loads(result.stdout)['stripped_files'], [])

    def test_unknown_or_missing_capability_fails(self):
        for name in ('unrecognized', 'journal'):
            self.run_script('--source', self.source, '--keep', name, success=False)

    def test_dashboard_removal_retains_required_empty_web_directory(self):
        target, _ = self.prepare('none')
        web = target / 'usr/share/netdata/web'
        self.assertTrue(web.is_dir(), 'Agent startup requires NETDATA_WEB_DIR')
        self.assertEqual(list(web.iterdir()), [])
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertNotIn('usr/share/netdata/web', manifest['removed_paths'])
        self.assertIn('usr/share/netdata/web/index.html', manifest['removed_paths'])

    def test_all_keeps_available_plugins(self):
        target, _ = self.prepare('all')
        self.assertTrue((target / 'usr/libexec/netdata/plugins.d/go.d.plugin').exists())
        self.assertTrue((target / 'usr/share/netdata/web/index.html').exists())

    def test_repeat_output_refuses_overwrite(self):
        target, _ = self.prepare()
        sentinel = target / 'sentinel'
        sentinel.write_text('preserve')
        self.run_script('--source', self.source, '--keep', 'none', '--apply', '--output', target, success=False)
        self.assertEqual(sentinel.read_text(), 'preserve')

    def test_overlap_and_live_path_refusal(self):
        self.run_script('--source', self.source, '--keep', 'none', '--apply',
                        '--output', self.source / 'nested', success=False)
        self.run_script('--source', self.source, '--keep', 'none', '--apply',
                        '--output', '/opt/netdata', success=False)

    def test_symlink_escape_and_special_file_refusal(self):
        os.symlink('../../outside', self.source / 'escape')
        self.run_script('--source', self.source, '--keep', 'none', success=False)
        # A separate fixture avoids modifying a previous refusal's evidence.
        other = self.root / 'fifo-source'
        shutil.copytree(self.source, other, symlinks=True, ignore=lambda directory, names: ['escape'])
        os.mkfifo(other / 'pipe')
        self.run_script('--source', other, '--keep', 'none', success=False)

    def test_internal_and_static_absolute_links_preserved(self):
        os.symlink('/opt/netdata/usr/lib/netdata/conf.d', self.source / 'etc/netdata/orig')
        os.symlink('netdata', self.source / 'bin/internal')
        target, _ = self.prepare()
        self.assertEqual(os.readlink(target / 'etc/netdata/orig'), '/opt/netdata/usr/lib/netdata/conf.d')
        self.assertEqual(os.readlink(target / 'bin/internal'), 'netdata')

    def test_manifest_symlink_chain_refused_before_writing(self):
        # Build an otherwise valid tree with no existing manifest ancestors.
        other = self.root / 'chain-source'
        shutil.copytree(self.source, other, symlinks=True,
                        ignore=lambda directory, names: ['share'] if Path(directory).name == 'usr' else [])
        (other / 'usr/share').mkdir()
        os.symlink('../../etc/netdata/orig', other / 'usr/share/netdata')
        os.symlink('/opt/netdata/usr/lib/netdata/conf.d', other / 'etc/netdata/orig')
        result = self.run_script('--source', other, '--keep', 'none', success=False)
        self.assertIn('ancestor of generated manifest', result.stderr)

    def test_missing_required_shared_helper_refused(self):
        other = self.root / 'missing-helper'
        shutil.copytree(self.source, other, symlinks=True,
                        ignore=lambda directory, names: ['bash'] if Path(directory).name == 'bin' else [])
        self.run_script('--source', other, '--keep', 'none', success=False)

    @unittest.skipUnless(shutil.which('objcopy'), 'native objcopy unavailable')
    def test_successful_default_debug_strip_preserves_executable_mode(self):
        shutil.copyfile('/bin/true', self.source / 'bin/srv/netdata')
        (self.source / 'bin/srv/netdata').chmod(0o755)
        shutil.copyfile('/bin/true', self.source / 'bin/nd-run')
        target = self.root / 'debug-output'
        self.run_script('--source', self.source, '--keep', 'apps', '--apply', '--output', target)
        binary = target / 'bin/srv/netdata'
        self.assertEqual(binary.stat().st_mode & 0o7777, 0o755)
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertEqual(manifest['strip_mode'], 'debug')
        self.assertEqual(len(manifest['stripped_files']), 2)

    @unittest.skipUnless(shutil.which('objcopy') and shutil.which('cc'), 'native compiler or objcopy unavailable')
    def test_user_state_and_unknown_elf_files_remain_byte_identical(self):
        source = self.root / 'fixture.c'
        source.write_text('int main(void) { return 0; }\n')
        executable = self.root / 'debug-executable'
        subprocess.run(['cc', '-g', str(source), '-o', str(executable)], check=True, capture_output=True)
        names = ('etc/netdata/firmware', 'var/lib/netdata/saved-binary',
                 'usr/libexec/netdata/plugins.d/custom.plugin')
        for name in names:
            path = self.source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(executable, path)
        target, _ = self.prepare('none', strip='all')
        for name in names:
            self.assertEqual((target / name).read_bytes(), (self.source / name).read_bytes(), name)

    def test_strip_failure_does_not_publish(self):
        shutil.copyfile('/bin/true', self.source / 'bin/srv/netdata')
        target, result = self.prepare(strip='debug', objcopy='/bin/false', success=False)
        self.assertFalse(target.exists())
        self.assertIn('command failed (1)', result.stderr)

    def test_allocated_section_mutation_refused(self):
        shutil.copyfile('/bin/true', self.source / 'bin/srv/netdata')
        tool = self.root / 'bad-objcopy'
        tool.write_text('#!/usr/bin/env python3\nimport pathlib,sys\np=pathlib.Path(sys.argv[2]); d=bytearray(p.read_bytes()); d[24]^=1; pathlib.Path(sys.argv[3]).write_bytes(d)\n')
        tool.chmod(0o755)
        target, result = self.prepare(strip='all', objcopy=tool, success=False)
        self.assertFalse(target.exists())
        self.assertIn('changed allocated sections or ELF identity', result.stderr)

    def installer(self, extra=None, omit_directories=False):
        data = io.BytesIO()
        with tarfile.open(fileobj=data, mode='w') as tf:
            if omit_directories:
                for file in sorted(self.source.rglob('*')):
                    if not file.is_dir() or file.is_symlink():
                        tf.add(file, arcname='./' + file.relative_to(self.source).as_posix(), recursive=False)
            else:
                tf.add(self.source, arcname='.', recursive=True)
            if extra:
                tf.addfile(extra, io.BytesIO(b'x'))
        payload = gzip.compress(data.getvalue(), mtime=0)
        header = ('#!/bin/sh\nskip="15"\nfilesizes="SIZE"\ntotalsize="SIZE"\nCRCsum="0"\nMD5="0"\nSHA="0"\n'
                  'SIGNATURE=""\ndecrypt_cmd=""\ntargetdir="/opt/netdata"\nscript="./system/post-installer.sh"\n'
                  'eval "gzip -cd"\necho Uncompressed size: 999 KB\nMS_Printf "About to extract 999 KB in $tmpdir (999 KB)"\n'
                  'if test "$leftspace" -lt 999; then\n').replace('SIZE', str(len(payload)))
        path = self.root / 'input.gz.run'
        path.write_bytes(header.encode() + payload)
        return path, hashlib.sha256(path.read_bytes()).hexdigest()

    def test_archive_rebuilt_metadata_checksums_and_space(self):
        path, sha = self.installer()
        target = self.root / 'reduced.gz.run'
        self.run_script('--input', path, '--sha256', sha, '--keep', 'apps', '--strip-mode', 'none', '--apply', '--output', target)
        content = target.read_bytes()
        header = b'\n'.join(content.split(b'\n', 15)[:15]) + b'\n'
        payload = content[len(header):]
        raw = gzip.decompress(payload)
        checksum = subprocess.check_output(['cksum'], input=payload).split()[0]
        self.assertIn(b'CRCsum="' + checksum + b'"', header)
        self.assertIn(b'MD5="' + hashlib.md5(payload).hexdigest().encode() + b'"', header)
        self.assertIn(b'SHA="' + hashlib.sha256(payload).hexdigest().encode() + b'"', header)
        self.assertNotIn(b'999', header)
        self.assertIn(str((len(raw) + 1023) // 1024).encode() + b' KB', header)
        self.assertTrue(Path(str(target) + '.sha256').exists())
        with tarfile.open(fileobj=io.BytesIO(raw)) as tf:
            self.assertIn('./bin/srv/netdata', tf.getnames())
            self.assertNotIn('./usr/libexec/netdata/plugins.d/go.d.plugin', tf.getnames())
            self.assertEqual(tf.getmember('./bin/srv/netdata').mode, (self.source / 'bin/srv/netdata').stat().st_mode & 0o7777)

    def test_archive_with_implicit_directories_keeps_directory_capabilities(self):
        path, sha = self.installer(omit_directories=True)
        result = self.run_script('--input', path, '--sha256', sha, '--keep', 'dashboard,go')
        self.assertEqual(json.loads(result.stdout)['resolved'], ['dashboard', 'go'])

    def test_archive_with_implicit_directories_retains_empty_web_directory(self):
        path, sha = self.installer(omit_directories=True)
        target = self.root / 'implicit-dirs.gz.run'
        self.run_script('--input', path, '--sha256', sha, '--keep', 'apps',
                        '--strip-mode', 'none', '--apply', '--output', target)
        content = target.read_bytes()
        payload = content.split(b'\n', 15)[15]
        with tarfile.open(fileobj=io.BytesIO(gzip.decompress(payload))) as tf:
            self.assertTrue(tf.getmember('./usr/share/netdata/web').isdir())
            self.assertNotIn('./usr/share/netdata/web/index.html', tf.getnames())

    def test_archive_checksum_and_traversal_refusal(self):
        path, sha = self.installer()
        self.run_script('--input', path, '--sha256', '0' * 64, '--keep', 'none', success=False)
        bad = tarfile.TarInfo('../outside'); bad.size = 1
        path, sha = self.installer(bad)
        self.run_script('--input', path, '--sha256', sha, '--keep', 'none', success=False)
        self.assertFalse((self.root / 'outside').exists())


if __name__ == '__main__':
    unittest.main()
