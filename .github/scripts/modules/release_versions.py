"""Release ordering and alias eligibility against published artifact metadata (stdlib only)."""

import re
import sys
from urllib.error import HTTPError
import urllib.request

ARTIFACT_URL_PREFIX = 'https://artifacts.netdata.cloud'


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


def release_key(value: str) -> tuple:
    """Order releases: release candidates (-rcNN) precede the stable release with the same core."""
    core = version_core(value)
    prerelease = value.split('+', 1)[0].partition('-')[2]
    if not prerelease:
        return (*core, 1, 0)
    match = re.fullmatch(r'rc\.?([0-9]+)', prerelease)
    return (*core, 0, int(match.group(1)) if match else 0)


def published_version(alias: str) -> str | None:
    """Return the version published under <channel>/<alias>, or None when nothing is published there."""
    try:
        with urllib.request.urlopen(f'{ARTIFACT_URL_PREFIX}/{alias}/Version', timeout=30) as response:
            return response.read().decode('utf-8').strip()
    except HTTPError as error:
        if error.code != 404:
            raise
        error.close()
        return None


def reaches(version: str, alias: str) -> bool:
    """True when VERSION is not older than the version already published under ALIAS (e.g. stable/2.14)."""
    key = release_key(version)
    published = published_version(alias)
    # Build metadata is ignored, so republishing the same release stays eligible.
    return published is None or key >= release_key(published)


if __name__ == '__main__':
    if sys.argv[1] == '--at-least':
        # Compare against an already-read published version (e.g. read from the bucket under a publication lock).
        print('true' if release_key(sys.argv[2]) >= release_key(sys.argv[3]) else 'false')
    else:
        print('true' if reaches(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else 'preview/latest') else 'false')
