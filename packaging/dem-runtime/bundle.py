#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Container-side bundle construction. Only the immutable donor executes this."""
import argparse
import base64
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import struct
import subprocess
import sys
import tarfile
from urllib.parse import urlsplit, urlunsplit


RESERVED = {'chrome', 'fonts.conf', 'prepared.json'}


def credential_free_https(value):
    try:
        url = urlsplit(value)
        valid = (url.scheme == 'https' and url.hostname and url.username is None
                 and url.password is None)
        url.port  # Parsing the authority alone does not validate its port.
        return bool(valid)
    except ValueError:
        return False


def validate_url_prefix(value):
    if value and (not credential_free_https(value) or '?' in value or '#' in value):
        raise ValueError('--url-prefix must be an HTTPS base without credentials, query or fragment')


def display_argument(value):
    # Commands use original argv. Only displays redact credentials/query values.
    value = str(value)
    if '://' in value:
        url = urlsplit(value)
        if url.scheme and url.netloc:
            value = urlunsplit((url.scheme, url.hostname or '', url.path,
                               '<redacted>' if url.query else '', ''))
    return shlex.quote(value)


def run(command, *, cwd=None, capture=False, env=None):
    command = [str(value) for value in command]
    directory = str(cwd or Path.cwd())
    print('+ (cwd=' + display_argument(directory) + ') ' +
          ' '.join(display_argument(value) for value in command), file=sys.stderr, flush=True)
    result = subprocess.run(command, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE if capture else sys.stderr,
                            stderr=subprocess.PIPE if capture else sys.stderr)
    if result.returncode:
        # Do not echo arbitrary command stderr: it can contain a sensitive URL.
        print(f'operation {Path(command[0]).name} failed in {directory}: status {result.returncode}',
              file=sys.stderr)
        raise subprocess.CalledProcessError(result.returncode, command)
    return result.stdout if capture else ''


def sha256(path):
    with Path(path).open('rb') as stream:
        digest = hashlib.sha256()
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(block)
        return digest.hexdigest()


def write_json(path, value):
    Path(path).write_text(json.dumps(value, sort_keys=True, indent=2) + '\n')


def elf_headers(path, machine):
    """Parse and validate actual ELF64 program headers, never search binary strings."""
    size = path.stat().st_size
    with path.open('rb') as stream:
        header = stream.read(64)
        if header[:4] != b'\x7fELF':
            return None
        if len(header) != 64 or header[:7] != b'\x7fELF\x02\x01\x01':
            raise ValueError('expected little-endian ELF64: ' + str(path))
        if struct.unpack_from('<H', header, 18)[0] != machine:
            raise ValueError('wrong ELF architecture: ' + str(path))
        offset = struct.unpack_from('<Q', header, 32)[0]
        entry_size, count = struct.unpack_from('<HH', header, 54)
        if entry_size != 56 or not count or offset < 64 or offset + count * entry_size > size:
            raise ValueError('invalid ELF program header table: ' + str(path))
        stream.seek(offset)
        headers = [struct.unpack('<IIQQQQQQ', stream.read(56)) for _ in range(count)]
        for item in headers:
            if item[2] + item[5] > size or (item[0] == 1 and item[5] > item[6]):
                raise ValueError('ELF segment outside file: ' + str(path))
        # patchelf >=0.18 corrupted this invariant on the pinned Node arm64 input.
        for item in headers:
            if item[0] == 6 and not any(load[0] == 1 and
                                        load[3] <= item[3] and item[3] + item[6] <= load[3] + load[6]
                                        for load in headers):
                raise ValueError('ELF PHDR not covered by LOAD: ' + str(path))
        interps = [item for item in headers if item[0] == 3]
        if len(interps) > 1:
            raise ValueError('multiple ELF interpreters: ' + str(path))
        return headers


def interpreter_binding(path, root, machine, expected):
    headers = elf_headers(path, machine)
    if headers is None:
        return None
    for item in headers:
        if item[0] != 3:
            continue
        with path.open('rb') as stream:
            stream.seek(item[2])
            actual = stream.read(item[5])
        if actual != expected:
            raise ValueError('ELF interpreter slot mismatch: ' + str(path))
        return {'path': path.relative_to(root).as_posix(), 'offset': item[2],
                'expected': base64.b64encode(actual).decode('ascii')}
    return None


def regular_files(root):
    return sorted(path for path in root.rglob('*') if path.is_file() and not path.is_symlink())


def package_archive(root, output, epoch):
    paths = sorted(root.rglob('*'))
    expanded = 0
    tar_path = output / 'runtime.tar'
    with tarfile.open(tar_path, 'w', format=tarfile.PAX_FORMAT) as archive:
        for path in paths:
            name = path.relative_to(root).as_posix()
            if name.split('/')[0] in RESERVED or any(c in name for c in '\\\r\n\x00'):
                raise ValueError('reserved/invalid archive path: ' + name)
            info = tarfile.TarInfo(name)
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mtime = epoch
            if path.is_symlink():
                target = os.readlink(path)
                if os.path.isabs(target) or any(c in target for c in '\\\r\n\x00') or not path.resolve(
                        strict=True).is_relative_to(root.resolve()):
                    raise ValueError('escaping archive symlink: ' + name)
                info.type, info.mode, info.linkname = tarfile.SYMTYPE, 0o777, target
                archive.addfile(info)
            elif path.is_dir():
                info.type, info.mode = tarfile.DIRTYPE, 0o555
                archive.addfile(info)
            elif path.is_file():
                info.mode = 0o555 if path.stat().st_mode & 0o111 else 0o444
                info.size = path.stat().st_size
                expanded += info.size
                with path.open('rb') as stream:
                    archive.addfile(info, stream)
            else:
                raise ValueError('unsupported archive file: ' + name)
    compressed = output / 'runtime.tar.gz'
    with tar_path.open('rb') as source, compressed.open('wb') as destination:
        with gzip.GzipFile(filename='', mode='wb', fileobj=destination, mtime=epoch, compresslevel=9) as zipped:
            shutil.copyfileobj(source, zipped)
    result = {'sha256': sha256(compressed), 'bytes': compressed.stat().st_size,
              'tar_bytes': tar_path.stat().st_size, 'expanded_bytes': expanded, 'entries': len(paths)}
    tar_path.unlink()  # Task-created intermediate, never a source file.
    return compressed, result


def copy_libraries(root, machine, triplet):
    library = root / 'lib'
    library.mkdir()
    seeds = [path for path in regular_files(root)
             if any(item[0] == 2 for item in (elf_headers(path, machine) or []))]
    # NSS dynamically loads these; they are not all represented in DT_NEEDED.
    for pattern in ('libnssckbi.so', 'libfreebl*.so', 'libsoftokn3.so'):
        matches = sorted(Path('/usr/lib', triplet).glob(pattern))
        if not matches:
            raise ValueError('missing NSS runtime library: ' + pattern)
        seeds.extend(matches)
    closure = {}
    for seed in seeds:
        result = run(['ldd', seed], capture=True)
        if 'not found' in result:
            raise ValueError('unresolved library dependency: ' + str(seed))
        found = [Path(match) for match in re.findall(r'(/[^\s()]+)', result)]
        if not seed.is_relative_to(root):
            found.append(seed)
        for source in found:
            if not source.is_file() or source.is_relative_to(root):
                continue
            previous = closure.get(source.name)
            if previous and sha256(previous) != sha256(source):
                raise ValueError('colliding private library basename: ' + source.name)
            closure[source.name] = source.resolve()
    for name, source in sorted(closure.items()):
        shutil.copyfile(source, library / name)
        (library / name).chmod(0o755)
    return closure


def donor_notices(root, files):
    owners = set()
    for path in sorted(set(files)):
        # dpkg's database can retain /lib names on merged-/usr images.
        names = [str(path)]
        if str(path).startswith('/usr/lib/'):
            names.append(str(path)[4:])
        owner = None
        for name in names:
            try:
                owner = run(['dpkg-query', '-S', name], capture=True).splitlines()[0].rsplit(': ', 1)[0]
                break
            except subprocess.CalledProcessError:
                pass
        if not owner:
            raise ValueError('no donor package owns ' + str(path))
        owners.add(owner)
    notices = root / 'notices' / 'debian'
    notices.mkdir(parents=True)
    components = []
    for owner in sorted(owners):
        fields = run(
            ['dpkg-query', '-W',
             '-f=${Package}\t${Version}\t${Architecture}\t${source:Package}\t${source:Version}', owner],
            capture=True).split('\t')
        name, version, architecture, source, source_version = fields
        copyright_path = Path('/usr/share/doc', name, 'copyright')
        if not copyright_path.is_file():
            raise ValueError('missing package copyright: ' + owner)
        shutil.copyfile(copyright_path, notices / (name + '.copyright'))
        components.append({'type': 'library', 'name': name, 'version': version,
                           'purl': f'pkg:deb/debian/{name}@{version}?arch={architecture}&distro=trixie',
                           'properties': [{'name': 'source-package', 'value': source},
                                          {'name': 'source-version', 'value': source_version}]})
    return components


def prepare(args):
    validate_url_prefix(args.url_prefix)
    recipe, assets, output = Path('/recipe'), Path('/assets'), Path('/out')
    from locked import check_inventory, download, source_key
    inputs = json.loads((recipe / 'inputs.json').read_text())
    debian_lock = json.loads((recipe / 'locks' / (args.arch + '.json')).read_text())
    check_inventory(debian_lock)
    arch = inputs['architectures'][args.arch]
    contract = json.loads((assets / 'manifest.json').read_text())
    if contract['chromium'] != debian_lock['chromium'] or contract['chromium']['source'] != 'debian':
        raise ValueError('Debian browser lock differs from the runner contract')
    if int(inputs['node_version'].split('.')[0]) < contract['node_major_min']:
        raise ValueError('Node input is below runner contract')
    work = Path('/tmp/dem-build')
    work.mkdir()
    root = work / 'payload'
    root.mkdir()
    node_name = f'node-v{inputs["node_version"]}-linux-{arch["node_arch"]}'
    node_tar = work / 'node.tar.xz'
    download({'url': f'https://nodejs.org/dist/v{inputs["node_version"]}/{node_name}.tar.xz',
              'sha256': arch['node_sha256'], 'name': node_tar.name}, work)
    with tarfile.open(node_tar) as source:
        source.extractall(work, filter='data')
    node = work / node_name
    patch_tar = work / 'patchelf.tar.gz'
    download({'url': f'https://github.com/NixOS/patchelf/releases/download/{inputs["patchelf_version"]}/'
                     f'patchelf-{inputs["patchelf_version"]}-{arch["patchelf_arch"]}.tar.gz',
              'sha256': arch['patchelf_sha256'], 'name': patch_tar.name}, work)
    patch_dir = work / 'patchelf'
    patch_dir.mkdir()
    with tarfile.open(patch_tar) as source:
        source.extractall(patch_dir, filter='data')
    patch = patch_dir / 'bin' / 'patchelf'
    if run([patch, '--version'], capture=True).strip() != 'patchelf ' + inputs['patchelf_version']:
        raise ValueError('unexpected patchelf version')
    dependencies = root / 'dependencies'
    dependencies.mkdir()
    for name in ('package.json', 'package-lock.json'):
        shutil.copyfile(assets / name, dependencies / name)
    env = dict(os.environ, PATH=str(node / 'bin') + ':' + os.environ['PATH'],
               PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD='1', npm_config_cache=str(work / 'npm-cache'))
    run([node / 'bin/npm', 'ci', '--omit=dev', '--ignore-scripts', '--no-audit', '--no-fund'],
        cwd=dependencies, env=env)
    components = []
    npm_lock = json.loads((assets / 'package-lock.json').read_text())
    for name, version in contract['packages'].items():
        installed = json.loads((dependencies / 'node_modules' / name / 'package.json').read_text())
        if installed['version'] != version:
            raise ValueError('npm contract mismatch: ' + name)
    for path, info in sorted(npm_lock['packages'].items()):
        if not path or info.get('dev'):
            continue
        name = path.rsplit('node_modules/', 1)[1]
        components.append({'type': 'library', 'name': name, 'version': info['version'],
                           'purl': 'pkg:npm/' + name.replace('@', '%40') + '@' + info['version'],
                           'properties': [{'name': 'npm-integrity', 'value': info['integrity']}],
                           'licenses': [{'license': {'name': info.get('license', 'NOASSERTION')}}],
                           'externalReferences': [{'type': 'distribution', 'url': info['resolved']}]})
    browsers = root / 'browsers'
    browsers.mkdir()
    browser_name = 'chromium-debian'
    source_browser = Path('/usr/lib/chromium')
    shutil.copytree(source_browser, browsers / browser_name, symlinks=True)
    browser = browsers / browser_name / 'chromium'
    version = run([browser, '--version'], capture=True).strip()
    if version != 'Chromium ' + contract['chromium']['version']:
        raise ValueError('unexpected Debian Chromium identity: ' + version)
    forbidden = [str(p) for p in source_browser.rglob(
        '*') if 'widevine' in p.name.lower() or 'screen_ai' in p.name.lower()]
    if forbidden:
        raise ValueError('unexpected opaque component payload: ' + repr(forbidden))
    # DEM disables video recording; no FFmpeg artifact is shipped.
    browser_sources = regular_files(source_browser)
    (root / 'node/bin').mkdir(parents=True)
    shutil.copyfile(node / 'bin/node', root / 'node/bin/node')
    (root / 'node/bin/node').chmod(0o755)
    shutil.copyfile(node / 'LICENSE', root / 'node/LICENSE')
    font_root = Path('/usr/share/fonts')
    font_files = [path for path in regular_files(font_root)
                  if path.name.lower().endswith(('.ttf', '.ttc', '.otf', '.pfa', '.pfb', '.afm',
                                                '.pcf', '.pcf.gz', '.bdf'))]
    for source in font_files:
        destination = root / 'fonts' / source.relative_to(font_root)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
    closure = copy_libraries(root, arch['elf_machine'], arch['triplet'])
    components.extend(donor_notices(root, list(closure.values()) + font_files +
                      browser_sources + regular_files(Path('/usr/share/common-licenses'))))
    notices = root / 'notices'
    shutil.copytree('/usr/share/common-licenses', notices / 'common-licenses', symlinks=False)
    required_sources = {}
    for component in components:
        if component.get('purl', '').startswith('pkg:deb/'):
            properties = {p['name']: p['value'] for p in component['properties']}
            key = source_key(properties['source-package'], properties['source-version'])
            if key not in debian_lock['sources']:
                raise ValueError('missing exact redistributed source record: ' + key)
            required_sources[key] = debian_lock['sources'][key]
    write_json(output / 'sources-required.json', required_sources)
    # Read the embedded Chromium notices before relocation. The root-only build
    # donor uses no sandbox solely to export chrome://credits; runtime launches
    # remain sandboxed and this operation is not execution qualification.
    credits_script = work / 'credits.cjs'
    credits_script.write_text(
        'const {chromium}=require(process.argv[2]); '
        '(async()=>{const b=await chromium.launch({executablePath:process.argv[3],headless:true});'
        "const p=await b.newPage();await p.goto('chrome://credits');"
        'console.log(await p.content());await b.close();})()'
        '.catch(e=>{console.error(e);process.exit(1)});\n')
    credits = run([node / 'bin/node', credits_script, dependencies / 'node_modules/playwright', browser], capture=True)
    if 'licens' not in credits.lower() or len(credits) < 10000:
        raise ValueError('Chromium third-party notices export is incomplete')
    (notices / 'chromium-credits.html').write_text(credits)
    (notices / 'README').write_text(
        'Node licenses: node/LICENSE. npm notices remain in dependencies/node_modules.\n'
        'Chromium embedded third-party licenses: notices/chromium-credits.html.\n'
        'Private library/font copyrights and source-package identities: notices/debian and sbom.cdx.json.\n'
        'The runtime does not contain patchelf, npm CLI, compilers, or donor package managers.\n')
    capacity = inputs['interpreter_capacity']
    if capacity != 4096:
        raise ValueError('unsupported interpreter capacity')
    expected = b'/' + b'N' * (capacity - 2) + b'\0'
    bindings = []
    for path in regular_files(root):
        headers = elf_headers(path, arch['elf_machine'])
        if not headers or path.name in (arch['loader'], 'libc.so.6'):
            continue
        if not any(item[0] == 2 for item in headers):
            continue
        rpath = '$ORIGIN:$ORIGIN/' + os.path.relpath(root / 'lib', path.parent)
        command = [patch, '--force-rpath', '--set-rpath', rpath]
        if any(item[0] == 3 for item in headers):
            command += ['--set-interpreter', expected[:-1].decode()]
        run(command + [path])
        binding = interpreter_binding(path, root, arch['elf_machine'], expected)
        if binding:
            bindings.append(binding)
    layout = {'node': 'node/bin/node', 'dependencies': 'dependencies',
              'browser': browser.relative_to(root).as_posix(), 'loader': 'lib/' + arch['loader'], 'fonts': 'fonts'}
    if not {layout['node'], layout['browser']}.issubset({binding['path'] for binding in bindings}):
        raise ValueError('missing executable binding')
    # A temporary copy verifies real execution without binding the archived files.
    probe = work / "probe with spaces & ' quote"
    shutil.copytree(root, probe, symlinks=True)
    for binding in bindings:
        target = (str(probe / layout['loader'])).encode() + b'\0'
        with (probe / binding['path']).open('r+b') as stream:
            stream.seek(binding['offset'])
            stream.write(target.ljust(capacity, b'\0'))
    if run([probe / layout['node'], '--version'], capture=True).strip() != 'v' + inputs['node_version']:
        raise ValueError('prepared Node execution mismatch')
    if run([probe / layout['browser'], '--version'],
           capture=True).strip() != 'Chromium ' + contract['chromium']['version']:
        raise ValueError('prepared Chromium execution mismatch')
    reexec = (
        "const cp=require('node:child_process');"
        "if(cp.execFileSync(process.execPath,['--version'],{encoding:'utf8'}).trim()!==process.version)"
        'process.exit(1)')
    run([probe / layout['node'], '-e', reexec])
    # Exercise the kernel's exact 4095-byte interpreter path limit. Components
    # stay below NAME_MAX; only the final absolute path consumes PATH_MAX.
    long_parent = str(work / 'long-prefix')
    remaining = capacity - 1 - len(arch['loader']) - 1 - len(long_parent)
    while remaining > 0:
        size = min(200, remaining - 1)
        if size < 1:
            raise ValueError('cannot construct PATH_MAX probe')
        long_parent += '/' + 'n' * size
        remaining -= size + 1
    Path(long_parent).mkdir(parents=True)
    long_loader = Path(long_parent) / arch['loader']
    shutil.copyfile(probe / layout['loader'], long_loader)
    long_loader.chmod(0o755)
    # Never reopen executed inodes for writing: Linux can still hold executable
    # mappings briefly after the process leader is reaped (ETXTBSY).
    probe = work / 'path-max-probe'
    shutil.copytree(root, probe, symlinks=True)
    target = str(long_loader).encode() + b'\0'
    if len(target) != capacity:
        raise ValueError('PATH_MAX probe length mismatch')
    for binding in bindings:
        with (probe / binding['path']).open('r+b') as stream:
            stream.seek(binding['offset'])
            stream.write(target)
    run([probe / layout['node'], '-e', reexec])
    run([probe / layout['browser'], '--version'])
    components.append({'type': 'application', 'name': 'node', 'version': inputs['node_version']})
    write_json(root / 'sbom.cdx.json', {'bomFormat': 'CycloneDX',
               'specVersion': '1.5', 'version': 1, 'components': components})
    provenance = {'inputs': inputs, 'platform': 'linux/' + args.arch,
                  'runner_manifest': contract,
                  'source_sha256': {name: sha256(assets / name)
                                    for name in ('manifest.json', 'package.json', 'package-lock.json')},
                  'recipe_sha256': {name: sha256(recipe / name) for name in
                                    ('build.py', 'bundle.py', 'locked.py', 'sources.py', 'refresh-lock.py',
                                     'inputs.json', 'Dockerfile', 'locks/' + args.arch + '.json')},
                  'debian_sources': required_sources,
                  'library_sources': {name: {'path': str(source), 'sha256': sha256(source)}
                                      for name, source in sorted(closure.items())}}
    write_json(root / 'provenance.json', provenance)
    write_json(root / 'files.json',
               {path.relative_to(root).as_posix(): {'sha256': sha256(path),
                                                    'bytes': path.stat().st_size} for path in regular_files(root)})
    compressed, archive = package_archive(root, output, inputs['source_date_epoch'])
    relative = f'dem-runtime/{inputs["version"]}/{args.arch}/{archive["sha256"]}.tar.gz'
    archive['url'] = args.url_prefix.rstrip('/') + '/' + relative if args.url_prefix else ''
    manifest = {'format': 1, 'version': inputs['version'], 'platform': 'linux/' + args.arch,
                'archive': archive, 'layout': layout, 'bindings': bindings}
    final = output / (archive['sha256'] + '.tar.gz')
    compressed.rename(final)
    write_json(output / 'manifest.json', manifest)
    write_json(output / 'provenance.json', provenance)
    shutil.copyfile(root / 'sbom.cdx.json', output / 'sbom.cdx.json')
    print(json.dumps({'archive': final.name, 'manifest': 'manifest.json', 'bounds': archive}), flush=True)
    for path in output.iterdir():
        os.chown(path, int(os.environ['OUTPUT_UID']), int(os.environ['OUTPUT_GID']))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', required=True, choices=('arm64', 'amd64'))
    parser.add_argument('--url-prefix', default='')
    try:
        prepare(parser.parse_args())
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)
