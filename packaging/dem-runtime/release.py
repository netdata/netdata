#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Assemble and publish an independent DEM release; no implicit publication."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

from bundle import run, sha256, validate_url_prefix, write_json
from locked import HTTPS_OPENER, DOWNLOAD_TIMEOUT_SECONDS


ARCHITECTURES = ('amd64', 'arm64')
METADATA_LIMIT = 1 << 20
RECIPE = Path(__file__).resolve().parent


def empty_directory(path):
    path = path.resolve()
    path.mkdir(parents=True, exist_ok=True)
    if any(path.iterdir()):
        raise ValueError('output must be new or empty')
    return path


def version_name(value):
    if not isinstance(value, str) or not re.fullmatch(r'[0-9][0-9A-Za-z.+~]*', value):
        raise ValueError('version must be a native-package version and a single URL path segment')
    return value


def file_identity(path, digest, size):
    if path.is_symlink() or not path.is_file() or path.stat().st_size != size or sha256(path) != digest:
        raise ValueError('artifact identity mismatch: ' + path.name)


def assemble(args):
    """Use the already verified source assembly and exactly the same checkout/tool."""
    output = empty_directory(args.output)
    manifest = json.loads((args.runtime_output / 'manifest.json').read_text())
    version = version_name(manifest['version'])
    if manifest['platform'] != 'linux/' + args.arch:
        raise ValueError('runtime platform differs from package architecture')
    archive_record = manifest['archive']
    archive = args.runtime_output / (archive_record['sha256'] + '.tar.zst')
    file_identity(archive, archive_record['sha256'], archive_record['bytes'])
    sources = json.loads((args.source_archive / 'source-archive.json').read_text())
    if sources['format'] != 1 or sources['runtime_archive'] != archive_record['sha256']:
        raise ValueError('source assembly belongs to another runtime')
    source_name = sources['sha256'] + '.sources.tar.zst'
    if sources['file'] != source_name:
        raise ValueError('unexpected source archive name')
    source = args.source_archive / source_name
    file_identity(source, sources['sha256'], sources['bytes'])
    # Work outside the release artifacts: never upload the expanded executable tree.
    with tempfile.TemporaryDirectory(prefix='dem-native-package-') as temporary:
        temporary = Path(temporary)
        payload = temporary / 'payload'
        run([args.release_tool, 'stage-native', '--manifest', args.runtime_output / 'manifest.json',
             '--archive', archive, '--output', payload])
        for fmt in ('DEB', 'RPM'):
            build = temporary / fmt
            run(['cmake', '-S', RECIPE / 'native', '-B', build,
                 '-DDEM_PAYLOAD_DIR=' + str(payload), '-DDEM_ARCH=' + args.arch,
                 '-DDEM_VERSION=' + version, '-DDEM_RELEASE=' + str(args.package_release),
                 '-DDEM_PACKAGE_FORMAT=' + fmt])
            run(['cpack', '--config', build / 'CPackConfig.cmake'])
            packages = list((build / 'packages').glob('*.' + fmt.lower()))
            if len(packages) != 1:
                raise ValueError('expected one native package per format')
            shutil.copyfile(packages[0], output / packages[0].name)
    for path in (archive, source, args.source_archive / 'source-archive.json'):
        shutil.copyfile(path, output / path.name)
    for name in ('manifest.json', 'provenance.json', 'sbom.cdx.json', 'sources-required.json'):
        shutil.copyfile(args.runtime_output / name, output / name)
    write_json(output / 'artifacts.json', {
        'format': 1, 'architecture': args.arch, 'version': version,
        'package_release': args.package_release,
        'files': {p.name: {'sha256': sha256(p), 'bytes': p.stat().st_size} for p in sorted(output.iterdir())}})


def verified_release(tool, key, path):
    # The Go protocol owner authenticates and strictly decodes before Python uses
    # any field. Publication workspaces are exclusive to this workflow job.
    run([tool, 'verify', '--key', key, '--release', path])
    return json.loads(base64.b64decode(json.loads(path.read_bytes())['payload'], validate=True))


def read_latest(remote):
    # Read the authenticated storage API, not a possibly stale public CDN, when
    # guarding publisher sequence. Only the workflow's serialized writer uses it.
    try:
        stat = json.loads(run(['rclone', 'lsjson', '--stat', remote], capture=True))
    except subprocess.CalledProcessError as error:
        if error.returncode in (3, 4):  # rclone directory/file not found; other failures are fatal.
            return None
        raise
    if stat['IsDir'] or not 0 < stat['Size'] <= METADATA_LIMIT:
        raise ValueError('latest descriptor exceeds the protocol limit')
    body = run(['rclone', 'cat', '--count', str(METADATA_LIMIT + 1), remote], capture=True).encode()
    if len(body) != stat['Size'] or len(body) > METADATA_LIMIT:
        raise ValueError('latest descriptor changed during publication inspection')
    return body


def guard_sequence(current, proposed):
    """Allow identical retry, never regress or reuse publication/version identity."""
    maximum = max((release['sequence'] for release in current.values()), default=0)
    sequence = next(iter(proposed.values()))['sequence']
    if sequence < maximum:
        raise ValueError('publication would regress the shared channel sequence')
    for arch, release in proposed.items():
        old = current.get(arch)
        if old and old['manifest']['platform'] != 'linux/' + arch:
            raise ValueError('current latest has the wrong platform')
        if old and old['sequence'] == sequence and old != release:
            raise ValueError('publication sequence was reused for different content')
        if old and old['manifest']['version'] == release['manifest']['version'] and old != release:
            raise ValueError('runtime version was reused for a different release')


def current_releases(args):
    current = {}
    with tempfile.TemporaryDirectory(prefix='dem-release-current-') as temporary:
        for arch in ARCHITECTURES:
            body = read_latest(args.remote.rstrip('/') + '/dem-runtime/latest/' + arch + '.json')
            if body is None:
                continue
            path = Path(temporary) / (arch + '.json')
            path.write_bytes(body)
            current[arch] = verified_release(args.release_tool, args.public_key, path)
    return current


def public_identity(url, digest, size):
    """Read the public serving path, including CDN, before allowing promotion."""
    actual, count = hashlib.sha256(), 0
    with HTTPS_OPENER.open(url, timeout=DOWNLOAD_TIMEOUT_SECONDS) as response:
        while True:
            block = response.read(min(1024 * 1024, size - count + 1))
            if not block:
                break
            count += len(block)
            if count > size:
                raise ValueError('public artifact exceeds the signed byte count')
            actual.update(block)
    if count != size or actual.hexdigest() != digest:
        raise ValueError('public artifact differs from the signed identity')


def upload(args, path, relative, *, mutable=False):
    command = ['rclone', 'copyto', '--checksum']
    if not mutable:
        command += ['--immutable']
    command += ['--header-upload', 'Cache-Control: no-cache, no-transform' if mutable
                else 'Cache-Control: public, max-age=31536000, immutable, no-transform',
                path, args.remote.rstrip('/') + '/' + relative]
    run(command)


def validate_inventory(directory, arch):
    record = json.loads((directory / 'artifacts.json').read_text())
    if record['format'] != 1 or record['architecture'] != arch:
        raise ValueError('unexpected artifact inventory')
    version_name(record['version'])
    for name, item in record['files'].items():
        if Path(name).name != name or name in ('.', '..'):
            raise ValueError('artifact inventory must contain plain basenames')
        file_identity(directory / name, item['sha256'], item['bytes'])
    actual = {path.name for path in directory.iterdir()}
    if actual != set(record['files']) | {'artifacts.json'}:
        raise ValueError('unexpected files alongside release artifacts')
    return record


def stage(args):
    """Sign both platforms before any external write; preserve native artifacts."""
    output = empty_directory(args.output)
    versions, proposed = set(), {}
    for arch in ARCHITECTURES:
        directory = args.artifacts / ('dem-runtime-' + arch)
        record = validate_inventory(directory, arch)
        version = record['version']
        versions.add(version)
        manifest = json.loads((directory / 'manifest.json').read_text())
        if manifest['platform'] != 'linux/' + arch or manifest['version'] != version:
            raise ValueError('artifact identity differs from manifest')
        sources = json.loads((directory / 'source-archive.json').read_text())
        archive_name = manifest['archive']['sha256'] + '.tar.zst'
        source_name = sources['sha256'] + '.sources.tar.zst'
        if sources['runtime_archive'] != manifest['archive']['sha256'] or sources['file'] != source_name:
            raise ValueError('source assembly belongs to another runtime')
        prefix = 'dem-runtime/' + version + '/' + arch
        destination = output / prefix
        destination.mkdir(parents=True)
        # Native packages are signed/uploaded through the package repository
        # lane. R2 serves the portable archive and corresponding sources only.
        for name in (archive_name, source_name, 'manifest.json', 'source-archive.json',
                     'provenance.json', 'sbom.cdx.json', 'sources-required.json'):
            shutil.copyfile(directory / name, destination / name)
        run([args.release_tool, 'sign', '--key', args.private_key,
             '--manifest', directory / 'manifest.json', '--archive', directory / archive_name,
             '--sources', directory / source_name, '--sequence', str(args.sequence),
             '--archive-url', args.public_base + '/' + prefix + '/' + archive_name,
             '--sources-url', args.public_base + '/' + prefix + '/' + source_name,
             '--output', destination / 'release.json'])
        proposed[arch] = verified_release(args.release_tool, args.public_key, destination / 'release.json')
    if len(versions) != 1:
        raise ValueError('both platforms must belong to one independent release')
    write_json(output / 'publication.json', {'format': 1, 'version': versions.pop(),
                                            'sequence': args.sequence, 'public_base': args.public_base})


def publication(args):
    record = json.loads((args.publication / 'publication.json').read_text())
    if record['format'] != 1 or record['public_base'] != args.public_base:
        raise ValueError('publication destination differs from the signed release staging')
    version = version_name(record['version'])
    proposed, directories = {}, {}
    for arch in ARCHITECTURES:
        directory = args.publication / 'dem-runtime' / version / arch
        release = verified_release(args.release_tool, args.public_key, directory / 'release.json')
        manifest = release['manifest']
        if manifest['platform'] != 'linux/' + arch or manifest['version'] != version or release['sequence'] != record['sequence']:
            raise ValueError('publication identity differs between platforms')
        prefix = args.public_base + '/dem-runtime/' + version + '/' + arch + '/'
        for item, suffix in ((manifest['archive'], '.tar.zst'), (release['sources'], '.sources.tar.zst')):
            name = item['sha256'] + suffix
            if item['url'] != prefix + name:
                raise ValueError('signed publication URL differs from its immutable path')
            file_identity(directory / name, item['sha256'], item['bytes'])
        proposed[arch], directories[arch] = release, directory
    return proposed, directories


def publish(args):
    proposed, directories = publication(args)
    guard_sequence(current_releases(args), proposed)
    for directory in directories.values():
        for path in sorted(directory.iterdir()):
            if path.is_symlink() or not path.is_file():
                raise ValueError('publication only accepts regular artifact files')
            relative = path.relative_to(args.publication).as_posix()
            upload(args, path, relative)
            public_identity(args.public_base + '/' + relative, sha256(path), path.stat().st_size)


def promote(args):
    proposed, directories = publication(args)
    guard_sequence(current_releases(args), proposed)
    # Check every platform before updating either pointer. Promotion is atomic
    # per object, not across architectures. A retry completes partial promotion.
    for arch, release in proposed.items():
        for item in (release['manifest']['archive'], release['sources']):
            public_identity(item['url'], item['sha256'], item['bytes'])
        path = directories[arch] / 'release.json'
        relative = path.relative_to(args.publication).as_posix()
        public_identity(args.public_base + '/' + relative, sha256(path), path.stat().st_size)
    for arch, directory in directories.items():
        path = directory / 'release.json'
        relative = 'dem-runtime/latest/' + arch + '.json'
        upload(args, path, relative, mutable=True)
        public_identity(args.public_base + '/' + relative, sha256(path), path.stat().st_size)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    assemble_parser = commands.add_parser('assemble')
    assemble_parser.add_argument('--arch', required=True, choices=ARCHITECTURES)
    assemble_parser.add_argument('--runtime-output', type=Path, required=True)
    assemble_parser.add_argument('--source-archive', type=Path, required=True)
    assemble_parser.add_argument('--output', type=Path, required=True)
    assemble_parser.add_argument('--package-release', type=int, default=1)
    stage_parser = commands.add_parser('stage')
    stage_parser.add_argument('--artifacts', type=Path, required=True)
    stage_parser.add_argument('--output', type=Path, required=True)
    stage_parser.add_argument('--private-key', type=Path, required=True)
    stage_parser.add_argument('--sequence', type=int, required=True)
    for name in ('publish', 'promote'):
        p = commands.add_parser(name)
        p.add_argument('--publication', type=Path, required=True)
        p.add_argument('--remote', required=True, help='confirmed rclone bucket destination, without dem-runtime suffix')
    for name, p in commands.choices.items():
        p.add_argument('--release-tool', type=Path, required=True)
        if name != 'assemble':
            p.add_argument('--public-key', type=Path, required=True)
            p.add_argument('--public-base', required=True)
    args = parser.parse_args()
    args.release_tool = args.release_tool.resolve()
    if args.command == 'assemble' and args.package_release <= 0:
        parser.error('package release must be positive')
    if args.command == 'stage' and not 0 < args.sequence < 2**64:
        parser.error('sequence must be a positive uint64')
    if args.command != 'assemble':
        validate_url_prefix(args.public_base)
        if not args.public_base:
            parser.error('confirmed public base URL is required')
        args.public_base = args.public_base.rstrip('/')
    if args.command in ('publish', 'promote') and not re.fullmatch(r'[A-Za-z0-9_-]+:[A-Za-z0-9_./-]+', args.remote):
        parser.error('remote must be a named rclone remote and bucket path')
    {'assemble': assemble, 'stage': stage, 'publish': publish, 'promote': promote}[args.command](args)


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)
    except ValueError as error:
        print('runtime release operation failed: ' + str(error), file=sys.stderr)
        sys.exit(1)
    except (OSError, KeyError) as error:
        # Do not print arbitrary network exceptions containing sensitive URLs.
        print('runtime release operation failed: ' + type(error).__name__, file=sys.stderr)
        sys.exit(1)
