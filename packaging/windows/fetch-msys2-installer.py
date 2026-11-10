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
    # OpenSSL 4 (msys-crypto-4.dll, msys-ssl-4.dll), which msys2-dependencies.sh builds netdata against. It replaces
    # the base's OpenSSL 3 DLLs, so every base package that imports them is pinned to its OpenSSL 4 rebuild as well.
    (f'{MSYS_REPO}/libopenssl-4.1.0-1-x86_64.pkg.tar.zst',
     '71f093de4a002913f39783b79d18da33b63ce8baf825434218cc1d8bd40304fb'),
    (f'{MSYS_REPO}/openssl-4.1.0-1-x86_64.pkg.tar.zst',
     'b108d0fe6ec296acdb5cc837d314f23a20f07ecd73d2a0e7c077446fd2d01df9'),
    (f'{MSYS_REPO}/libcurl-8.23.0-2-x86_64.pkg.tar.zst',
     '84095312cbe4155c7d2c6d067bb552ea3ec2b693fc9802a01e7320e28946ba59'),
    (f'{MSYS_REPO}/curl-8.23.0-2-x86_64.pkg.tar.zst',
     'cbfbef860ad01a30c7f52b40a12bc2c64e731280ace77efcfa333bfe40a48b2c'),
    (f'{MSYS_REPO}/libngtcp2-1.26.0-2-x86_64.pkg.tar.zst',
     'a36645aad55fc2a43ad0c121a91957648a01cbe4006f690338cd9ce0c1987257'),
    (f'{MSYS_REPO}/libssh2-1.11.1-2-x86_64.pkg.tar.zst',
     '23edc3dea95daac38e58039256aa6923dcd5cc96313f9b062cd60792881367fa'),
    (f'{MSYS_REPO}/bsdtar-3.8.11-2-x86_64.pkg.tar.zst',
     '84b582de0005ff01c9d95347162ea551169c3fb8fd572d341ac4589b4b3cbbb8'),
    # Security updates for other libraries netdata loads.
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
        url, sha256 = BASE
        fetch_pinned(url, sha256, target)
        return

    target.mkdir(parents=True, exist_ok=True)
    pinned = {url.rsplit('/', 1)[1] for url, _ in PACKAGES}

    # Leave only the pinned packages: package-windows.sh installs every package in the directory.
    for path in target.glob('*.pkg.tar.zst'):
        if path.name not in pinned:
            print(f'>>> Removing {path}, it is not pinned')
            path.unlink()

    for url, sha256 in PACKAGES:
        fetch_pinned(url, sha256, target / url.rsplit('/', 1)[1])


if __name__ == '__main__':
    main()
