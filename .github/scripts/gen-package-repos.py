#!/usr/bin/env python3

import sys

from modules.release_versions import reaches, version_core


def package_repos(version: str, repo: str) -> list[str]:
    repos = [repo]
    version_core(version, stable=True)
    if reaches(version, 'preview/latest'):
        repos.append(f'{repo}-preview')
    return repos


if __name__ == '__main__':
    print(' '.join(package_repos(sys.argv[1], sys.argv[2])))
