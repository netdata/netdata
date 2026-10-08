#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Tests for check-a53-erratum-843419.py, on synthetic AArch64 ELF images.

import contextlib
import importlib.util
import io
import os
import struct
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / 'check-a53-erratum-843419.py'
spec = importlib.util.spec_from_file_location('check_a53', SCRIPT)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

ADRP_X12 = 0x900067cc        # adrp x12, <page>
STP_X2_X3_SP = 0xa9018fe2    # stp x2, x3, [sp, #24]
LDR_X8_X12_280 = 0xf9408d88  # ldr x8, [x12, #280]
STR_W1_SP = 0xb9002fe1       # str w1, [sp, #44]
STR_X1_SP = 0xf90007e1       # str x1, [sp, #8]
NOP = 0xd503201f
B_SELF = 0x14000000          # b .
LDR_X12_SP = 0xf94003ec      # ldr x12, [sp]   (writes the ADRP register)
LDR_X0_X12 = 0xf9400180      # ldr x0, [x12]

TEXT_ADDR = 0x400000


def code_at(page_offset, words):
    """A 0x2000-byte code blob starting at TEXT_ADDR, with words placed at page_offset of its first page."""
    blob = bytearray(struct.pack('<I', NOP) * (0x2000 // 4))
    struct.pack_into(f'<{len(words)}I', blob, page_offset, *words)
    return bytes(blob)


def make_elf(code, machine=check.EM_AARCH64, e_type=check.ET_DYN, extra_sections=()):
    """Build an ELF64 LE image: null section, .text (executable), extra sections, .shstrtab."""
    names = ['', '.text'] + [n for n, _ in extra_sections] + ['.shstrtab']
    shstrtab = b''
    name_off = []
    for n in names:
        name_off.append(len(shstrtab))
        shstrtab += n.encode() + b'\0'

    body = bytearray(b'\0' * 64)
    text_off = len(body)
    body += code
    extra_offs = []
    for _, content in extra_sections:
        extra_offs.append(len(body))
        body += content
    strtab_off = len(body)
    body += shstrtab
    while len(body) % 8:
        body += b'\0'
    shoff = len(body)

    def sh(name, sh_type, flags, addr, off, size):
        return struct.pack('<IIQQQQIIQQ', name, sh_type, flags, addr, off, size, 0, 0, 1, 0)

    headers = [sh(0, 0, 0, 0, 0, 0),
               sh(name_off[1], 1, 0x2 | check.SHF_EXECINSTR, TEXT_ADDR, text_off, len(code))]
    for n, ((_, content), off) in enumerate(zip(extra_sections, extra_offs)):
        headers.append(sh(name_off[2 + n], 1, 0x2, 0, off, len(content)))
    headers.append(sh(name_off[-1], 3, 0, 0, strtab_off, len(shstrtab)))
    for h in headers:
        body += h

    ident = b'\x7fELF' + bytes([2, 1, 1]) + b'\0' * 9
    struct.pack_into('<16sHHIQQQIHHHHHH', body, 0, ident, e_type, machine, 1, 0, 0, shoff, 0, 64, 0, 0, 64,
                     len(headers), len(headers) - 1)
    return bytes(body)


class SequenceTests(unittest.TestCase):
    def hits(self, page_offset, words):
        return check.scan_code(code_at(page_offset, words), TEXT_ADDR)

    def test_static_aarch64_crash_site(self):
        # The exact words of SQLite pcache1FetchStage2 in the v2.12.0-72 static netdata, ADRP at page offset 0xff8.
        self.assertEqual(self.hits(0xff8, [ADRP_X12, STP_X2_X3_SP, LDR_X8_X12_280, STR_W1_SP]), [TEXT_ADDR + 0xff8])

    def test_three_instruction_sequence_at_0xffc(self):
        self.assertEqual(self.hits(0xffc, [ADRP_X12, STR_X1_SP, LDR_X0_X12]), [TEXT_ADDR + 0xffc])

    def test_four_instruction_sequence_at_0xffc(self):
        self.assertEqual(self.hits(0xffc, [ADRP_X12, STR_X1_SP, NOP, LDR_X0_X12]), [TEXT_ADDR + 0xffc])

    def test_sequence_away_from_page_end(self):
        self.assertEqual(self.hits(0xff0, [ADRP_X12, STR_X1_SP, LDR_X0_X12]), [])

    def test_branch_in_optional_slot(self):
        self.assertEqual(self.hits(0xff8, [ADRP_X12, STR_X1_SP, B_SELF, LDR_X0_X12]), [])

    def test_second_instruction_overwrites_adrp_register(self):
        self.assertEqual(self.hits(0xff8, [ADRP_X12, LDR_X12_SP, LDR_X0_X12]), [])

    def test_section_ending_mid_instruction(self):
        # 13 bytes from the ADRP to the end of the code: no room for a fourth instruction, and no crash.
        code = code_at(0xff8, [ADRP_X12, STR_X1_SP, NOP])[:0xff8 + 13]
        self.assertEqual(check.scan_code(code, TEXT_ADDR), [])

    def test_final_load_uses_another_base(self):
        self.assertEqual(self.hits(0xff8, [ADRP_X12, STR_X1_SP, STR_X1_SP]), [])


class ElfTests(unittest.TestCase):
    def run_dir(self, files):
        with tempfile.TemporaryDirectory() as d:
            for name, data in files.items():
                Path(d, name).write_bytes(data)
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                rc = check.main(['check', d])
            return rc, out.getvalue()

    BAD = code_at(0xff8, [ADRP_X12, STP_X2_X3_SP, LDR_X8_X12_280])
    GOOD = code_at(0xff0, [ADRP_X12, STP_X2_X3_SP, LDR_X8_X12_280])

    def test_clean_binary_passes(self):
        rc, out = self.run_dir({'prog': make_elf(self.GOOD)})
        self.assertEqual(rc, 0, out)

    def test_exposed_binary_fails(self):
        rc, out = self.run_dir({'prog': make_elf(self.BAD), 'ok': make_elf(self.GOOD)})
        self.assertEqual(rc, 1)
        self.assertIn('ERROR: prog: 1 sequence(s)', out)

    def test_go_binary_is_reported_but_passes(self):
        go = make_elf(self.BAD, extra_sections=[('.go.buildinfo', b'\xff Go buildinf:')])
        rc, out = self.run_dir({'go-prog': go})
        self.assertEqual(rc, 0, out)
        self.assertIn('NOTICE: go-prog', out)

    def test_go_build_id_note_alone_is_not_go(self):
        not_go = make_elf(self.BAD, extra_sections=[('.note.go.buildid', b'\0' * 16)])
        rc, out = self.run_dir({'prog': not_go})
        self.assertEqual(rc, 1)
        self.assertIn('ERROR: prog: 1 sequence(s)', out)

    @unittest.skipIf(os.geteuid() == 0, 'root can read any directory')
    def test_unreadable_directory_fails(self):
        with tempfile.TemporaryDirectory() as d:
            Path(d, 'prog').write_bytes(make_elf(self.GOOD))
            hidden = Path(d, 'hidden')
            hidden.mkdir()
            Path(hidden, 'prog').write_bytes(make_elf(self.BAD))
            hidden.chmod(0)
            try:
                out = io.StringIO()
                with contextlib.redirect_stdout(out):
                    rc = check.main(['check', d])
            finally:
                hidden.chmod(0o700)
        self.assertEqual(rc, 1, out.getvalue())
        self.assertIn('ERROR: hidden:', out.getvalue())

    def test_other_machines_and_object_files_are_skipped(self):
        rc, out = self.run_dir({
            'prog': make_elf(self.GOOD),
            'x86': make_elf(self.BAD, machine=62),
            'obj.o': make_elf(self.BAD, e_type=1),
            'text': b'not an ELF file',
        })
        self.assertEqual(rc, 0, out)
        self.assertIn('checked 1 AArch64', out)

    def test_broken_elf_fails(self):
        data = bytearray(make_elf(self.GOOD))
        struct.pack_into('<Q', data, 0x28, len(data) + 4096)  # section header table past the end of the file
        rc, out = self.run_dir({'prog': bytes(data)})
        self.assertEqual(rc, 1)
        self.assertIn('ERROR: prog: invalid section header table', out)

    def test_truncated_elf_does_not_crash(self):
        # Too short to say what it is: skipped. Claims ELF64 LE but is truncated: reported as broken.
        rc, out = self.run_dir({'prog': make_elf(self.GOOD), 'short': b'\x7fELF\x02'})
        self.assertEqual(rc, 0, out)
        rc, out = self.run_dir({'prog': make_elf(self.GOOD), 'broken': b'\x7fELF\x02\x01'})
        self.assertEqual(rc, 1)
        self.assertIn('ERROR: broken:', out)

    def test_no_aarch64_binaries_fails(self):
        rc, out = self.run_dir({'text': b'nothing to see'})
        self.assertEqual(rc, 1)


if __name__ == '__main__':
    unittest.main()
