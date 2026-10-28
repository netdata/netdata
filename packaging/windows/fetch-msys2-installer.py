#!/usr/bin/env python

'''Fetch the pinned MSYS2 files bundled in the Windows installer.'''

from __future__ import annotations

import hashlib
import shutil
import sys

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Final
from urllib.request import Request, urlopen

MSYS_REPO: Final = 'https://repo.msys2.org/msys/x86_64'

# Pinned so an MSYS2 release can not change what we ship without a reviewed change.
#
# The MSYS2 tree the installer ships.
BASE: Final = (
    'https://github.com/msys2/msys2-installer/releases/download/2026-09-27/msys2-base-x86_64-20260927.tar.zst',
    '8a2095647afbf799d113d0cae35f7544589d059819042635504e75bd2179bd02',
)

# Packages package-windows.sh installs over the base, because the base predates fixes netdata needs. Each one must be
# newer than the copy in the base; drop it once the base pin moves to a release that bundles it or later.
PACKAGES: Final = (
    # The base's msys2-runtime 3.6.10 kills netdata at startup; 3.6.11 carries the upstream fixes it lacks.
    (f'{MSYS_REPO}/msys2-runtime-3.6.11-4-x86_64.pkg.tar.zst',
     '7b1c4b625483ceff9a87f2dfcf3659704dc3da655a0ad414ecb7d9175e7d3ba0'),
    # Security updates for libraries netdata loads, and for the matching command line tools.
    (f'{MSYS_REPO}/libopenssl-3.6.5-1-x86_64.pkg.tar.zst',
     '05f6cc946f0dde3aecc3632007fa1d874e40720226117b750327bc2a01846325'),
    (f'{MSYS_REPO}/openssl-3.6.5-1-x86_64.pkg.tar.zst',
     '4dc7c1199010b26d6382c59d8b16a9dbc857d314d76d9668f0f333bc5fecdd38'),
    (f'{MSYS_REPO}/libcurl-8.23.0-1-x86_64.pkg.tar.zst',
     '3034aae7fb58e129c7af62a7e7d2ae12e0a3475a5310d7fdad38fe9a3c672c9d'),
    (f'{MSYS_REPO}/curl-8.23.0-1-x86_64.pkg.tar.zst',
     '6f21e0187e3f309199e01f729d415ccfeb304cc9f8a27365c8aa9c216351022e'),
    (f'{MSYS_REPO}/brotli-1.2.1-1-x86_64.pkg.tar.zst',
     '0cfd703cbe467c6b645444c5d1ec03f423dcaca241bb99b5baab61d70447cefd'),
    (f'{MSYS_REPO}/libpcre2_8-10.49-1-x86_64.pkg.tar.zst',
     '2de42bf4656e3952a72809663e85899be76c7020e016687055ac0acd05145a3b'),
    # The C and C++ runtime of the GCC netdata is compiled with (msys2-dependencies.sh updates the build host).
    (f'{MSYS_REPO}/gcc-libs-16.2.0-1-x86_64.pkg.tar.zst',
     '50e1bbbb1b4186ba1ed8c08cdbb5c0a8c2343e4b5747bace24273e26fb148471'),
)


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


def fetch_pinned(url: str, sha256: str, target: Path) -> None:
    '''Make target hold the pinned file, keeping a cached copy that matches the pin.'''
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


def main() -> None:
    '''Core program logic.'''
    if len(sys.argv) != 3 or sys.argv[1] not in ('base', 'packages'):
        print(f'Usage: {__file__} base <target file> | packages <target directory>')
        sys.exit(1)

    target = Path(sys.argv[2])

    if sys.argv[1] == 'base':
        fetch_pinned(*BASE, target)
        return

    target.mkdir(parents=True, exist_ok=True)
    pinned = {url.rsplit('/', 1)[1] for url, _ in PACKAGES}

    # Leave only the pinned packages: package-windows.sh installs everything in the directory.
    for path in target.iterdir():
        if path.name not in pinned:
            print(f'>>> Removing {path}, it is not pinned')
            path.unlink()

    for url, sha256 in PACKAGES:
        fetch_pinned(url, sha256, target / url.rsplit('/', 1)[1])


if __name__ == '__main__':
    main()
