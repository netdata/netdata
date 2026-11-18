#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess

from modules.release_versions import release_key, version_core

REPOS = {
    'docker': 'netdata/netdata',
    'quay': 'quay.io/netdata/netdata',
    'ghcr': 'ghcr.io/netdata/netdata',
}
NIGHTLY_TAG = 'edge'
VERSION_LABEL = 'org.opencontainers.image.version'


def published_version(repo: str, alias: str) -> str | None:
    """Read a published Linux/amd64 image's version label without downloading its layers."""
    reference = f'{repo}:{alias}'
    result = subprocess.run(
        ['skopeo', '--command-timeout', '60s', '--override-os', 'linux', '--override-arch', 'amd64',
         'inspect', '--no-tags', f'docker://{reference}'],
        capture_output=True, text=True, check=False, timeout=90,
    )
    if result.returncode:
        # Only an explicitly absent top-level alias is eligible. Auth, rate-limit, transport,
        # missing config/blob and other registry errors must stop publication.
        missing = rf'(?:reading|fetching) manifest {re.escape(alias)} in [^\n]*: (?:manifest unknown|name unknown)\b'
        if re.search(missing, result.stderr, re.IGNORECASE):
            return None
        # Do not forward CLI diagnostics, which may contain authentication/service details.
        raise RuntimeError(f'Unable to inspect published image {reference}')
    labels = json.loads(result.stdout).get('Labels') or {}
    version = labels.get(VERSION_LABEL)
    if not isinstance(version, str):
        raise ValueError(f'Published image {reference} has no {VERSION_LABEL} label')
    version_core(version)
    return version


def generate(version: str, registries: list[str], *, metadata_only: bool = False) -> dict[str, str]:
    nightly = version == 'nightly'
    prerelease = False
    if version not in ('', 'nightly'):
        version_core(version)
        version = version.lstrip('v')
        prerelease = '-' in version.split('+', 1)[0]

    tags = {}
    for registry in registries:
        repo = REPOS[registry]
        if not version:
            versions = ['test']
        elif nightly:
            versions = [NIGHTLY_TAG, 'latest']
        else:
            # latest tracks publication time; only channel and series aliases are monotonic.
            versions = [f'v{version}', 'latest']
            aliases = []
            if not prerelease:
                major, minor, _ = version.split('+', 1)[0].split('.')
                aliases.extend((f'v{major}', f'v{major}.{minor}', 'stable'))
            aliases.append('preview')
            if not metadata_only:
                for alias in aliases:
                    published = published_version(repo, alias)
                    if published is None or release_key(version) >= release_key(published):
                        versions.append(alias)
        tags[registry] = [f'{repo}:{tag}' for tag in versions]

    result = {
        'tags': ','.join(tag for registry_tags in tags.values() for tag in registry_tags),
        'nightly': '1' if nightly else '0',
        'prerelease': 'true' if prerelease else 'false',
        'nightly_tag': NIGHTLY_TAG,
    }
    for registry in registries:
        result[f'{registry}_tags'] = ','.join(tags[registry])
        result[f'{registry}_repo'] = REPOS[registry]
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description='Select Docker tags against published image version labels.')
    parser.add_argument('event')
    parser.add_argument('version')
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--registry', choices=REPOS, help='Check only this destination registry before publication')
    mode.add_argument('--metadata-only', action='store_true', help='Emit build metadata without checking aliases')
    args = parser.parse_args()
    registries = [args.registry] if args.registry else list(REPOS)
    result = generate(args.version, registries, metadata_only=args.metadata_only)
    with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
        for name, value in result.items():
            output.write(f'{name}={value}\n')


if __name__ == '__main__':
    main()
