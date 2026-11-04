#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Embed the canonical privileged helper without adding a runtime download.

Run with --check in validation. Generation changes only the marked inert heredoc.
"""
import argparse
from pathlib import Path


def generated_block(helper: bytes) -> bytes:
    delimiter = b'NETDATA_DEM_HOST_HELPER_EMBEDDED'
    if delimiter in helper.splitlines():
        raise ValueError('embedded helper contains the heredoc delimiter')
    if not helper.endswith(b'\n'):
        raise ValueError('canonical helper must end with a newline')
    return b"cat <<'" + delimiter + b"'\n" + helper + delimiter + b'\n'


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    path = root / 'kickstart.sh'
    source = path.read_bytes()
    start = b'# BEGIN GENERATED DEM HOST HELPER\n'
    end = b'# END GENERATED DEM HOST HELPER\n'
    if source.count(start) != 1 or source.count(end) != 1:
        raise ValueError('kickstart must contain exactly one helper marker pair')
    before, rest = source.split(start)
    old, after = rest.split(end)
    block = generated_block((root / 'netdata-dem-runtime.sh').read_bytes())
    if args.check:
        if old != block:
            parser.exit(1, 'kickstart embedded helper differs; run generate-dem-kickstart-helper.py\n')
    else:
        path.write_bytes(before + start + block + end + after)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
