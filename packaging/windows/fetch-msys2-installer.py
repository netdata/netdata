#!/usr/bin/env python

'''Fetch a pinned MSYS2 file bundled in the Windows installer.'''

from __future__ import annotations

import hashlib
import shutil
import sys

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Final
from urllib.request import Request, urlopen

# Pinned so an MSYS2 release can not change what we ship without a reviewed change.
PINS: Final = {
    # The MSYS2 tree the installer ships. This release bundles msys2-runtime 3.6.10, whose msys-2.0.dll kills netdata
    # at startup; package-windows.sh replaces that DLL with the one from the runtime package below.
    'base': (
        'https://github.com/msys2/msys2-installer/releases/download/2026-09-27/msys2-base-x86_64-20260927.tar.zst',
        '8a2095647afbf799d113d0cae35f7544589d059819042635504e75bd2179bd02',
    ),
    # MSYS2's msys2-runtime 3.6.11, which carries the upstream fixes 3.6.10 lacks. Drop it once the base pin moves to
    # a release that bundles 3.6.11 or later.
    'runtime': (
        'https://repo.msys2.org/msys/x86_64/msys2-runtime-3.6.11-4-x86_64.pkg.tar.zst',
        '7b1c4b625483ceff9a87f2dfcf3659704dc3da655a0ad414ecb7d9175e7d3ba0',
    ),
}


def sha256sum(path: Path) -> str:
    '''Return the SHA256 checksum of a file.'''
    return hashlib.sha256(path.read_bytes()).hexdigest().casefold()


def fetch(tmpdir: Path, url: str) -> Path:
    '''Fetch a file into a temporary directory.'''
    FILE: Final = url.rsplit('/', 1)[1]
    TARGET: Final = tmpdir / FILE

    print(f'>>> Downloading {FILE}')

    with urlopen(Request(url=url, method='GET'), timeout=15) as response:
        if response.status != 200:
            print(f'!!! Failed to fetch {FILE}, status={response.status}')
            sys.exit(1)

        TARGET.write_bytes(response.read())

    return TARGET


def main() -> None:
    '''Core program logic.'''
    if len(sys.argv) != 3 or sys.argv[1] not in PINS:
        print(f'Usage: {__file__} <{"|".join(PINS)}> <target>')
        sys.exit(1)

    url, sha256 = PINS[sys.argv[1]]
    target = Path(sys.argv[2])
    tmp_target = target.with_name(f'.{target.name}.tmp')

    if target.is_file() and sha256sum(target) == sha256:
        print(f'>>> {target} already holds {url}')
        return

    with TemporaryDirectory() as tmpdir:
        file = fetch(Path(tmpdir), url)

        print('>>> Verifying SHA256 checksum')
        actual_checksum = sha256sum(file)

        if actual_checksum != sha256:
            print('!!! Checksum mismatch')
            print(f'!!! Expected: {sha256}')
            print(f'!!! Actual:   {actual_checksum}')
            sys.exit(1)

        print(f'>>> Copying to {target}')

        shutil.copy(file, tmp_target)
        tmp_target.replace(target)


if __name__ == '__main__':
    main()
