# SPDX-License-Identifier: GPL-3.0-or-later
import base64
from contextlib import redirect_stderr, redirect_stdout
import gzip
import io
import json
import os
from pathlib import Path
import struct
import subprocess
import sys
import tarfile
import tempfile
import unittest

import bundle


class BuilderTests(unittest.TestCase):
    def test_command_wrapper(self):
        cases = {'success': ('pass', 0), 'known_failure': ('raise SystemExit(37)', 37)}
        for name, (code, status) in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                stderr, stdout = io.StringIO(), io.StringIO()
                with redirect_stderr(stderr), redirect_stdout(stdout):
                    if status:
                        with self.assertRaises(subprocess.CalledProcessError) as error:
                            bundle.run([sys.executable, '-c', code], cwd=directory, capture=True)
                        self.assertEqual(error.exception.returncode, status)
                    else:
                        self.assertEqual(bundle.run([sys.executable, '-c', code], cwd=directory, capture=True), '')
                self.assertEqual(stdout.getvalue(), '')
                self.assertIn(directory, stderr.getvalue())
                if status:
                    self.assertIn('status 37', stderr.getvalue())
        argument = 'spaces ; $HOME `echo no` "quotes"'
        secret = 'https://user:synthetic-secret@example.invalid/path?token=synthetic-secret'
        stderr = io.StringIO()
        with redirect_stderr(stderr):
            result = bundle.run(
                [sys.executable, '-c', 'import json,sys;print(json.dumps(sys.argv[1:]))', argument, secret],
                capture=True)
        self.assertEqual(json.loads(result), [argument, secret])
        self.assertNotIn('synthetic-secret', stderr.getvalue())
        self.assertIn('<redacted>', stderr.getvalue())

    def test_archive_contract_and_reproducibility(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'root'
            root.mkdir()
            (root / 'bin').mkdir()
            (root / 'bin/tool').write_bytes(b'payload')
            (root / 'bin/tool').chmod(0o755)
            (root / 'data').write_bytes(b'data')
            (root / 'bin/link').symlink_to('tool')
            first, second = Path(directory) / 'first', Path(directory) / 'second'
            first.mkdir()
            second.mkdir()
            a, ma = bundle.package_archive(root, first, 12345)
            # Source mtimes/ownership cannot affect the release identity.
            os.utime(root / 'data', (9999, 9999))
            b, mb = bundle.package_archive(root, second, 12345)
            self.assertEqual(a.read_bytes(), b.read_bytes())
            self.assertEqual(ma, mb)
            raw = gzip.decompress(a.read_bytes())
            self.assertEqual(ma, {'sha256': bundle.sha256(a), 'bytes': a.stat().st_size,
                                  'tar_bytes': len(raw), 'expanded_bytes': 11, 'entries': 4})
            with tarfile.open(a) as archive:
                members = archive.getmembers()
                self.assertEqual([m.name for m in members], ['bin', 'bin/link', 'bin/tool', 'data'])
                self.assertEqual([m.mode for m in members], [0o555, 0o777, 0o555, 0o444])
                self.assertTrue(all(m.uid == m.gid == 0 and m.mtime == 12345 for m in members))

    def test_archive_refuses_reserved_names_and_escape(self):
        cases = {'chrome': None, 'fonts.conf': None, 'prepared.json': None,
                 'escape': '/etc/passwd', 'relative_escape': '../outside', 'dangling': 'missing'}
        for name, target in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root, output = Path(directory) / 'root', Path(directory) / 'output'
                root.mkdir()
                output.mkdir()
                (Path(directory) / 'outside').write_text('outside payload')
                if target:
                    (root / name).symlink_to(target)
                else:
                    (root / name).write_text('forbidden')
                with self.assertRaises(FileNotFoundError if name == 'dangling' else ValueError):
                    bundle.package_archive(root, output, 0)

    def elf(self, root, *, bad_phdr=False, offset=4096, machine=183):
        # Independent ELF64 fixture: LOAD, PHDR and a complete 4096-byte INTERP.
        header = bytearray(64)
        header[:7] = b'\x7fELF\x02\x01\x01'
        struct.pack_into('<H', header, 18, machine)
        struct.pack_into('<Q', header, 32, 64)
        struct.pack_into('<HH', header, 54, 56, 3)
        capacity = 4096
        expected = b'/' + b'N' * (capacity - 2) + b'\0'
        phdrs = [(1, 5, 0, 0, 0, 8192, 8192, 4096),
                 (6, 4, 64, 9000 if bad_phdr else 64, 0, 168, 168, 8),
                 (3, 4, offset, offset, 0, capacity, capacity, 1)]
        data = header + b''.join(struct.pack('<IIQQQQQQ', *p) for p in phdrs)
        data += b'\0' * (4096 - len(data)) + expected
        path = root / 'executable'
        path.write_bytes(data)
        return path, expected

    def test_elf_slot_is_real_program_header(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path, expected = self.elf(root)
            binding = bundle.interpreter_binding(path, root, 183, expected)
            self.assertEqual(binding, {'path': 'executable', 'offset': 4096,
                                       'expected': base64.b64encode(expected).decode()})
            # Linux PATH_MAX permits 4095 path bytes plus terminating NUL.
            self.assertEqual(len(base64.b64decode(binding['expected'])), 4096)
            with self.assertRaisesRegex(ValueError, 'slot mismatch'):
                bundle.interpreter_binding(path, root, 183, b'wrong\0')
            with self.assertRaisesRegex(ValueError, 'architecture'):
                bundle.elf_headers(path, 62)
            self.elf(root, bad_phdr=True)
            with self.assertRaisesRegex(ValueError, 'PHDR not covered'):
                bundle.elf_headers(path, 183)
            self.elf(root, offset=8190)
            with self.assertRaisesRegex(ValueError, 'outside file'):
                bundle.elf_headers(path, 183)


if __name__ == '__main__':
    unittest.main()
