# SPDX-License-Identifier: GPL-3.0-or-later
"""Maintainer-only Snapshot mapping; signed-index SHA256/size remain authoritative."""
import copy
import json
from pathlib import Path
import re
import sys
import tempfile
from urllib.error import HTTPError
from urllib.parse import quote

from locked import DOWNLOAD_TIMEOUT_SECONDS, HTTPS_OPENER, download


BASE = 'https://snapshot.debian.org'
METADATA_LIMIT = 16 * 1024 * 1024


def package_files(name, version):
    address = BASE + '/mr/package/' + quote(name, safe='') + '/' + quote(version, safe='') + '/allfiles?fileinfo=1'
    print('+ snapshot metadata ' + address, file=sys.stderr, flush=True)
    with HTTPS_OPENER.open(address, timeout=DOWNLOAD_TIMEOUT_SECONDS) as response:
        data = response.read(METADATA_LIMIT + 1)
    if len(data) > METADATA_LIMIT:
        raise ValueError('Snapshot metadata exceeds limit: ' + name + '@' + version)
    record = json.loads(data)
    if record.get('package') != name or record.get('version') != version:
        raise ValueError('Snapshot source identity mismatch: ' + name + '@' + version)
    return record


def resolve_file(record, entries, fileinfo, verified):
    name = record.get('file', record['name'])
    digest, size = record['sha256'], record['bytes']
    if (Path(name).name != name or name in ('', '.', '..') or
            not re.fullmatch('[a-f0-9]{64}', digest) or type(size) is not int or size <= 0):
        raise ValueError('invalid locked Snapshot input: ' + name)
    candidates = set()
    for entry in entries:
        identity = entry['hash']
        if not re.fullmatch('[a-f0-9]{40}', identity):
            raise ValueError('invalid Snapshot file identifier')
        if any(item['name'] == name and item['size'] == size for item in fileinfo.get(identity, [])):
            candidates.add(identity)
    if not candidates:
        raise ValueError('Snapshot has no matching file: ' + name)
    known = verified.get((digest, size))
    if known in candidates:
        return BASE + '/file/' + known
    for identity in sorted(candidates):
        address = BASE + '/file/' + identity
        # A fresh scratch per candidate prevents a valid cached file from making
        # an unverified candidate URL appear correct. Duplicate locked identities
        # reuse only a mapping verified during this invocation.
        with tempfile.TemporaryDirectory(prefix='dem-snapshot-file-') as scratch:
            try:
                download({'name': name, 'url': address, 'sha256': digest, 'bytes': size}, Path(scratch))
            except ValueError:
                continue
            except HTTPError as error:
                if error.code in (403, 404, 451):
                    continue
                raise
        verified[digest, size] = identity
        return address
    raise ValueError('Snapshot candidates do not supply the locked bytes: ' + name)


def resolve_locks(locks):
    """Return URL-only copies after verifying every file, with no apt resolution."""
    result = copy.deepcopy(locks)
    metadata, verified = {}, {}

    def source(name, version):
        key = name, version
        if key not in metadata:
            metadata[key] = package_files(name, version)
        return metadata[key]

    for lock in result:
        if lock['format'] != 1 or lock['architecture'] not in ('amd64', 'arm64'):
            raise ValueError('unsupported Debian lock identity')
        for package in lock['binary_packages']:
            listing = source(package['source'], package['source_version'])
            entries = [entry for binary in listing['result']['binaries']
                       if binary['name'] == package['name'] and binary['version'] == package['version']
                       for entry in binary['files'] if entry['architecture'] == package['architecture']]
            package['url'] = resolve_file(package, entries, listing['fileinfo'], verified)
        for package in lock['sources'].values():
            listing = source(package['name'], package['version'])
            for item in package['files']:
                item['url'] = resolve_file(item, listing['result']['source'], listing['fileinfo'], verified)
    return result
