#!/usr/bin/env python3

import sys

from modules.release_versions import stable_reaches_preview


def package_repos(version: str, repo: str) -> list[str]:
    repos = [repo]
    if stable_reaches_preview(version):
        repos.append(f'{repo}-preview')
    return repos


if __name__ == '__main__':
    print(' '.join(package_repos(sys.argv[1], sys.argv[2])))
