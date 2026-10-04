#!/usr/bin/env python3
"""Exercise fleet preparation with offline fixtures; never execute an installer."""
import errno
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import runpy
import shutil
import struct
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / 'packaging/makeself/prepare-fleet.py'


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
        for plugin in ('apps.plugin', 'network-viewer.plugin', 'go.d.plugin', 'local-listeners',
                       'snmp-trap-profile-gen', 'ndsudo', 'loopsleepms.sh.inc'):
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

    def elf_fixture(self, name, wide=True, byte_order='<'):
        # Linux package fixtures must also work on non-ELF build hosts.
        path = self.put(name, '')
        names = b'\0.text\0.shstrtab\0'
        header_size, section_size, section_offset = (64, 64, 88) if wide else (52, 40, 76)
        ident = b'\x7fELF' + bytes((2 if wide else 1, 1 if byte_order == '<' else 2, 1)) + bytes(9)
        header = struct.pack(byte_order + ('16sHHIQQQIHHHHHH' if wide else '16sHHIIIIIHHHHHH'), ident,
                             2, 62 if wide else 40, 1, 0x1000, 0, section_offset, 0,
                             header_size, 0, 0, section_size, 3, 2)
        data = header + b'\xc3' + names
        data += bytes(section_offset - len(data))
        section_format = byte_order + ('IIQQQQIIQQ' if wide else 'IIIIIIIIII')
        data += bytes(section_size)
        data += struct.pack(section_format, 1, 1, 6, 0x1000, header_size, 1, 0, 0, 1, 0)
        data += struct.pack(section_format, 7, 3, 0, 0, header_size + 1, len(names), 0, 0, 1, 0)
        path.write_bytes(data)
        return path

    def require_elf_strip(self):
        tool = shutil.which('llvm-strip') or shutil.which('strip')
        if not tool:
            self.skipTest('ELF-compatible strip unavailable')
        source = self.elf_fixture(str(self.root / 'strip-probe'))
        result = subprocess.run([tool, '--strip-debug', '-o', str(self.root / 'strip-probe-output'), str(source)],
                                capture_output=True)
        if result.returncode:
            self.skipTest('available strip does not support the Linux ELF fixture')
        return tool

    def run_script(self, *args, success=True, env=None):
        result = subprocess.run([str(SCRIPT), *map(str, args)], cwd=self.root, capture_output=True, text=True, env=env)
        self.assertEqual(result.returncode == 0, success, result.stderr)
        return result

    def prepare(self, keep='none', **options):
        target = self.root / 'output'
        args = ['--source', self.source, '--keep', keep, '--strip-mode', options.get('strip', 'none'),
                '--apply', '--output', target]
        if 'strip_tool' in options:
            args += ['--strip-tool', options['strip_tool']]
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

    def test_all_rejects_missing_declared_dependency(self):
        other = self.root / 'incomplete-network'
        shutil.copytree(self.source, other, symlinks=True,
                        ignore=lambda directory, names: ['apps.plugin'])
        target = self.root / 'incomplete-network-output'
        result = self.run_script('--source', other, '--keep', 'all', '--strip-mode', 'none',
                                 '--apply', '--output', target, success=False)
        self.assertIn('incomplete capability network: missing dependency apps', result.stderr)
        self.assertFalse(target.exists())
        self.assertTrue((other / 'usr/libexec/netdata/plugins.d/network-viewer.plugin').exists())

    def test_netflow_downloader_follows_capability(self):
        paths = ('usr/libexec/netdata/plugins.d/netflow-plugin',
                 'usr/sbin/topology-ip-intel-downloader')
        for path in paths:
            self.put(path, 'netflow helper')
        target, _ = self.prepare()
        for path in paths:
            self.assertFalse((target / path).exists())
        self.run_script('--source', self.source, '--keep', 'netflow',
                        '--strip-mode', 'none', '--apply', '--output', self.root / 'netflow')
        for path in paths:
            self.assertTrue((self.root / 'netflow' / path).is_file())

    def test_ebpf_stock_bundle_follows_capability(self):
        paths = ('usr/libexec/netdata/plugins.d/ebpf.plugin',
                 'usr/libexec/netdata/plugins.d/ebpf-go.plugin',
                 'usr/libexec/netdata/plugins.d/ebpf.d/pnetdata_ebpf_socket.5.4.o',
                 'usr/lib/netdata/conf.d/ebpf.d.conf',
                 'usr/lib/netdata/conf.d/ebpf.d/socket.conf')
        for path in paths:
            self.put(path, 'ebpf stock')
        legacy_object = self.elf_fixture(paths[2])
        self.put('etc/netdata/ebpf.d/socket.conf', 'custom user configuration')
        target, _ = self.prepare()
        for path in paths:
            self.assertFalse((target / path).exists())
        self.assertTrue((target / 'etc/netdata/ebpf.d/socket.conf').is_file())
        self.run_script('--source', self.source, '--keep', 'ebpf', '--strip-mode', 'all',
                        '--apply', '--output', self.root / 'ebpf')
        for path in paths:
            self.assertTrue((self.root / 'ebpf' / path).is_file())
        self.assertEqual((self.root / 'ebpf' / paths[2]).read_bytes(), legacy_object.read_bytes())

    def test_retained_netflow_downloader_is_stripped(self):
        self.put('usr/libexec/netdata/plugins.d/netflow-plugin', 'plugin')
        downloader = self.elf_fixture('usr/sbin/topology-ip-intel-downloader')
        downloader.chmod(0o755)
        target, _ = self.prepare('netflow', strip='debug', strip_tool=self.require_elf_strip())
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertIn('usr/sbin/topology-ip-intel-downloader',
                      [item['path'] for item in manifest['stripped_files']])
        self.assertEqual((target / 'usr/sbin/topology-ip-intel-downloader').stat().st_mode & 0o7777, 0o755)

    def test_preview_does_not_publish(self):
        result = self.run_script('--source', self.source, '--keep', 'none', '--output', self.root / 'preview')
        self.assertIn('Preview only', result.stderr)
        self.assertFalse((self.root / 'preview').exists())
        self.assertIn('Keep: none', result.stdout)
        self.assertNotIn('removed_paths', result.stdout)

    def test_import_does_not_execute_cli(self):
        spec = importlib.util.spec_from_file_location('prepare_fleet', SCRIPT)
        module = importlib.util.module_from_spec(spec)
        with patch('sys.argv', ['unrelated-program', '--invalid']), patch('builtins.print') as output:
            spec.loader.exec_module(module)
        output.assert_not_called()
        self.assertTrue(callable(module.main))

    def test_windows_cli_fails_with_platform_message(self):
        with patch.object(os, 'name', 'nt'):
            with self.assertRaisesRegex(RuntimeError, 'Windows is not supported'):
                runpy.run_path(str(SCRIPT), run_name='__main__')

    @unittest.skipUnless(os.name == 'posix' and os.geteuid() != 0, 'requires unprivileged POSIX permissions')
    def test_unreadable_source_subtree_refused(self):
        directory = self.put('usr/share/netdata/private-source/config', 'must survive').parent
        directory.chmod(0)
        try:
            try:
                with os.scandir(directory) as entries:
                    list(entries)
            except PermissionError:
                pass
            else:
                self.skipTest('process can read mode-000 directories')
            target, result = self.prepare(success=False)
            self.assertFalse(target.exists())
            self.assertIn('Permission denied', result.stderr)
        finally:
            directory.chmod(0o755)

    def test_default_output_summarizes_large_inventory(self):
        for index in range(100):
            self.put(f'usr/share/netdata/web/asset-{index}.js', 'dashboard asset')
        result = self.run_script('--source', self.source, '--keep', 'network')
        self.assertIn('Keep: apps, network', result.stdout)
        self.assertIn('Original payload:', result.stdout)
        self.assertIn('Payload before stripping:', result.stdout)
        self.assertNotIn('asset-99.js', result.stdout + result.stderr)
        self.assertLess(len(result.stdout.splitlines()), 15)

    def test_verbose_retains_detailed_removal_report(self):
        result = self.run_script('--source', self.source, '--keep', 'none', '--verbose')
        self.assertIn('"removed_paths":', result.stdout)
        self.assertIn('usr/share/netdata/web/index.html', result.stdout)

    def test_unknown_or_missing_capability_fails(self):
        for name in ('unrecognized', 'journal', 'apps,', 'none,apps', 'all,apps', ' , '):
            with self.subTest(keep=name):
                self.run_script('--source', self.source, '--keep', name, success=False)

    def test_cli_validation_errors_are_concise(self):
        path, _ = self.installer()
        cases = (
            (['--source', self.source, '--keep', 'unknown'], 'unknown capability'),
            (['--input', path, '--sha256', '0' * 64, '--keep', 'none'], 'installer SHA-256 does not match'),
            (['--source', self.source, '--keep', 'none', '--apply', '--output', self.source / 'nested'],
             'source and output must not overlap'),
        )
        for args, reason in cases:
            with self.subTest(reason=reason):
                result = self.run_script(*args, success=False)
                self.assertEqual(result.returncode, 1)
                self.assertIn('ERROR: ' + reason, result.stderr)
                self.assertNotIn('Traceback', result.stderr)
        result = self.run_script('--strip-mode', 'invalid', success=False)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn('Traceback', result.stderr)

    def test_capability_list_accepts_whitespace(self):
        self.put('usr/libexec/netdata/plugins.d/debugfs.plugin', 'debugfs')
        target, _ = self.prepare(' apps, debugfs ')
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertEqual(manifest['requested'], ['apps', 'debugfs'])
        self.assertEqual(manifest['resolved'], ['apps', 'debugfs'])
        for name in ('apps.plugin', 'debugfs.plugin'):
            self.assertTrue((target / 'usr/libexec/netdata/plugins.d' / name).is_file())

    def test_capability_sentinels_accept_whitespace(self):
        for name in ('all', 'none'):
            with self.subTest(keep=name):
                plain = self.run_script('--source', self.source, '--keep', name)
                padded = self.run_script('--source', self.source, '--keep', ' ' + name + ' ')
                self.assertEqual(padded.stdout, plain.stdout)
                self.assertIn('Preview only.', padded.stderr)

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
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertIn('network', manifest['resolved'])
        self.assertIn('apps', manifest['resolved'])

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

    def test_successful_default_debug_strip_preserves_executable_mode(self):
        self.elf_fixture('bin/srv/netdata')
        (self.source / 'bin/srv/netdata').chmod(0o755)
        self.elf_fixture('bin/nd-run')
        target = self.root / 'debug-output'
        result = self.run_script('--source', self.source, '--keep', 'apps', '--apply', '--output', target,
                                 '--strip-tool', self.require_elf_strip())
        self.assertIn('Stripped bin/srv/netdata:', result.stdout)
        self.assertIn('Prepared payload:', result.stdout)
        self.assertIn('Disk reduction:', result.stdout)
        binary = target / 'bin/srv/netdata'
        self.assertEqual(binary.stat().st_mode & 0o7777, 0o755)
        manifest = json.loads((target / 'usr/share/netdata/fleet-manifest.json').read_text())
        self.assertEqual(manifest['strip_mode'], 'debug')
        self.assertEqual(len(manifest['stripped_files']), 2)

    @unittest.skipUnless(shutil.which('cc'), 'native compiler unavailable')
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
        self.elf_fixture('bin/srv/netdata')
        tool = self.root / 'fail-strip'
        tool.write_text('#!/bin/sh\nexit 1\n')
        tool.chmod(0o755)
        target, result = self.prepare(strip='debug', strip_tool=tool, success=False)
        self.assertFalse(target.exists())
        self.assertIn('command failed (1)', result.stderr)

    def test_allocated_section_mutation_refused(self):
        self.elf_fixture('bin/srv/netdata')
        tool = self.root / 'bad-strip'
        tool.write_text('#!/usr/bin/env python3\n'
                        'import pathlib, sys\n'
                        'p = pathlib.Path(sys.argv[-1])\n'
                        'd = bytearray(p.read_bytes())\n'
                        'd[24] ^= 1\n'
                        'pathlib.Path(sys.argv[-2]).write_bytes(d)\n')
        tool.chmod(0o755)
        target, result = self.prepare(strip='all', strip_tool=tool, success=False)
        self.assertFalse(target.exists())
        self.assertIn('changed allocated sections or ELF identity', result.stderr)

    def test_elf_contract_accepts_both_classes_and_byte_orders(self):
        contract = runpy.run_path(str(SCRIPT))['elf_contract']
        for wide in (False, True):
            for byte_order in ('<', '>'):
                for variant in ('named', 'nameless', 'nobits'):
                    with self.subTest(wide=wide, byte_order=byte_order, variant=variant):
                        path = self.elf_fixture('bin/srv/netdata', wide, byte_order)
                        data = bytearray(path.read_bytes())
                        header_size, section_size, section_offset = (64, 64, 88) if wide else (52, 40, 76)
                        name, section_type, contents = b'.text', 1, b'\xc3'
                        if variant == 'nameless':
                            struct.pack_into(byte_order + 'H', data, header_size - 2, 0)
                            struct.pack_into(byte_order + 'I', data, section_offset + section_size, 0)
                            name = b''
                        elif variant == 'nobits':
                            struct.pack_into(byte_order + 'I', data, section_offset + section_size + 4, 8)
                            struct.pack_into(byte_order + ('Q' if wide else 'I'), data,
                                             section_offset + section_size + (24 if wide else 16), len(data) + 1)
                            section_type, contents = 8, b''
                        path.write_bytes(data)
                        expected = (bytes((2 if wide else 1, 1 if byte_order == '<' else 2)),
                                    (2, 62 if wide else 40, 1, 0x1000),
                                    [(name, section_type, 6, 0x1000, 1, hashlib.sha256(contents).hexdigest())])
                        self.assertEqual(contract(path), expected)

    def test_elf_table_ranges_are_validated(self):
        contract = runpy.run_path(str(SCRIPT))['elf_contract']
        for wide in (False, True):
            for byte_order in ('<', '>'):
                for variant in ('section_start', 'section_end', 'names_start', 'names_end'):
                    with self.subTest(wide=wide, byte_order=byte_order, variant=variant):
                        path = self.elf_fixture('bin/srv/netdata', wide, byte_order)
                        data = bytearray(path.read_bytes())
                        word_format = byte_order + ('Q' if wide else 'I')
                        section_size, section_offset = (64, 88) if wide else (40, 76)
                        if variant == 'section_start':
                            struct.pack_into(word_format, data, 40 if wide else 32, len(data) + 1)
                        elif variant == 'section_end':
                            data = data[:-1]
                        else:
                            field_offset = (24 if wide else 16) if variant == 'names_start' else (32 if wide else 20)
                            struct.pack_into(word_format, data, section_offset + 2 * section_size + field_offset,
                                             len(data) + 1)
                        path.write_bytes(data)
                        message = 'ELF section table' if variant.startswith('section') else 'ELF string table'
                        with self.assertRaisesRegex(ValueError, message):
                            contract(path)

    def test_elf_allocated_section_names_are_validated(self):
        contract = runpy.run_path(str(SCRIPT))['elf_contract']
        for wide in (False, True):
            for byte_order in ('<', '>'):
                for variant in ('out_of_range', 'unterminated'):
                    with self.subTest(wide=wide, byte_order=byte_order, variant=variant):
                        path = self.elf_fixture('bin/srv/netdata', wide, byte_order)
                        data = bytearray(path.read_bytes())
                        header_size, section_size, section_offset = (64, 64, 88) if wide else (52, 40, 76)
                        if variant == 'out_of_range':
                            struct.pack_into(byte_order + 'I', data, section_offset + section_size, 999)
                        else:
                            data[header_size + 2:header_size + 18] = b'x' * 16
                        path.write_bytes(data)
                        with self.assertRaisesRegex(ValueError, 'ELF section name'):
                            contract(path)

    def tree_publication_fixture(self, suffix=''):
        stage = self.root / ('tree-stage' + suffix)
        directory = stage / 'readonly'
        directory.mkdir(parents=True)
        for name in ('first', 'second'):
            (directory / name).write_text(name)
        outside = self.root / ('outside' + suffix)
        outside.mkdir()
        (outside / 'sentinel').write_text('preserve linked target')
        outside.chmod(0o555)
        os.symlink('../../' + outside.name, directory / 'external')
        member = tarfile.TarInfo('readonly')
        member.type, member.mode = tarfile.DIRTYPE, 0o555
        return stage, {'readonly': member}, self.root / ('tree-output' + suffix), outside

    def fail_second_tree_copy(self):
        original = shutil.copy2
        count = 0

        def failing_copy(*args, **kwargs):
            nonlocal count
            count += 1
            if count == 2:
                raise OSError(errno.ENOSPC, 'tree copy disk full')
            return original(*args, **kwargs)

        return failing_copy

    def test_failed_tree_copy_rolls_back_readonly_output(self):
        publish = runpy.run_path(str(SCRIPT))['publish_tree']
        stage, kept, target, outside = self.tree_publication_fixture()
        with patch('shutil.copy2', side_effect=self.fail_second_tree_copy()):
            with self.assertRaisesRegex(OSError, 'tree copy disk full'):
                publish(stage, kept, target)
        self.assertFalse(target.exists())
        for name in ('first', 'second'):
            self.assertEqual((stage / 'readonly' / name).read_text(), name)
        self.assertEqual((stage / 'readonly').stat().st_mode & 0o777, 0o555)
        self.assertEqual(outside.stat().st_mode & 0o777, 0o555)
        self.assertEqual((outside / 'sentinel').read_text(), 'preserve linked target')

    def test_existing_tree_output_survives_reservation_failure(self):
        publish = runpy.run_path(str(SCRIPT))['publish_tree']
        stage, kept, target, _ = self.tree_publication_fixture()
        target.mkdir()
        (target / 'sentinel').write_text('competing tree')
        with patch('shutil.rmtree') as cleanup, patch('shutil.copytree') as copying:
            with self.assertRaises(FileExistsError):
                publish(stage, kept, target)
        cleanup.assert_not_called()
        copying.assert_not_called()
        self.assertEqual((target / 'sentinel').read_text(), 'competing tree')

    def test_tree_copy_failure_preserves_replaced_output(self):
        publish = runpy.run_path(str(SCRIPT))['publish_tree']
        for variant in ('directory', 'symlink', 'missing'):
            with self.subTest(variant=variant):
                stage, kept, target, _ = self.tree_publication_fixture('-' + variant)
                original = self.root / ('interrupted-tree-' + variant)

                def interrupted_copy(*args, **kwargs):
                    target.rename(original)
                    (original / 'partial').write_text('this publication')
                    if variant == 'directory':
                        target.mkdir()
                        (target / 'sentinel').write_text('competing tree')
                    elif variant == 'symlink':
                        os.symlink(original.name, target)
                    raise OSError('tree copy interrupted')

                with patch('shutil.copytree', side_effect=interrupted_copy):
                    with self.assertRaisesRegex(OSError, 'tree copy interrupted'):
                        publish(stage, kept, target)
                self.assertEqual((original / 'partial').read_text(), 'this publication')
                if variant == 'directory':
                    self.assertEqual((target / 'sentinel').read_text(), 'competing tree')
                elif variant == 'symlink':
                    self.assertEqual(os.readlink(target), original.name)
                else:
                    self.assertFalse(target.exists())

    def test_tree_cleanup_failure_preserves_original_error(self):
        publish = runpy.run_path(str(SCRIPT))['publish_tree']
        stage, kept, target, _ = self.tree_publication_fixture()
        with patch('shutil.copy2', side_effect=self.fail_second_tree_copy()):
            with patch('shutil.rmtree', side_effect=PermissionError('tree rollback denied')):
                with self.assertRaisesRegex(OSError, 'tree copy disk full') as raised:
                    publish(stage, kept, target)
        self.assertIsInstance(raised.exception.__cause__, PermissionError)
        self.assertEqual(str(raised.exception.__cause__), 'tree rollback denied')
        self.assertTrue(target.is_dir())

    def test_successful_tree_publish_retains_readonly_modes_and_links(self):
        publish = runpy.run_path(str(SCRIPT))['publish_tree']
        stage, kept, target, outside = self.tree_publication_fixture()
        publish(stage, kept, target)
        self.assertEqual((target / 'readonly').stat().st_mode & 0o777, 0o555)
        self.assertEqual(os.readlink(target / 'readonly/external'), '../../' + outside.name)
        for name in ('first', 'second'):
            self.assertEqual((target / 'readonly' / name).read_text(), name)

    def test_concurrent_installer_publication_leaves_no_checksum(self):
        path, sha = self.installer()
        target = self.root / 'racing.gz.run'
        tools = self.root / 'tools'
        tools.mkdir()
        cksum = tools / 'cksum'
        cksum.write_text('#!/usr/bin/env python3\nimport os,pathlib\n'
                         'pathlib.Path(os.environ["FLEET_RACE_OUTPUT"]).write_text("other publisher")\n'
                         'print("1 1")\n')
        cksum.chmod(0o755)
        env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'],
                   FLEET_RACE_OUTPUT=str(target))
        self.run_script('--input', path, '--sha256', sha, '--keep', 'none', '--strip-mode', 'none',
                        '--apply', '--output', target, success=False, env=env)
        self.assertEqual(target.read_text(), 'other publisher')
        self.assertFalse(Path(str(target) + '.sha256').exists())

    def checksum_publication_race(self, replace_installer=False):
        path, sha = self.installer()
        target = self.root / 'checksum-race.gz.run'
        injection = self.root / 'injection'
        injection.mkdir()
        (injection / 'sitecustomize.py').write_text(
            'import os, pathlib\n'
            'original = os.link\n'
            'def race(source, target, *args, **kwargs):\n'
            '    if pathlib.Path(source).name == "installer.sha256":\n'
            '        pathlib.Path(target).write_text("competing checksum")\n'
            '        if os.environ.get("FLEET_REPLACE_INSTALLER") == "yes":\n'
            '            installer = pathlib.Path(str(target).removesuffix(".sha256"))\n'
            '            installer.unlink()\n'
            '            installer.write_text("competing installer")\n'
            '    return original(source, target, *args, **kwargs)\n'
            'os.link = race\n')
        env = dict(os.environ, PYTHONPATH=str(injection),
                   FLEET_REPLACE_INSTALLER='yes' if replace_installer else 'no')
        result = self.run_script('--input', path, '--sha256', sha, '--keep', 'none',
                                 '--strip-mode', 'none', '--apply', '--output', target,
                                 success=False, env=env)
        self.assertIn('File exists', result.stderr)
        self.assertEqual(Path(str(target) + '.sha256').read_text(), 'competing checksum')
        return target

    def test_checksum_publication_failure_rolls_back_own_installer(self):
        target = self.checksum_publication_race()
        self.assertFalse(target.exists())

    def test_checksum_publication_failure_preserves_replaced_installer(self):
        target = self.checksum_publication_race(replace_installer=True)
        self.assertEqual(target.read_text(), 'competing installer')

    def publication_failure(self, injection_code):
        path, sha = self.installer()
        target = self.root / 'failure.gz.run'
        injection = self.root / 'injection'
        injection.mkdir()
        (injection / 'sitecustomize.py').write_text(injection_code)
        result = self.run_script('--input', path, '--sha256', sha, '--keep', 'none',
                                 '--strip-mode', 'none', '--apply', '--output', target,
                                 success=False, env=dict(os.environ, PYTHONPATH=str(injection)))
        return target, result

    def test_checksum_logging_failure_preserves_published_pair(self):
        target, result = self.publication_failure(
            'import builtins\n'
            'original = builtins.print\n'
            'def faulty(*args, **kwargs):\n'
            '    if args and str(args[0]).startswith("Published ") and str(args[0]).endswith(".sha256"):\n'
            '        raise OSError("checksum logging failed")\n'
            '    return original(*args, **kwargs)\n'
            'builtins.print = faulty\n')
        self.assertIn('checksum logging failed', result.stderr)
        self.assertTrue(target.exists())
        self.assertTrue(Path(str(target) + '.sha256').exists())

    def test_rollback_failure_preserves_original_error(self):
        target, result = self.publication_failure(
            'import os, pathlib\n'
            'link = os.link\n'
            'unlink = pathlib.Path.unlink\n'
            'def faulty_link(source, target, *args, **kwargs):\n'
            '    if pathlib.Path(source).name == "installer.sha256":\n'
            '        raise PermissionError("checksum publication denied")\n'
            '    return link(source, target, *args, **kwargs)\n'
            'def faulty_unlink(self, *args, **kwargs):\n'
            '    if self.name == "failure.gz.run":\n'
            '        raise PermissionError("rollback denied")\n'
            '    return unlink(self, *args, **kwargs)\n'
            'os.link = faulty_link\n'
            'pathlib.Path.unlink = faulty_unlink\n')
        self.assertIn('checksum publication denied', result.stderr.splitlines()[-1])
        self.assertIn('rollback denied', result.stderr)
        self.assertTrue(target.exists())
        self.assertFalse(Path(str(target) + '.sha256').exists())

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
                  'eval "gzip -cd"\necho Uncompressed size: 999 KB\n'
                  'MS_Printf "About to extract 999 KB in $tmpdir (999 KB)"\n'
                  'if test "$leftspace" -lt 999; then\n').replace('SIZE', str(len(payload)))
        path = self.root / 'input.gz.run'
        path.write_bytes(header.encode() + payload)
        return path, hashlib.sha256(path.read_bytes()).hexdigest()

    def test_archive_rebuilt_metadata_checksums_and_space(self):
        path, sha = self.installer()
        target = self.root / 'reduced.gz.run'
        self.run_script('--input', path, '--sha256', sha, '--keep', 'apps',
                        '--strip-mode', 'none', '--apply', '--output', target)
        content = target.read_bytes()
        header = b'\n'.join(content.split(b'\n', 15)[:15]) + b'\n'
        payload = content[len(header):]
        raw = gzip.decompress(payload)
        checksum = subprocess.check_output(['cksum'], input=payload).split()[0]
        self.assertIn(b'CRCsum="' + checksum + b'"', header)
        self.assertIn(b'MD5="' + hashlib.md5(payload).hexdigest().encode() + b'"', header)
        self.assertIn(b'SHA="' + hashlib.sha256(payload).hexdigest().encode() + b'"', header)
        self.assertNotIn(b'999 KB', header)
        self.assertNotIn(b'-lt 999;', header)
        self.assertIn(str((len(raw) + 1023) // 1024).encode() + b' KB', header)
        self.assertTrue(Path(str(target) + '.sha256').exists())
        with tarfile.open(fileobj=io.BytesIO(raw)) as tf:
            self.assertIn('./bin/srv/netdata', tf.getnames())
            self.assertNotIn('./usr/libexec/netdata/plugins.d/go.d.plugin', tf.getnames())
            self.assertEqual(tf.getmember('./bin/srv/netdata').mode,
                             (self.source / 'bin/srv/netdata').stat().st_mode & 0o7777)

    def test_archive_with_implicit_directories_keeps_directory_capabilities(self):
        path, sha = self.installer(omit_directories=True)
        result = self.run_script('--input', path, '--sha256', sha, '--keep', 'dashboard,go')
        self.assertIn('Keep: dashboard, go', result.stdout)

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
        bad = tarfile.TarInfo('../outside')
        bad.size = 1
        path, sha = self.installer(bad)
        self.run_script('--input', path, '--sha256', sha, '--keep', 'none', success=False)
        self.assertFalse((self.root / 'outside').exists())


if __name__ == '__main__':
    unittest.main()
