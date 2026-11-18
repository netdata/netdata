#!/usr/bin/env python3
"""Render explicit release ranges; stable tags need not be ancestors of master."""

import argparse
from pathlib import Path
import re
import subprocess
import tempfile
import tomllib

FIRST_TAG = 'v1.47.0'  # Last release prior to v2.0.0.
NUMBER = r'(0|[1-9][0-9]*)'
STABLE = re.compile(rf'^v{NUMBER}\.{NUMBER}\.{NUMBER}$')
MARKER = re.compile(rf'^v{NUMBER}\.{NUMBER}$')


def git(repo, *args):
    return subprocess.run(['git', *args], cwd=repo, check=True,
                          capture_output=True, text=True).stdout.strip()


def version_key(match):
    return tuple(int(part) for part in match.groups())


def generate(repo):
    config = repo / 'packaging/cliff.toml'
    body = tomllib.loads(config.read_text())['changelog']['body']
    # Fail before touching output if the checkout lacks the historical cutoff.
    git(repo, 'rev-parse', '--verify', f'refs/tags/{FIRST_TAG}^{{commit}}')
    cutoff = version_key(STABLE.fullmatch(FIRST_TAG))
    # creatordate is the tagger date for annotated tags and the commit date for lightweight tags.
    dates = dict(line.split('\t') for line in git(
        repo, 'for-each-ref', '--format=%(refname:strip=2)%09%(creatordate:unix)', 'refs/tags').splitlines())
    releases = sorted((version_key(match), tag)
                      for tag in dates
                      if (match := STABLE.fullmatch(tag)) and version_key(match) > cutoff)

    version = (repo / 'packaging/version').read_text().strip()
    match = re.match(rf'^v?{NUMBER}\.{NUMBER}(?:\.{NUMBER})?(?:-|$)', version)
    if match is None:
        raise ValueError(f'Unrecognized packaging/version: {version!r}')
    current = tuple(int(part or 0) for part in match.groups())
    # A marker commit opens the next series before the next nightly updates packaging/version.
    for tag in git(repo, 'tag', '--merged', 'HEAD').splitlines():
        if marker := MARKER.fullmatch(tag):
            current = max(current, (*version_key(marker), 0))
    baseline = max(((key, tag) for key, tag in releases if key <= current),
                   default=(cutoff, FIRST_TAG))[1]

    def render(start, end, tag=None):
        # Only the end tag may create a heading. Markers, RCs and intermediate release tags cannot
        # split this range. Supply release dates explicitly: git-cliff otherwise uses commit dates,
        # which can precede the date an annotated release tag was created.
        pattern = f'^{re.escape(tag)}$' if tag else '^$'
        template = f'{{% set timestamp = {int(dates[tag])} %}}' + body if tag else body
        return subprocess.run(['git-cliff', '--config', str(config), '--tag-pattern', pattern,
                               '--body', template, '--strip', 'all', f'{start}..{end}'], cwd=repo, check=True,
                              capture_output=True, text=True).stdout.strip()

    sections = [render(baseline, 'HEAD')]
    predecessors = [FIRST_TAG, *(tag for _, tag in releases[:-1])]
    for start, (_, tag) in reversed(list(zip(predecessors, releases))):
        sections.append(render(start, tag, tag))
    return '\n\n'.join(section for section in sections if section) + '\n'


def main():
    repo = Path(__file__).resolve().parent.parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=repo / 'CHANGELOG.md')
    args = parser.parse_args()
    try:
        text = generate(repo)
    except subprocess.CalledProcessError as error:
        parser.exit(1, f'{error.cmd[0]} failed: {error.stderr}')
    except ValueError as error:
        parser.exit(1, f'{error}\n')
    # Preserve the previous changelog if any range fails; replace only a fully rendered document.
    output = args.output.resolve()
    with tempfile.TemporaryDirectory(dir=output.parent, prefix='.changelog-') as directory:
        temporary = Path(directory) / output.name
        temporary.write_text(text)
        temporary.replace(output)


if __name__ == '__main__':
    main()
