#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Assemble exact release sources separately from the executable runtime."""
import argparse
import base64
import json
import os
from pathlib import Path, PurePosixPath
import shutil

from bundle import package_archive, recipe_sources, run, runtime_sources, sha256, write_json
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


def archive_sources(source, output, runtime_digest, epoch):
    """Package only a complete verified assembly, outside its reusable input tree."""
    source, output = source.resolve(), output.resolve()
    if output.is_relative_to(source) or source.is_relative_to(output):
        raise ValueError('source archive output must be separate from source assembly')
    record = json.loads((source / 'source-assembly.json').read_text())
    if record.get('format') != 1 or record.get('runtime_archive') != runtime_digest:
        raise ValueError('source assembly belongs to a different runtime')
    declared = record['files']
    for name, item in declared.items():
        relative = PurePosixPath(name)
        if relative.is_absolute() or '..' in relative.parts or str(relative) != name:
            raise ValueError('invalid source inventory path')
        path = source / name
        if not path.is_file() or path.is_symlink() or path.stat().st_size != item['bytes'] or sha256(path) != item['sha256']:
            raise ValueError('source assembly content differs from inventory: ' + name)
    actual = set()
    for path in source.rglob('*'):
        if path.is_symlink() or not (path.is_file() or path.is_dir()):
            raise ValueError('source assembly contains a non-regular entry')
        if path.is_file():
            actual.add(path.relative_to(source).as_posix())
    if actual != set(declared) | {'source-assembly.json'}:
        raise ValueError('source assembly file set differs from inventory')
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        raise ValueError('source archive output must be empty')
    archive, bounds = package_archive(source, output, epoch)
    final = output / (bounds['sha256'] + '.sources.tar.gz')
    archive.rename(final)
    write_json(output / 'source-archive.json', {'format': 1, 'runtime_archive': runtime_digest,
                                              'file': final.name, 'sha256': bounds['sha256'],
                                              'bytes': bounds['bytes']})
    return final


# These are the source/input fields reported by `go list -json`, not a recursive
# directory copy. Module-cache dependencies are rebuilt from go.mod/go.sum pins.
GO_BUILD_INPUTS = ('GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'MFiles', 'HFiles',
                   'FFiles', 'SFiles', 'SwigFiles', 'SwigCXXFiles', 'SysoFiles', 'EmbedFiles')


def source_file(root, name):
    relative = PurePosixPath(name)
    if relative.is_absolute() or '..' in relative.parts or str(relative) != name:
        raise ValueError('invalid source input path')
    path = root / name
    if not path.is_file() or path.is_symlink() or not path.resolve().is_relative_to(root.resolve()):
        raise ValueError('source input must be a regular file inside its source root: ' + name)
    return path


def release_tool_sources(module):
    """Exact local build closure for both supported Linux targets and cgo modes."""
    module = module.resolve()
    names = {'go.mod', 'go.sum'}
    decoder = json.JSONDecoder()
    for arch in ('amd64', 'arm64'):
        for cgo in ('0', '1'):
            env = dict(os.environ, GOOS='linux', GOARCH=arch, CGO_ENABLED=cgo, GOWORK='off', GOFLAGS='')
            data = run(['go', 'list', '-mod=readonly', '-deps', '-json', './tools/dem-runtime-release'],
                       cwd=module, capture=True, env=env)
            while data.strip():
                package, end = decoder.raw_decode(data.lstrip())
                data = data.lstrip()[end:]
                if package.get('Standard'):
                    continue
                owner = package.get('Module')
                if not owner or (owner.get('Replace') and not owner['Replace'].get('Version')):
                    raise ValueError('release tool dependency requires an unpinned local module')
                if not owner.get('Main'):
                    continue
                directory = Path(package['Dir']).resolve()
                if Path(owner['Dir']).resolve() != module or not directory.is_relative_to(module):
                    raise ValueError('release tool package escapes the expected Go module')
                for field in GO_BUILD_INPUTS:
                    for name in package.get(field, []):
                        path = source_file(directory, name)
                        names.add(path.relative_to(module).as_posix())
    return {name: sha256(source_file(module, name)) for name in sorted(names)}


def verified_sources(root, recorded, expected, label):
    if set(recorded) != set(expected):
        raise ValueError(label + ' source inventory differs from current recipe')
    for name, digest in recorded.items():
        if sha256(source_file(root, name)) != digest:
            raise ValueError(label + ' source input differs from runtime: ' + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime-output', required=True, type=Path,
                        help='completed builder output containing provenance.json')
    parser.add_argument('--output', required=True, type=Path,
                        help='source assembly/cache; reruns verify existing files')
    parser.add_argument('--archive-output', type=Path,
                        help='new/empty separate directory for deterministic source archive and identity')
    args = parser.parse_args()
    recipe = Path(__file__).resolve().parent
    output = args.output.resolve()
    if output.is_relative_to(recipe):
        parser.error('--output must be outside the recipe directory')
    if args.archive_output and args.archive_output.resolve().is_relative_to(recipe):
        parser.error('--archive-output must be outside the recipe directory')
    assets = recipe.parent.parent / 'src/dem-runtime'
    provenance = json.loads((args.runtime_output / 'provenance.json').read_text())
    verified_sources(recipe, provenance['recipe_sha256'], recipe_sources(), 'recipe')
    verified_sources(assets, provenance['source_sha256'], runtime_sources(assets), 'runner')
    module = recipe.parent.parent / 'src/go'
    tool_sources = release_tool_sources(module)
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
    for name in provenance['recipe_sha256']:
        copies.append((recipe / name, destination / name))
    for name in provenance['source_sha256']:
        copies.append((assets / name, output / 'runtime-source' / name))
    for name in tool_sources:
        copies.append((module / name, output / 'release-tool' / name))
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
    verified_sources(output / 'recipe', provenance['recipe_sha256'], recipe_sources(), 'recipe')
    verified_sources(output / 'runtime-source', provenance['source_sha256'],
                     runtime_sources(assets), 'runner')
    verified_sources(output / 'release-tool', tool_sources, tool_sources, 'release tool')
    inventory = {path.relative_to(output).as_posix(): {'sha256': sha256(path), 'bytes': path.stat().st_size}
                 for path in sorted(expected)}
    write_json(output / 'source-assembly.json', {'format': 1, 'runtime_archive': json.loads(
        (args.runtime_output / 'manifest.json').read_text())['archive']['sha256'], 'files': inventory,
        'release_tool': {'source_root': 'release-tool', 'platforms': ['linux/amd64', 'linux/arm64'],
                         'source_sha256': tool_sources}})
    print('Verified release source assembly: ' + str(output))
    if args.archive_output:
        digest = json.loads((args.runtime_output / 'manifest.json').read_text())['archive']['sha256']
        archive = archive_sources(output, args.archive_output, digest, provenance['inputs']['source_date_epoch'])
        print('Verified release source archive: ' + str(archive))


if __name__ == '__main__':
    main()
