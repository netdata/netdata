# SPDX-License-Identifier: GPL-3.0-or-later
"""Small readers for the reviewed Debian input lock; no package resolution."""
import hashlib
import json
from pathlib import Path
import re
import urllib.request

from bundle import display_argument, run, sha256, write_json


def paragraphs(text):
    result, current, key = [], {}, None
    for line in text.splitlines() + ['']:
        if line.startswith('-----BEGIN PGP SIGNATURE-----'):
            break
        if not line:
            if current:
                result.append(current)
            current, key = {}, None
        elif line[0].isspace() and key:
            current[key] += '\n' + line[1:]
        elif ': ' in line or line.endswith(':'):
            key, value = line.split(':', 1)
            current[key] = value.lstrip()
    if current:
        result.append(current)
    return result


def inventory():
    text = run(['dpkg-query', '-W', '-f=${Package}\t${Version}\t${Architecture}\t${source:Package}\t${source:Version}\n'], capture=True)
    return [dict(zip(('name', 'version', 'architecture', 'source', 'source_version'), line.split('\t')))
            for line in sorted(text.splitlines())]


def check_inventory(lock):
    if inventory() != lock['installed']:
        raise ValueError('installed Debian packages differ from the reviewed binary lock')


def source_key(name, version):
    return name + '@' + version


def checksums(record):
    result = []
    for line in record['Checksums-Sha256'].splitlines():
        if not line.strip():
            continue
        digest, size, name = line.split()
        if not re.fullmatch('[a-f0-9]{64}', digest) or Path(name).name != name or name in ('.', '..'):
            raise ValueError('invalid Debian source checksum record')
        result.append({'name': name, 'sha256': digest, 'bytes': int(size)})
    return result


def repository_url(path):
    base = 'https://security.debian.org/debian-security/' if path.startswith('pool/updates/') else 'https://deb.debian.org/debian/'
    return base + path


def download(record, directory):
    """Hash-only acquisition, with an idempotent cache and atomic verified publication."""
    import sys
    from urllib.parse import urlsplit
    url = record['url']
    parsed = urlsplit(url)
    if parsed.scheme != 'https' or not parsed.hostname or parsed.username or parsed.password:
        raise ValueError('source download must use credential-free HTTPS')
    name = record.get('name', Path(parsed.path).name)
    if Path(name).name != name or name in ('', '.', '..'):
        raise ValueError('invalid locked download basename')
    directory.mkdir(parents=True, exist_ok=True)
    target = directory / name
    algorithm = record.get('algorithm', 'sha256')
    expected = record.get('digest', record.get('sha256'))
    def verify(path):
        digest = hashlib.new(algorithm)
        with path.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(chunk)
        return digest.hexdigest() == expected and ('bytes' not in record or path.stat().st_size == record['bytes'])
    if target.exists():
        if not verify(target):
            raise ValueError('cached input hash/size mismatch: ' + name)
        return target
    temporary = target.with_name(target.name + '.partial')
    print('+ download ' + display_argument(url) + ' -> ' + display_argument(target), file=sys.stderr, flush=True)
    with urllib.request.urlopen(url) as response, temporary.open('wb') as output:
        if urlsplit(response.url).scheme != 'https':
            raise ValueError('download redirected outside HTTPS')
        while True:
            chunk = response.read(1024 * 1024)
            if not chunk:
                break
            output.write(chunk)
    if not verify(temporary):
        raise ValueError('download hash/size mismatch: ' + name)
    temporary.rename(target)
    return target
