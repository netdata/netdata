#!/usr/bin/env python3

import re
import sys
from pathlib import Path


def manifest_args(digest_dir, repo, all_tags, expected_arches):
    tags = [tag for tag in all_tags.split(',') if tag.startswith(f'{repo}:')]
    if not tags:
        raise ValueError(f'no publication tags for {repo}')
    files = {path.name: path for path in digest_dir.iterdir()}
    if not expected_arches or set(files) != set(expected_arches):
        raise ValueError(f'expected digest artifacts for {expected_arches}, found {sorted(files)}')
    digests = []
    for arch in expected_arches:
        digest = files[arch].read_text().strip()
        if not re.fullmatch(r'sha256:[0-9a-f]{64}', digest):
            raise ValueError(f'invalid image digest for {arch}')
        digests.append(digest)
    if len(set(digests)) != len(digests):
        raise ValueError('each architecture must have a distinct image digest')
    return [arg for tag in tags for arg in ('-t', tag)] + [f'{repo}@{digest}' for digest in digests]


if __name__ == '__main__':
    digest_path, repo, tags, *arches = sys.argv[1:]
    print(' '.join(manifest_args(Path(digest_path), repo, tags, arches)))
