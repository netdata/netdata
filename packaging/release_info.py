#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Stamp the official Agent release identity without changing build versions."""
import argparse
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile


FIELDS = ('version', 'channel', 'source')
MAX_BYTES = 256


def validate_release_info(info):
    """Validate the complete identity shared by Agent and companion builds."""
    if set(info) != set(FIELDS) or not all(isinstance(v, str) for v in info.values()):
        raise ValueError('release identity requires exactly version, channel and source')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,127}', info['version']):
        raise ValueError('release version must be a path-safe identifier of 1 to 128 characters')
    if info['channel'] not in ('stable', 'nightly', 'devel'):
        raise ValueError('release channel must be stable, nightly or devel')
    if not re.fullmatch(r'[0-9a-f]{40}', info['source']):
        raise ValueError('release source must be a full lowercase Git commit SHA')
    return dict(info)


def read_release_info(path):
    """Read strict line metadata; never interpret its contents as code."""
    with Path(path).open('rb') as stream:
        data = stream.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError('release identity exceeds its size limit')
    text = data.decode('ascii')
    if not text.endswith('\n') or len(text.split('\n')) != 4:
        raise ValueError('release identity must contain three newline-terminated fields')
    info = {}
    for key, line in zip(FIELDS, text.split('\n')[:-1]):
        if not line.startswith(key + '='):
            raise ValueError('release identity must contain version, channel and source in that order')
        info[key] = line[len(key) + 1:]
    return validate_release_info(info)


def stamp(root, channel, source):
    root = Path(root).resolve()
    info = validate_release_info({
        # Normal development native packages already use this commit identity;
        # packaging/version may still name the preceding nightly on many commits.
        'version': '0.' + source if channel == 'devel' else (root / 'packaging/version').read_text().strip(),
        'channel': channel, 'source': source,
    })
    command = ['git', 'rev-parse', 'HEAD']
    print('release-info: run ' + shlex.join(command) + ' in ' + shlex.quote(str(root)), file=sys.stderr)
    result = subprocess.run(command, cwd=root, check=True, capture_output=True, text=True)
    if result.stdout.strip() != source:
        raise ValueError('checked-out Git commit differs from the expected release source')
    output = root / 'packaging/release-info'
    body = ''.join(key + '=' + info[key] + '\n' for key in FIELDS)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode='w', encoding='ascii', dir=output.parent,
                                         prefix='.release-info-', delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(body)
            stream.flush()
            os.fsync(stream.fileno())
            os.fchmod(stream.fileno(), 0o644)
        os.replace(temporary, output)
    finally:
        if temporary is not None and temporary.exists():
            temporary.unlink()
    print('release-info: wrote ' + str(output), file=sys.stderr)
    return info


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--channel', required=True, choices=('stable', 'nightly', 'devel'))
    parser.add_argument('--source', required=True, help='expected full Git commit SHA')
    args = parser.parse_args()
    try:
        stamp(args.root, args.channel, args.source)
    except subprocess.CalledProcessError as error:
        print('release-info: git rev-parse HEAD failed in ' + str(args.root) +
              ': status ' + str(error.returncode), file=sys.stderr)
        if error.stderr:
            print(error.stderr.rstrip(), file=sys.stderr)
        return error.returncode
    except (OSError, UnicodeError, ValueError) as error:
        print('release-info: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
