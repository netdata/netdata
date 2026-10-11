#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Explicit maintainer-only resolution of Debian inputs. Normal builds never call this."""
import argparse
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys

from bundle import run, sha256, write_json
from locked import checksums, inventory, paragraphs, repository_url, source_key


PACKAGES = ['python3', 'curl', 'ca-certificates', 'xz-utils', 'fontconfig',
            'fonts-dejavu-core', 'fonts-noto-color-emoji', 'fonts-liberation',
            'fonts-freefont-ttf', 'fonts-ipafont-gothic', 'fonts-tlwg-loma-otf',
            'fonts-unifont', 'fonts-wqy-zenhei', 'xfonts-cyrillic', 'xfonts-scalable']


def capture(args):
    installed = inventory()
    base = json.loads(Path('/out/base.json').read_text())
    original = {(p['name'], p['architecture']): p['version'] for p in base}
    binaries, sources = [], {}
    for package in installed:
        name, version = package['name'], package['version']
        if original.get((name, package['architecture'])) != version:
            records = paragraphs(run(['apt-cache', 'show', name + '=' + version], capture=True))
            record = next(r for r in records if r.get('Version') ==
                          version and r.get('Architecture') == package['architecture'])
            binaries.append(dict(package, name=name, file=Path(record['Filename']).name,
                                 url=repository_url(record['Filename']), bytes=int(record['Size']),
                                 sha256=record['SHA256']))
        key = source_key(package['source'], package['source_version'])
        if key in sources:
            continue
        records = paragraphs(run(['apt-cache', 'showsrc', '--only-source', package['source']], capture=True))
        record = next((r for r in records if r.get('Version') == package['source_version']), None)
        if record is None:
            # Tool-only/base source gaps may be irrelevant. The bundle requires an
            # exact source record for every component it actually redistributes.
            print('source unavailable in current indexes: ' + key, file=sys.stderr)
            continue
        files = checksums(record)
        for item in files:
            item['url'] = repository_url(record['Directory'] + '/' + item['name'])
        if not any(item['name'].endswith('.dsc') for item in files):
            raise ValueError('source index has no descriptor: ' + key)
        sources[key] = {'name': package['source'], 'version': package['source_version'], 'files': files}
    lock = {'format': 1, 'architecture': args.arch, 'base_image': args.base_image,
            'chromium': json.loads(Path('/assets/manifest.json').read_text())['chromium'],
            'requested_packages': PACKAGES, 'base': base, 'installed': installed,
            'binary_packages': binaries, 'sources': sources,
            'index_sha256': {p.name: sha256(p) for p in sorted(Path('/var/lib/apt/lists').glob('*InRelease'))}}
    write_json(Path('/out') / (args.arch + '.json'), lock)
    for path in Path('/out').iterdir():
        os.chown(path, int(os.environ['OUTPUT_UID']), int(os.environ['OUTPUT_GID']))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', required=True, choices=('arm64', 'amd64'))
    parser.add_argument('--output', type=Path)
    parser.add_argument('--capture', action='store_true', help=argparse.SUPPRESS)
    parser.add_argument('--base-image', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.capture:
        capture(args)
        return
    if args.output is None:
        parser.error('--output is required')
    recipe = Path(__file__).resolve().parent
    assets = recipe.parent.parent / 'src/go/plugin/dem/synthetic/runner/assets'
    inputs = json.loads((recipe / 'inputs.json').read_text())
    image = inputs['base'].split('@')[0] + '@' + inputs['architectures'][args.arch]['base_manifest']
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error('--output must be empty; review generated locks before adoption')
    # No Python is assumed in the pinned base. Shell tools record its inventory;
    # authenticated apt resolution is restricted to this explicit refresh step.
    version = json.loads((assets / 'manifest.json').read_text())['chromium']['package_version']
    packages = ['chromium=' + version, 'chromium-common=' + version] + PACKAGES
    script = '''set -eu
printf '%s\\n' '+ capture immutable base inventory' >&2
dpkg-query -W '-f=${Package}\\t${Version}\\t${Architecture}\\t${source:Package}\\t${source:Version}\\n' > /out/base.tsv
printf '%s\\n' '+ enable source indexes; apt-get update' >&2
sed -i 's/^Types: deb$/Types: deb deb-src/' /etc/apt/sources.list.d/debian.sources
apt-get update
printf '%s\\n' '+ apt-get upgrade --no-install-recommends' >&2
DEBIAN_FRONTEND=noninteractive apt-get upgrade -y --no-install-recommends
printf '%s\\n' '+ apt-get install exact Chromium and build inputs' >&2
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ''' + shlex.join(packages) + '''
python3 - <<'PYTHON'
import json
from pathlib import Path
p = Path("/out/base.tsv")
fields = ("name", "version", "architecture", "source", "source_version")
rows = [dict(zip(fields, line.split("\\t"))) for line in sorted(p.read_text().splitlines())]
Path("/out/base.json").write_text(json.dumps(rows))
PYTHON
exec python3 /recipe/refresh-lock.py --capture --arch ''' + shlex.quote(args.arch) + ' --base-image ' + \
        shlex.quote(image)
    run(['docker', 'run', '--rm', '--platform', 'linux/' + args.arch,
         '--mount', f'type=bind,src={recipe},dst=/recipe,readonly',
         '--mount', f'type=bind,src={assets},dst=/assets,readonly',
         '--mount', f'type=bind,src={output},dst=/out',
         '-e', 'OUTPUT_UID=' + str(os.getuid()), '-e', 'OUTPUT_GID=' + str(os.getgid()), image, 'sh', '-c', script])


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)
