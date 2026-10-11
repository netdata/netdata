#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Assemble exact release sources separately from the executable runtime."""
import argparse
import base64
import json
from pathlib import Path
import shutil

from bundle import sha256, write_json
from locked import checksums, download, paragraphs


def verify_descriptor(path, source):
    descriptor = next((p for p in paragraphs(path.read_text()) if 'Source' in p), None)
    if descriptor is None or descriptor['Source'] != source['name'] or descriptor['Version'] != source['version']:
        raise ValueError('source descriptor identity mismatch')
    actual = {(p['name'], p['sha256'], p['bytes']) for p in checksums(descriptor)}
    expected = {(p['name'], p['sha256'], p['bytes']) for p in source['files'] if not p['name'].endswith('.dsc')}
    if actual != expected:
        raise ValueError('source descriptor companions differ from reviewed lock')


def npm_sources(lock):
    for name, package in sorted(lock['packages'].items()):
        if not name or package.get('dev'):
            continue
        algorithm, integrity = package['integrity'].split('-', 1)
        if algorithm not in ('sha256', 'sha512'):
            raise ValueError('unsupported npm source integrity algorithm')
        digest = base64.b64decode(integrity, validate=True).hex()
        yield {'name': digest + '.tgz', 'url': package['resolved'], 'algorithm': algorithm, 'digest': digest}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime-output', required=True, type=Path,
                        help='completed builder output containing provenance.json')
    parser.add_argument('--output', required=True, type=Path,
                        help='source assembly/cache; reruns verify existing files')
    args = parser.parse_args()
    recipe = Path(__file__).resolve().parent
    output = args.output.resolve()
    if output.is_relative_to(recipe):
        parser.error('--output must be outside the recipe directory')
    assets = recipe.parent.parent / 'src/go/plugin/dem/synthetic/runner/assets'
    provenance = json.loads((args.runtime_output / 'provenance.json').read_text())
    for name, digest in provenance['recipe_sha256'].items():
        if sha256(recipe / name) != digest:
            raise ValueError('check out the exact runtime recipe before assembling sources: ' + name)
    for name, digest in provenance['source_sha256'].items():
        if sha256(assets / name) != digest:
            raise ValueError('runner source input differs from runtime: ' + name)
    sources = provenance['debian_sources']
    downloads = []
    descriptors = []
    copies = []
    for source in sources.values():
        directory = output / 'debian' / (source['name'] + '_' + source['version'])
        downloads.extend((item, directory) for item in source['files'])
        descriptor = next(item for item in source['files'] if item['name'].endswith('.dsc'))
        descriptors.append((directory / descriptor['name'], source))
    downloads.extend((item, output / name) for name, item in provenance['inputs']['sources'].items())
    npm = json.loads((assets / 'package-lock.json').read_text())
    downloads.extend((item, output / 'npm') for item in npm_sources(npm))
    destination = output / 'recipe'
    for path in recipe.rglob('*'):
        if not path.is_file() or '__pycache__' in path.parts or path.suffix == '.pyc':
            continue
        copies.append((path, destination / path.relative_to(recipe)))
    for name in ('manifest.json', 'package.json', 'package-lock.json'):
        copies.append((assets / name, destination / ('runner-' + name)))
    for name in ('manifest.json', 'provenance.json', 'sbom.cdx.json', 'sources-required.json'):
        copies.append((args.runtime_output / name, output / ('runtime-' + name)))
    expected = {directory / item['name'] for item, directory in downloads}
    expected.update(target for _, target in copies)
    # A reusable assembly may contain only this runtime's declared inputs.
    # Never certify or delete unrelated files left by another invocation.
    for path in expected | {output / 'source-assembly.json'}:
        if path.exists() and not path.is_file():
            raise ValueError('unexpected source assembly entry: ' + str(path.relative_to(output)))
    for path in output.rglob('*'):
        if path.is_symlink() or (not path.is_dir() and path not in expected
                                 and path != output / 'source-assembly.json'):
            raise ValueError('unexpected source assembly entry: ' + str(path.relative_to(output)))
    output.mkdir(parents=True, exist_ok=True)
    for item, directory in downloads:
        download(item, directory)
    for descriptor, source in descriptors:
        verify_descriptor(descriptor, source)
    for source, target in copies:
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
    inventory = {path.relative_to(output).as_posix(): {'sha256': sha256(path), 'bytes': path.stat().st_size}
                 for path in sorted(expected)}
    write_json(output / 'source-assembly.json', {'format': 1, 'runtime_archive': json.loads(
        (args.runtime_output / 'manifest.json').read_text())['archive']['sha256'], 'files': inventory})
    print('Verified release source assembly: ' + str(output))


if __name__ == '__main__':
    main()
