"""Shared stable-release eligibility for preview publishing (stdlib only)."""

import re
import sys
from urllib.error import HTTPError
import urllib.request

PREVIEW_VERSION_URL = 'https://artifacts.netdata.cloud/preview/latest/Version'


def version_core(value: str, *, stable: bool = False) -> tuple[int, int, int]:
    """Validate SemVer (allowing Netdata's v prefix) and return its numeric core."""
    number = r'(?:0|[1-9][0-9]*)'
    prerelease = rf'(?:{number}|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
    suffix = '' if stable else rf'(?:-{prerelease}(?:\.{prerelease})*)?'
    match = re.fullmatch(
        rf'v?({number})\.({number})\.({number}){suffix}'
        r'(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?', value,
    )
    if match is None:
        raise ValueError(f'Invalid {"stable " if stable else ""}semantic version: {value!r}')
    return tuple(int(part) for part in match.groups())


def stable_reaches_preview(version: str) -> bool:
    stable_core = version_core(version, stable=True)
    try:
        with urllib.request.urlopen(PREVIEW_VERSION_URL, timeout=30) as response:
            preview_version = response.read().decode('utf-8').strip()
    except HTTPError as error:
        if error.code != 404:
            raise
        # Missing preview metadata means no published version to outrank this release.
        error.close()
        return True
    # Stable releases outrank prereleases with the same core; metadata is ignored.
    return stable_core >= version_core(preview_version)


if __name__ == '__main__':
    print('true' if stable_reaches_preview(sys.argv[1]) else 'false')
