#!/usr/bin/env python

'''Fetch the MSYS2 installer.'''

from __future__ import annotations

import hashlib
import json
import shutil
import sys

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Final
from urllib.request import Request, urlopen

REPO: Final = 'msys2/msys2-installer'


def get_latest_release() -> tuple[str, str]:
    '''Get the latest release for the repo.'''
    REQUEST: Final = Request(
        url=f'https://api.github.com/repos/{REPO}/releases',
        headers={
            'Accept': 'application/vnd.github+json',
            'X-GitHub-API-Version': '2022-11-28',
        },
        method='GET',
    )

    print('>>> Fetching release list')

    with urlopen(REQUEST, timeout=15) as response:
        if response.status != 200:
            print(f'!!! Failed to fetch release list, status={response.status}')
            sys.exit(1)

        data = json.load(response)

    data = list(filter(lambda x: x['name'] != 'Nightly Installer Build', data))

    name = data[0]['name']
    version = data[0]['tag_name'].replace('-', '')

    return name, version


def fetch_release_asset(tmpdir: Path, name: str, file: str) -> Path:
    '''Fetch a specific release asset.'''
    REQUEST: Final = Request(
        url=f'https://github.com/{REPO}/releases/download/{name}/{file}',
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

    target = Path(sys.argv[1])
    tmp_target = target.with_name(f'.{target.name}.tmp')

    name, version = get_latest_release()

    with TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)

        installer = fetch_release_asset(tmppath, name, f'msys2-base-x86_64-{version}.tar.zst')
        checksums = fetch_release_asset(tmppath, name, f'msys2-base-x86_64-{version}.tar.zst.sha256')

        print('>>> Verifying SHA256 checksum')
        expected_checksum = checksums.read_text().partition(' ')[0].casefold()
        actual_checksum = hashlib.sha256(installer.read_bytes()).hexdigest().casefold()

        if expected_checksum != actual_checksum:
            print('!!! Checksum mismatch')
            print(f'!!! Expected: {expected_checksum}')
            print(f'!!! Actual:   {actual_checksum}')
            sys.exit(1)

        print(f'>>> Copying to {target}')

        shutil.copy(installer, tmp_target)
        tmp_target.replace(target)


if __name__ == '__main__':
    main()
