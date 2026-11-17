#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Assemble and publish the exact companion of an Agent release."""
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
import tarfile
from urllib.error import HTTPError
from urllib.request import Request

from bundle import read_release_info, run, sha256, validate_release_info, validate_url_prefix, write_json
from locked import HTTPS_OPENER, DOWNLOAD_TIMEOUT_SECONDS


ARCHITECTURES = ('amd64', 'arm64')
METADATA_LIMIT = 1 << 20
PUBLIC_KEY_LIMIT = 1 << 20
PUBLIC_KEY_URL = 'https://repository.netdata.cloud/netdatabot.gpg.key'
PUBLIC_USER_AGENT = 'Netdata-DEM-release/1'


def empty_directory(path):
    path = path.resolve()
    path.mkdir(parents=True, exist_ok=True)
    if any(path.iterdir()):
        raise ValueError('output must be new or empty')
    return path


def file_identity(path, digest, size):
    if path.is_symlink() or not path.is_file() or path.stat().st_size != size or sha256(path) != digest:
        raise ValueError('artifact identity mismatch: ' + path.name)


def archive_native(payload, output, epoch):
    """Transport the existing Go-prepared tree to the normal Agent CPack jobs."""
    tar_path = output / 'native-payload.tar'
    with tarfile.open(tar_path, 'w', format=tarfile.PAX_FORMAT) as archive:
        for path in sorted(payload.rglob('*')):
            info = archive.gettarinfo(path, arcname=path.relative_to(payload).as_posix())
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mtime = epoch
            if info.isfile():
                with path.open('rb') as stream:
                    archive.addfile(info, stream)
            else:
                archive.addfile(info)
    destination = output / 'native-payload.tar.zst'
    run(['zstd', '-19', '--single-thread', '--zstd=wlog=23', '--check',
         '--no-progress', '-o', destination, tar_path])
    tar_path.unlink()  # Task-created intermediate, never a source file.


def runtime_identity(directory, arch):
    agent = read_release_info(directory / 'release-info')
    provenance = json.loads((directory / 'provenance.json').read_text())
    manifest = json.loads((directory / 'manifest.json').read_text())
    if (provenance['agent'] != agent or provenance['platform'] != 'linux/' + arch or
            manifest['platform'] != 'linux/' + arch or manifest['version'] != agent['version']):
        raise ValueError('runtime manifest, provenance and Agent release identity differ')
    return agent, manifest, provenance


def assemble(args):
    """Keep portable/source artifacts and a prebound tree for normal Agent packaging."""
    output = empty_directory(args.output)
    agent, manifest, provenance = runtime_identity(args.runtime_output, args.arch)
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
    with tempfile.TemporaryDirectory(prefix='dem-native-payload-') as temporary:
        payload = Path(temporary) / 'payload'
        run([args.release_tool, 'stage-native', '--manifest', args.runtime_output / 'manifest.json',
             '--archive', archive, '--output', payload])
        native = json.loads((payload / 'native.json').read_text())
        if (native['format'] != 1 or native['path'] != '/usr/lib/netdata/dem-runtime' or
                native['manifest'] != manifest or
                (payload / 'release-info').read_bytes() != (args.runtime_output / 'release-info').read_bytes()):
            raise ValueError('native stage differs from the exact portable runtime')
        archive_native(payload, output, provenance['inputs']['source_date_epoch'])
    for path in (archive, source, args.source_archive / 'source-archive.json'):
        shutil.copyfile(path, output / path.name)
    for name in ('manifest.json', 'provenance.json', 'sbom.cdx.json', 'sources-required.json', 'release-info'):
        shutil.copyfile(args.runtime_output / name, output / name)
    write_json(output / 'artifacts.json', {
        'format': 2, 'architecture': args.arch, 'agent': agent,
        'files': {p.name: {'sha256': sha256(p), 'bytes': p.stat().st_size} for p in sorted(output.iterdir())}})


def verified_release(tool, key, path):
    # The Go protocol owner authenticates and strictly decodes before Python uses
    # any field. Publication workspaces are exclusive to this workflow job.
    run([tool, 'verify', '--key', key, '--release', path])
    return json.loads(base64.b64decode(json.loads(path.read_bytes())['payload'], validate=True))


def fetch_public_key(args):
    """Acquire current online trust from the fixed package-signing authority."""
    request = Request(PUBLIC_KEY_URL, headers={'User-Agent': PUBLIC_USER_AGENT})
    with HTTPS_OPENER.open(request, timeout=DOWNLOAD_TIMEOUT_SECONDS) as response:
        body = response.read(PUBLIC_KEY_LIMIT + 1)
    if not 0 < len(body) <= PUBLIC_KEY_LIMIT:
        raise ValueError('public keyring exceeds the protocol limit or is empty')
    # No fallback/cache: failed acquisition leaves no partially written keyring.
    args.output.write_bytes(body)


def read_stored_descriptor(remote):
    # Authenticate retry state from storage rather than a possibly stale CDN.
    # A bucket stat can report an absent object as a directory. List its parent
    # with a trailing slash to avoid probing an object as the filesystem root.
    parent, name = remote.rsplit('/', 1)
    entries = json.loads(run(['rclone', 'lsjson', '--files-only', '--include', '/' + name,
                              '--no-mimetype', '--no-modtime', parent + '/'], capture=True))
    for stat in entries:
        if stat['Path'] != name:
            continue
        if stat['IsDir'] or not 0 < stat['Size'] <= METADATA_LIMIT:
            raise ValueError('stored descriptor exceeds the protocol limit')
        body = run(['rclone', 'cat', '--count', str(METADATA_LIMIT + 1), remote], capture=True).encode()
        if len(body) != stat['Size'] or len(body) > METADATA_LIMIT:
            raise ValueError('stored descriptor changed during publication inspection')
        return body
    return None  # Only a successful listing can establish absence.


def signed_agent(release, arch):
    agent = validate_release_info(release['agent'])
    if (release['format'] != 2 or release.get('sequence', 0) != 0 or
            release['manifest']['version'] != agent['version'] or
            release['manifest']['platform'] != 'linux/' + arch):
        raise ValueError('signed release does not match its Agent identity or architecture')
    return agent


def public_identity(url, digest, size):
    """Read the public serving path, including CDN, before completing publication."""
    actual, count = hashlib.sha256(), 0
    request = Request(url, headers={'User-Agent': PUBLIC_USER_AGENT})
    with HTTPS_OPENER.open(request, timeout=DOWNLOAD_TIMEOUT_SECONDS) as response:
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


def upload(args, path, relative):
    parent, name = relative.rsplit('/', 1)
    if path.name != name:
        raise ValueError('publication upload must preserve its filename')
    # The bucket is provisioned by SRE; publishing needs only object-level access.
    # rclone 1.60 copyto ignores --immutable for changed contents. Directory copy
    # enforces it; select only this file so descriptors cannot upload early.
    run(['rclone', 'copy', '--s3-no-check-bucket', '--checksum', '--immutable', '--no-traverse',
         '--include', '/' + name, '--header-upload',
         'Cache-Control: public, max-age=31536000, immutable, no-transform',
         path.parent, args.remote.rstrip('/') + '/' + parent + '/'])


def validate_inventory(directory, arch):
    record = json.loads((directory / 'artifacts.json').read_text())
    if record['format'] != 2 or record['architecture'] != arch:
        raise ValueError('unexpected artifact inventory')
    validate_release_info(record['agent'])
    for name, item in record['files'].items():
        if Path(name).name != name or name in ('.', '..'):
            raise ValueError('artifact inventory must contain plain basenames')
        file_identity(directory / name, item['sha256'], item['bytes'])
    actual = {path.name for path in directory.iterdir()}
    if actual != set(record['files']) | {'artifacts.json'}:
        raise ValueError('unexpected files alongside release artifacts')
    agent, _, _ = runtime_identity(directory, arch)
    if record['agent'] != agent:
        raise ValueError('artifact inventory Agent identity differs from the runtime')
    return record


def stage(args):
    """Sign both exact companions before any external write."""
    output = empty_directory(args.output)
    inventories = {arch: validate_inventory(args.artifacts / ('dem-runtime-' + arch), arch)
                   for arch in ARCHITECTURES}
    agent = inventories[ARCHITECTURES[0]]['agent']
    if any(record['agent'] != agent for record in inventories.values()):
        raise ValueError('both platforms must belong to the same Agent release')
    for arch in ARCHITECTURES:
        directory = args.artifacts / ('dem-runtime-' + arch)
        _, manifest, _ = runtime_identity(directory, arch)
        sources = json.loads((directory / 'source-archive.json').read_text())
        archive_name = manifest['archive']['sha256'] + '.tar.zst'
        source_name = sources['sha256'] + '.sources.tar.zst'
        if sources['runtime_archive'] != manifest['archive']['sha256'] or sources['file'] != source_name:
            raise ValueError('source assembly belongs to another runtime')
        prefix = 'dem-runtime/' + agent['channel'] + '/' + agent['version'] + '/' + arch
        destination = output / prefix
        destination.mkdir(parents=True)
        # Native payload transport belongs only to normal Agent packaging jobs.
        for name in (archive_name, source_name, 'manifest.json', 'source-archive.json',
                     'provenance.json', 'sbom.cdx.json', 'sources-required.json', 'release-info'):
            shutil.copyfile(directory / name, destination / name)
        run([args.release_tool, 'sign', '--key', args.signing_key, '--public-key', args.public_key,
             '--manifest', directory / 'manifest.json', '--archive', directory / archive_name,
             '--sources', directory / source_name, '--agent-version', agent['version'],
             '--agent-channel', agent['channel'], '--agent-source', agent['source'],
             '--archive-url', args.public_base + '/' + prefix + '/' + archive_name,
             '--sources-url', args.public_base + '/' + prefix + '/' + source_name,
             '--output', destination / 'release.json'])
        signed = verified_release(args.release_tool, args.public_key, destination / 'release.json')
        if signed_agent(signed, arch) != agent:
            raise ValueError('signed descriptor differs from the built Agent identity')
    write_json(output / 'publication.json', {'format': 2, 'agent': agent, 'public_base': args.public_base})


def publication(args):
    record = json.loads((args.publication / 'publication.json').read_text())
    if record['format'] != 2 or record['public_base'] != args.public_base:
        raise ValueError('publication destination differs from the signed release staging')
    agent = validate_release_info(record['agent'])
    proposed, directories = {}, {}
    for arch in ARCHITECTURES:
        directory = args.publication / 'dem-runtime' / agent['channel'] / agent['version'] / arch
        release = verified_release(args.release_tool, args.public_key, directory / 'release.json')
        if signed_agent(release, arch) != agent:
            raise ValueError('publication identity differs between platforms')
        prefix = args.public_base + '/dem-runtime/' + agent['channel'] + '/' + agent['version'] + '/' + arch + '/'
        for item, suffix in ((release['manifest']['archive'], '.tar.zst'), (release['sources'], '.sources.tar.zst')):
            name = item['sha256'] + suffix
            if item['url'] != prefix + name:
                raise ValueError('signed publication URL differs from its immutable path')
            file_identity(directory / name, item['sha256'], item['bytes'])
        if read_release_info(directory / 'release-info') != agent:
            raise ValueError('publication release-info differs from its signed identity')
        proposed[arch], directories[arch] = release, directory
    return proposed, directories


def canonical_descriptors(args, proposed, directories, temporary):
    """Authenticate immutable retry state without rewriting the staging workspace."""
    paths, existing = {}, set()
    for arch, directory in directories.items():
        staged = directory / 'release.json'
        relative = staged.relative_to(args.publication).as_posix()
        body = read_stored_descriptor(args.remote.rstrip('/') + '/' + relative)
        if body is None:
            paths[arch] = staged
            continue
        canonical = temporary / (arch + '.json')
        canonical.write_bytes(body)
        if verified_release(args.release_tool, args.public_key, canonical) != proposed[arch]:
            raise ValueError('immutable release payload differs from proposed release')
        paths[arch] = canonical
        existing.add(arch)
    return paths, existing


def publish(args):
    proposed, directories = publication(args)
    with tempfile.TemporaryDirectory(prefix='dem-release-publish-') as temporary:
        # GPG signatures vary between attempts. Reuse only a trusted immutable
        # descriptor with the same complete payload, never replace its bytes.
        canonical, existing = canonical_descriptors(args, proposed, directories, Path(temporary))
        for arch, directory in directories.items():
            for path in sorted(directory.iterdir()):
                if path.is_symlink() or not path.is_file():
                    raise ValueError('publication only accepts regular artifact files')
                if path.name == 'release.json':
                    continue
                relative = path.relative_to(args.publication).as_posix()
                upload(args, path, relative)
                public_identity(args.public_base + '/' + relative, sha256(path), path.stat().st_size)
        for arch, path in canonical.items():
            relative = (directories[arch] / 'release.json').relative_to(args.publication).as_posix()
            if arch not in existing:
                upload(args, path, relative)
            public_identity(args.public_base + '/' + relative, sha256(path), path.stat().st_size)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    key_parser = commands.add_parser('fetch-public-key')
    key_parser.add_argument('--output', type=Path, required=True)
    assemble_parser = commands.add_parser('assemble')
    assemble_parser.add_argument('--arch', required=True, choices=ARCHITECTURES)
    assemble_parser.add_argument('--runtime-output', type=Path, required=True)
    assemble_parser.add_argument('--source-archive', type=Path, required=True)
    assemble_parser.add_argument('--output', type=Path, required=True)
    stage_parser = commands.add_parser('stage')
    stage_parser.add_argument('--artifacts', type=Path, required=True)
    stage_parser.add_argument('--output', type=Path, required=True)
    stage_parser.add_argument('--signing-key', required=True, help='imported package-signing GPG fingerprint')
    publish_parser = commands.add_parser('publish')
    publish_parser.add_argument('--publication', type=Path, required=True)
    publish_parser.add_argument('--remote', required=True,
                                help='confirmed rclone bucket destination, without dem-runtime suffix')
    for name, p in commands.choices.items():
        if name == 'fetch-public-key':
            continue
        p.add_argument('--release-tool', type=Path, required=True)
        if name != 'assemble':
            p.add_argument('--public-key', type=Path, required=True)
            p.add_argument('--public-base', required=True)
    args = parser.parse_args()
    if args.command == 'fetch-public-key':
        fetch_public_key(args)
        return
    args.release_tool = args.release_tool.resolve()
    if args.command != 'assemble':
        validate_url_prefix(args.public_base)
        if not args.public_base:
            parser.error('confirmed public base URL is required')
        args.public_base = args.public_base.rstrip('/')
    if args.command == 'publish' and not re.fullmatch(r'[A-Za-z0-9_-]+:[A-Za-z0-9_./-]+', args.remote):
        parser.error('remote must be a named rclone remote and bucket path')
    {'assemble': assemble, 'stage': stage, 'publish': publish}[args.command](args)


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)
    except ValueError as error:
        print('runtime release operation failed: ' + str(error), file=sys.stderr)
        sys.exit(1)
    except HTTPError as error:
        error.close()
        print(f'runtime release operation failed: HTTP {error.code}', file=sys.stderr)
        sys.exit(1)
    except (OSError, KeyError) as error:
        # Do not print arbitrary network exceptions containing sensitive URLs.
        print('runtime release operation failed: ' + type(error).__name__, file=sys.stderr)
        sys.exit(1)
