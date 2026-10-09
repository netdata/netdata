#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Fail if any AArch64 executable or shared object under a directory still contains a Cortex-A53 erratum 843419
# instruction sequence, i.e. it was linked without --fix-cortex-a53-843419.
#
# Static aarch64 builds run on Cortex-A53 boards (Raspberry Pi 3, ODROID-C2, ...). There an ADRP at page offset
# 0xff8/0xffc followed by a load or store can use an address one page off, so a function pointer is loaded from the
# wrong place and the agent dies at startup. Neither the CI runners' ARM cores nor QEMU exhibit the erratum, so the
# runtime checks of the static build cannot catch a missing workaround; this scan can.
#
# The sequence rules are a port of lld's scanner (llvm/llvm-project lld/ELF/AArch64ErrataFix.cpp), which follows
# the Cortex-A53 Software Developers Errata Notice (ARM-EPM-048406). ld.bfd, which links the static build, patches a
# superset of these sequences, so a correctly linked binary has none left.
#
# Go binaries are linked by the Go linker, which has no workaround for this erratum. They are reported but do not fail
# the check.
#
# Usage: check-a53-erratum-843419.py <directory>

import os
import struct
import sys

EM_AARCH64 = 183
ET_EXEC = 2
ET_DYN = 3
SHF_EXECINSTR = 0x4


class ElfError(Exception):
    pass


def is_adrp(i):
    return (i & 0x9f000000) == 0x90000000


def is_load_store_class(i):
    return (i & 0x0a000000) == 0x08000000


def is_st1_multiple_opcode(i):
    return (i & 0x0000f000) in (0x00002000, 0x00006000, 0x00007000, 0x0000a000)


def is_st1_multiple(i):
    return (i & 0xbfff0000) == 0x0c000000 and is_st1_multiple_opcode(i)


def is_st1_multiple_post(i):
    return (i & 0xbfe00000) == 0x0c800000 and is_st1_multiple_opcode(i)


def is_st1_single_opcode(i):
    return (i & 0x0040e000) in (0x00000000, 0x00004000, 0x00008000)


def is_st1_single(i):
    return (i & 0xbfff0000) == 0x0d000000 and is_st1_single_opcode(i)


def is_st1_single_post(i):
    return (i & 0xbfe00000) == 0x0d800000 and is_st1_single_opcode(i)


def is_st1(i):
    return is_st1_multiple(i) or is_st1_multiple_post(i) or is_st1_single(i) or is_st1_single_post(i)


def is_load_store_exclusive(i):
    return (i & 0x3f000000) == 0x08000000


def is_load_exclusive(i):
    return (i & 0x3f400000) == 0x08400000


def is_load_literal(i):
    return (i & 0x3b000000) == 0x18000000


def is_stnp(i):
    return (i & 0x3bc00000) == 0x28000000


def is_stp_post(i):
    return (i & 0x3bc00000) == 0x28800000


def is_stp_offset(i):
    return (i & 0x3bc00000) == 0x29000000


def is_stp_pre(i):
    return (i & 0x3bc00000) == 0x29800000


def is_stp(i):
    return is_stp_post(i) or is_stp_offset(i) or is_stp_pre(i)


def is_load_store_unscaled(i):
    return (i & 0x3b000c00) == 0x38000000


def is_load_store_immediate_post(i):
    return (i & 0x3b200c00) == 0x38000400


def is_load_store_unpriv(i):
    return (i & 0x3b200c00) == 0x38000800


def is_load_store_immediate_pre(i):
    return (i & 0x3b200c00) == 0x38000c00


def is_load_store_register_off(i):
    return (i & 0x3b200c00) == 0x38200800


def is_load_store_register_unsigned(i):
    return (i & 0x3b000000) == 0x39000000


def get_rt(i):
    return i & 0x1f


def get_rn(i):
    return (i >> 5) & 0x1f


def is_branch(i):
    return ((i & 0xfe000000) == 0xd6000000 or  # unconditional branch (register)
            (i & 0xfe000000) == 0x54000000 or  # conditional branch
            (i & 0x7c000000) == 0x14000000 or  # unconditional branch (immediate)
            (i & 0x7c000000) == 0x34000000)    # compare/test and branch


def is_v8_single_register_non_structure_load_store(i):
    return (is_load_store_unscaled(i) or is_load_store_immediate_post(i) or is_load_store_unpriv(i) or
            is_load_store_immediate_pre(i) or is_load_store_register_off(i) or is_load_store_register_unsigned(i))


def is_v8_non_structure_load(i):
    if is_load_exclusive(i) or is_load_literal(i):
        return True
    if is_v8_single_register_non_structure_load_store(i):
        size = (i >> 30) & 0x3
        v = (i >> 26) & 0x1
        opc = (i >> 22) & 0x3
        return opc != 0 and not (size == 0 and v == 1 and opc == 2) and not (size == 3 and v == 0 and opc == 2)
    return False


def has_writeback(i):
    return (is_load_store_immediate_pre(i) or is_load_store_immediate_post(i) or is_stp_pre(i) or is_stp_post(i) or
            is_st1_single_post(i) or is_st1_multiple_post(i))


def does_load_store_write_to_reg(i, reg):
    return (is_v8_non_structure_load(i) and get_rt(i) == reg) or (has_writeback(i) and get_rn(i) == reg)


def is_erratum_sequence(i1, i2, i4):
    if not is_adrp(i1):
        return False
    rn = get_rt(i1)
    return (is_load_store_class(i2) and
            (is_load_store_exclusive(i2) or is_load_literal(i2) or
             is_v8_single_register_non_structure_load_store(i2) or is_stp(i2) or is_stnp(i2) or is_st1(i2)) and
            not does_load_store_write_to_reg(i2, rn) and
            is_load_store_register_unsigned(i4) and get_rn(i4) == rn)


def scan_code(code, addr):
    """Return the addresses of the ADRPs that start an erratum sequence in code located at addr."""
    hits = []
    end = len(code)
    off = 0
    while off < end:
        page_off = (addr + off) & 0xfff
        if page_off < 0xff8:
            off += 0xff8 - page_off
        if end - off < 12:
            break
        i1, i2, i3 = struct.unpack_from('<III', code, off)
        if is_erratum_sequence(i1, i2, i3):
            hits.append(addr + off)
        elif end - off >= 16 and not is_branch(i3):
            i4, = struct.unpack_from('<I', code, off + 12)
            if is_erratum_sequence(i1, i2, i4):
                hits.append(addr + off)
        off += 4 if ((addr + off) & 0xfff) == 0xff8 else 0xffc
    return hits


def read_sections(data):
    """Return [(name, flags, addr, file offset, size)] of an ELF64 little-endian image."""
    if len(data) < 64:
        raise ElfError('truncated ELF header')
    shoff, = struct.unpack_from('<Q', data, 0x28)
    shentsize, shnum, shstrndx = struct.unpack_from('<HHH', data, 0x3a)
    if shoff == 0 or shnum == 0:
        raise ElfError('no section headers')
    if shentsize != 64 or shoff + shnum * 64 > len(data) or shstrndx >= shnum:
        raise ElfError('invalid section header table')
    headers = [struct.unpack_from('<IIQQQQ', data, shoff + n * 64) for n in range(shnum)]
    strtab_off, strtab_size = headers[shstrndx][4], headers[shstrndx][5]
    if strtab_off + strtab_size > len(data):
        raise ElfError('invalid section name table')
    sections = []
    for name, sh_type, flags, addr, off, size in headers:
        if name >= strtab_size:
            raise ElfError('invalid section name offset')
        end = data.find(b'\0', strtab_off + name, strtab_off + strtab_size)
        if end < 0:
            raise ElfError('unterminated section name')
        sections.append((data[strtab_off + name:end].decode('latin-1'), sh_type, flags, addr, off, size))
    return sections


def check_elf(data):
    """Return (is_go, [hit addresses]) for an AArch64 ELF image, or None if it is not an AArch64 program to check.

    Each executable output section is scanned as a whole. The linkers scan input sections, so they cannot patch a
    sequence that straddles two input sections; such a sequence would be reported here. Like inline data in code
    sections, it would need investigating rather than a relaxed rule."""
    if data[4:6] != b'\x02\x01':
        return None  # only ELF64 little-endian can be AArch64 code
    e_type, e_machine = struct.unpack_from('<HH', data, 0x10)
    if e_machine != EM_AARCH64 or e_type not in (ET_EXEC, ET_DYN):
        return None
    sections = read_sections(data)
    is_go = any(s[0] == '.go.buildinfo' for s in sections)
    hits = []
    for name, sh_type, flags, addr, off, size in sections:
        if not flags & SHF_EXECINSTR or sh_type == 8:  # SHT_NOBITS has no file contents
            continue
        if off + size > len(data):
            raise ElfError(f'section {name} extends past the end of the file')
        hits.extend(scan_code(data[off:off + size], addr))
    return is_go, hits


def regular_files(top, errors):
    """Yield the regular files under top in a stable order; directories that cannot be read go to errors."""
    for root, dirs, files in os.walk(top, onerror=errors.append):
        dirs.sort()
        for name in sorted(files):
            path = os.path.join(root, name)
            if not os.path.islink(path) and os.path.isfile(path):
                yield path


def check_file(path):
    """Return check_elf()'s result for an ELF file, or None for anything else."""
    with open(path, 'rb') as f:
        if f.read(4) != b'\x7fELF':
            return None
        f.seek(0)
        return check_elf(f.read())


def report(rel, is_go, hits):
    """Print the hits of one executable; return True if they fail the check."""
    if not hits:
        return False
    addrs = ', '.join(f'{a:#x}' for a in hits[:8]) + (', ...' if len(hits) > 8 else '')
    if is_go:
        print(f'NOTICE: {rel}: {len(hits)} sequence(s) in a Go binary (no Go linker workaround): {addrs}')
        return False
    print(f'ERROR: {rel}: {len(hits)} sequence(s), linked without --fix-cortex-a53-843419: {addrs}')
    return True


def main(argv):
    if len(argv) != 2 or not os.path.isdir(argv[1]):
        print(f'usage: {argv[0]} <directory>', file=sys.stderr)
        return 2
    top = argv[1]

    failed = False
    checked = 0
    walk_errors = []
    for path in regular_files(top, walk_errors):
        rel = os.path.relpath(path, top)
        try:
            result = check_file(path)
        except (OSError, ElfError, struct.error) as e:
            print(f'ERROR: {rel}: {e}')
            failed = True
            continue
        if result is not None:
            checked += 1
            failed |= report(rel, *result)

    for e in walk_errors:
        print(f'ERROR: {os.path.relpath(e.filename, top)}: {e.strerror}')
        failed = True

    if checked == 0:
        print('ERROR: no AArch64 executables found')
        return 1
    print(f'checked {checked} AArch64 executable(s): {"FAILED" if failed else "OK"}')
    return 1 if failed else 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
