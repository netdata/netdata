#!/usr/bin/env python

'''Fetch the pinned MSYS2 base tarball bundled in the Windows installer.'''

from __future__ import annotations

import hashlib
import shutil
import sys

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Final
from urllib.request import Request, urlopen

REPO: Final = 'msys2/msys2-installer'

# Pinned so an MSYS2 release can not change what we ship without a reviewed change. This release bundles
# msys2-runtime 3.6.10, whose msys-2.0.dll kills netdata at startup; package-windows.sh replaces that DLL with
# the patched build from msys2-runtime/ (see msys2-runtime/runtime.env).
RELEASE: Final = '2026-09-27'
SHA256: Final = '8a2095647afbf799d113d0cae35f7544589d059819042635504e75bd2179bd02'
FILE: Final = f'msys2-base-x86_64-{RELEASE.replace("-", "")}.tar.zst'


def sha256sum(path: Path) -> str:
    '''Return the SHA256 checksum of a file.'''
    return hashlib.sha256(path.read_bytes()).hexdigest().casefold()


def fetch_release_asset(tmpdir: Path, file: str) -> Path:
    '''Fetch a specific asset of the pinned release.'''
    REQUEST: Final = Request(
        url=f'https://github.com/{REPO}/releases/download/{RELEASE}/{file}',
        method='GET',
    )
    TARGET: Final = tmpdir / file

    print(f'>>> Downloading {file}')

    with urlopen(REQUEST, timeout=15) as response:
        if response.status != 200:
            print(f'!!! Failed to fetch {file}, status={response.status}')
            sys.exit(1)

        TARGET.write_bytes(response.read())

    return TARGET


def main() -> None:
    '''Core program logic.'''
    if len(sys.argv) != 2:
        print(f'{__file__} must be run with exactly one argument.')
        sys.exit(1)

    target = Path(sys.argv[1])
    tmp_target = target.with_name(f'.{target.name}.tmp')

    if target.is_file() and sha256sum(target) == SHA256:
        print(f'>>> {target} already holds {FILE}')
        return

    with TemporaryDirectory() as tmpdir:
        installer = fetch_release_asset(Path(tmpdir), FILE)

        print('>>> Verifying SHA256 checksum')
        actual_checksum = sha256sum(installer)

        if actual_checksum != SHA256:
            print('!!! Checksum mismatch')
            print(f'!!! Expected: {SHA256}')
            print(f'!!! Actual:   {actual_checksum}')
            sys.exit(1)

        print(f'>>> Copying to {target}')

        shutil.copy(installer, tmp_target)
        tmp_target.replace(target)


if __name__ == '__main__':
    main()
