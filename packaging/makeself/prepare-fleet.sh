#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Prepare new fleet images without changing an installed Agent or the source package.
set -eu
run() {
  printf >&2 '%q > ' "$PWD"
  printf >&2 '%q ' "$@"
  printf >&2 '\n'
  if "$@"; then return 0; else
    local status=$?
    printf >&2 'ERROR: command failed (%s), working directory %q: ' "$status" "$PWD"
    printf >&2 '%q ' "$@"
    printf >&2 '\n'
    return "$status"
  fi
}
run python3 - "$@" <<'PY'
import argparse
import copy
import gzip
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shlex
import shutil
import stat
import struct
import subprocess
import sys
import tarfile
import tempfile

PLUGIN = 'usr/libexec/netdata/plugins.d/'
CONF = 'usr/lib/netdata/conf.d/'
# Only dedicated stock paths belong here. Shared helpers and unknown files survive.
CAPS = {
    'apps': ([PLUGIN + 'apps.plugin', CONF + 'apps_groups.conf'], []),
    'debugfs': ([PLUGIN + 'debugfs.plugin'], []),
    'network': ([PLUGIN + 'network-viewer.plugin'], ['apps']),
    'containers': ([PLUGIN + 'cgroup-name', PLUGIN + 'cgroup-network',
                    PLUGIN + 'cgroup-network-helper.sh', PLUGIN + 'get-kubernetes-labels.sh'], []),
    'go': ([PLUGIN + 'go.d.plugin', PLUGIN + 'snmp-trap-profile-gen',
            PLUGIN + 'local-listeners', CONF + 'go.d.conf', CONF + 'go.d'], []),
    'mcp': (['bin/nd-mcp'], []),
    'journal': ([PLUGIN + 'systemd-journal.plugin',
                 CONF + 'schema.d/systemd-journal%3Amonitored-directories.json'], []),
    'otel': ([PLUGIN + 'otel-plugin', CONF + 'otel.yaml', CONF + 'otel.d'], []),
    'netflow': ([PLUGIN + 'netflow-plugin', CONF + 'netflow.yaml',
                 CONF + 'topology-ip-intel.yaml', 'usr/share/netdata/topology-ip-intel'], []),
    'dashboard': (['usr/share/netdata/web'], []),
    'python': ([PLUGIN + 'python.d.plugin', 'usr/libexec/netdata/python.d',
                CONF + 'python.d.conf', CONF + 'python.d'], []),
    'charts': ([PLUGIN + 'charts.d.plugin', PLUGIN + 'charts.d.dryrun-helper.sh',
                'usr/libexec/netdata/charts.d', CONF + 'charts.d.conf', CONF + 'charts.d'], []),
    'scripts': ([PLUGIN + 'scripts.d.plugin', CONF + 'scripts.d.conf', CONF + 'scripts.d'], []),
    'perf': ([PLUGIN + 'perf.plugin'], []),
    'slabinfo': ([PLUGIN + 'slabinfo.plugin'], []),
    'nfacct': ([PLUGIN + 'nfacct.plugin'], []),
    'ipmi': ([PLUGIN + 'freeipmi.plugin'], []),
    'xen': ([PLUGIN + 'xenstat.plugin'], []),
    'cups': ([PLUGIN + 'cups.plugin'], []),
    'ebpf': ([PLUGIN + 'ebpf.plugin', PLUGIN + 'ebpf-go.plugin'], []),
    'systemd-units': ([PLUGIN + 'systemd-units.plugin'], []),
    'ioping': ([PLUGIN + 'ioping', PLUGIN + 'ioping.plugin', CONF + 'ioping.conf'], []),
    'log2journal': (['bin/log2journal', CONF + 'log2journal.d'], []),
}
STOCK_ELF = {
    path for paths, _ in CAPS.values() for path in paths
    if path.startswith(PLUGIN) or path.startswith('bin/')
} | {'bin/bash', 'bin/curl', 'bin/nd-run', 'bin/netdatacli', 'bin/systemd-cat-native',
     'bin/srv/netdata', PLUGIN + 'ndsudo'}
REQUIRED_COMPANIONS = {
    'containers': [PLUGIN + 'cgroup-network', PLUGIN + 'cgroup-network-helper.sh'],
    'go': [PLUGIN + 'local-listeners', PLUGIN + 'snmp-trap-profile-gen', CONF + 'go.d.conf', CONF + 'go.d'],
    'python': ['usr/libexec/netdata/python.d'],
    'charts': ['usr/libexec/netdata/charts.d'],
}
ABS_LINKS = {
    'bin/srv/netdata-claim.sh': '/opt/netdata/bin/netdata-claim.sh',
    'etc/netdata/orig': '/opt/netdata/usr/lib/netdata/conf.d',
}
MANIFEST = 'usr/share/netdata/fleet-manifest.json'


def fail(message):
    raise ValueError(message)


def command(args):
    print(f'{shlex.quote(os.getcwd())} > {shlex.join(args)}', file=sys.stderr)
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        fail(f'command failed ({result.returncode}): {shlex.join(args)}\n'
             f'{result.stderr.decode(errors="replace")}')
    if result.stderr:
        print(result.stderr.decode(errors='replace'), file=sys.stderr, end='')
    return result.stdout


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def normalized(name):
    p = PurePosixPath(name)
    if p.is_absolute() or '..' in p.parts or '\\' in name or '\x00' in name:
        fail(f'unsafe path: {name!r}')
    return str(p)


def validate_members(members):
    records = {}
    for m in members:
        name = normalized(m.name)
        if name == '.' and m.isdir():
            continue
        if name in records or name == '.':
            fail(f'duplicate or invalid member: {name}')
        if not (m.isfile() or m.isdir() or m.issym()):
            fail(f'unsupported member type: {name}')
        if m.issym():
            if m.linkname.startswith('/'):
                if ABS_LINKS.get(name) != m.linkname:
                    fail(f'unsupported absolute symlink: {name}')
            else:
                parts = list(PurePosixPath(name).parent.parts)
                for part in PurePosixPath(m.linkname).parts:
                    if part == '..':
                        if not parts:
                            fail(f'symlink escapes source: {name}')
                        parts.pop()
                    elif part != '.':
                        parts.append(part)
                if not m.linkname or '\\' in m.linkname:
                    fail(f'invalid symlink: {name}')
        records[name] = m
    for name in records:
        for parent in PurePosixPath(name).parents:
            ancestor = records.get(str(parent))
            if ancestor and not ancestor.isdir():
                fail(f'member beneath non-directory: {name}')
    for parent in PurePosixPath(MANIFEST).parents:
        ancestor = records.get(str(parent))
        if ancestor and not ancestor.isdir():
            fail(f'non-directory ancestor of generated manifest: {parent}')
    for required in ('bin/netdata', 'bin/srv/netdata', 'bin/bash', 'bin/nd-run',
                     'bin/curl', 'bin/netdatacli', 'system/post-installer.sh',
                     'system/functions.sh', 'system/install-or-update.sh'):
        if required not in records or not records[required].isfile():
            fail(f'not a supported static Netdata tree: missing {required}')
    return records


def tree_members(root):
    result = []
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            info = path.lstat()
            member = tarfile.TarInfo(path.relative_to(root).as_posix())
            member.mode = stat.S_IMODE(info.st_mode)
            member.uid, member.gid, member.mtime = info.st_uid, info.st_gid, int(info.st_mtime)
            if stat.S_ISLNK(info.st_mode):
                member.type, member.linkname = tarfile.SYMTYPE, os.readlink(path)
            elif stat.S_ISDIR(info.st_mode):
                member.type = tarfile.DIRTYPE
            elif stat.S_ISREG(info.st_mode):
                member.size = info.st_size
            else:
                fail(f'unsupported source file: {path}')
            result.append(member)
    return result


def header_info(path, expected):
    if not re.fullmatch('[0-9a-fA-F]{64}', expected or '') or digest(path) != expected.lower():
        fail('installer SHA-256 does not match --sha256 (required for archive inputs)')
    with path.open('rb') as f:
        prefix = f.read(65536)
        matches = re.findall(rb'^skip="([0-9]+)"$', prefix, re.M)
        if len(matches) != 1 or not 1 <= int(matches[0]) <= 2000:
            fail('unsupported Makeself header line count')
        f.seek(0)
        header = b''.join(f.readline() for _ in range(int(matches[0])))
        offset = f.tell()
    text = header.decode('utf-8')
    values = {}
    for key in ('filesizes', 'totalsize', 'CRCsum', 'MD5', 'SHA', 'SIGNATURE',
                'decrypt_cmd', 'targetdir', 'script'):
        found = re.findall(r'^' + key + r'="([^"\n]*)"$', text, re.M)
        if len(found) != 1:
            fail(f'unsupported Makeself assignment: {key}')
        values[key] = found[0]
    if (values['SIGNATURE'] or values['decrypt_cmd'] or values['targetdir'] != '/opt/netdata'
            or values['script'] != './system/post-installer.sh'
            or 'eval "gzip -cd"' not in text):
        fail('only unsigned, unencrypted, single-gzip static Netdata installers are supported')
    if not values['filesizes'].isdigit() or values['filesizes'] != values['totalsize']:
        fail('unsupported multipart installer')
    if path.stat().st_size - offset != int(values['filesizes']):
        fail('installer payload length mismatch')
    return text, offset


def patch_permissions(text):
    # Keep the official permission policy, adding existence checks for omitted plugins.
    for plugin in ('apps.plugin', 'slabinfo.plugin', 'debugfs.plugin', 'go.d.plugin', 'perf.plugin'):
        path = PLUGIN + plugin
        pattern = re.compile(r'(?m)^  if ! run setcap ([^\n]+) "' + re.escape(path)
                             + r'"; then\n    run chmod 4750 "' + re.escape(path) + r'"\n  fi$')
        replacement = lambda m: ('  if [ -f "' + path + '" ]; then\n'
                                 + '\n'.join('  ' + line for line in m[0].splitlines()) + '\n  fi')
        text, count = pattern.subn(replacement, text)
        if count == 0 and f'  if [ -f "{path}" ]; then' not in text:
            fail(f'unrecognized installer permission policy for {plugin}')
    old = '    f="usr/libexec/netdata/plugins.d/${x}"\n    run chmod 4750 "${f}"'
    new = '    f="usr/libexec/netdata/plugins.d/${x}"\n    if [ -f "${f}" ]; then\n      run chmod 4750 "${f}"\n    fi'
    if old in text:
        text = text.replace(old, new)
    elif new not in text:
        fail('unrecognized installer fallback permission policy')
    return text


def elf_contract(path):
    data = path.read_bytes()
    if not data.startswith(b'\x7fELF'):
        return None
    if len(data) < 64 or data[4] not in (1, 2) or data[5] not in (1, 2):
        fail(f'unsupported ELF: {path}')
    endian = '<' if data[5] == 1 else '>'
    wide = data[4] == 2
    h = struct.unpack_from(endian + ('16sHHIQQQIHHHHHH' if wide else '16sHHIIIIIHHHHHH'), data)
    offset, size, count, names_index = h[6], h[11], h[12], h[13]
    fmt = endian + ('IIQQQQIIQQ' if wide else 'IIIIIIIIII')
    if not count or count > 10000 or size != struct.calcsize(fmt) or names_index >= count:
        fail(f'unsupported ELF section table: {path}')
    sections = [struct.unpack_from(fmt, data, offset + i * size) for i in range(count)]
    ns = sections[names_index]
    names = data[ns[4]:ns[4] + ns[5]]
    allocated = []
    for s in sections:
        if s[2] & 2:
            if s[1] != 8 and s[4] + s[5] > len(data):
                fail(f'truncated ELF: {path}')
            contents = data[s[4]:s[4] + s[5]] if s[1] != 8 else b''
            allocated.append((names[s[0]:].split(b'\0', 1)[0], s[1], s[2], s[3], s[5],
                              hashlib.sha256(contents).hexdigest()))
    return (data[4:6], h[1:5], allocated)


def publish_file(source, target):
    # A hard link provides atomic publication without overwriting an existing output.
    os.link(source, target)
    print(f'Published {target}', file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description='Prepare a new reduced static Netdata image. Never run on a live installation.')
    source = parser.add_mutually_exclusive_group()
    source.add_argument('--input', type=Path, help='official single-gzip .gz.run installer')
    source.add_argument('--source', type=Path, help='offline static package tree')
    parser.add_argument('--sha256', help='published full-installer SHA-256, required with --input')
    parser.add_argument('--output', type=Path, help='new installer file or new staging directory')
    parser.add_argument('--keep', help='required: all, none, or comma-separated capabilities')
    parser.add_argument('--strip-mode', choices=('debug', 'all', 'none'), default='debug')
    parser.add_argument('--objcopy', default='objcopy', help='objcopy supporting the target architecture')
    parser.add_argument('--apply', action='store_true', help='write the new output; default only previews removal policy')
    parser.add_argument('--list-capabilities', action='store_true')
    args = parser.parse_args()
    if args.list_capabilities:
        print('\n'.join(f'{name}: {", ".join(paths)}' + (f' (requires {", ".join(deps)})' if deps else '')
                        for name, (paths, deps) in CAPS.items()))
        return
    if not (args.input or args.source) or args.keep is None:
        parser.error('provide --input or --source, and explicit --keep')
    if args.apply and not args.output:
        parser.error('--apply requires --output')
    requested = args.keep.split(',')
    if args.keep not in ('all', 'none') and any(n not in CAPS for n in requested):
        fail('unknown capability; use --list-capabilities')
    selected = set(CAPS) if args.keep == 'all' else set() if args.keep == 'none' else set(requested)
    for name in list(selected):
        selected.update(CAPS[name][1])
    input_path = (args.input or args.source).resolve(strict=True)
    live = Path('/opt/netdata')
    if input_path == live or live in input_path.parents:
        fail('refusing the live /opt/netdata installation; use an offline image tree')
    output = None
    if args.output:
        parent = args.output.absolute().parent.resolve(strict=True)
        output = parent / args.output.name
        if output.exists() or output.is_symlink():
            fail('output already exists; choose a fresh output')
        if output == live or live in output.parents:
            fail('refusing output in /opt/netdata')
        if args.source and (output == input_path or input_path in output.parents or output in input_path.parents):
            fail('source and output must not overlap')
        if output == input_path:
            fail('input and output must differ')
    else:
        parent = Path.cwd().resolve(strict=True)
    header, offset, archive = None, None, None
    if args.input:
        header, offset = header_info(input_path, args.sha256)
        work = Path(tempfile.mkdtemp(prefix='.netdata-fleet-', dir=parent))
        print(f'Workspace retained for inspection: {work}', file=sys.stderr)
        raw_tar = work / 'input.tar'
        with input_path.open('rb') as f, raw_tar.open('xb') as out:
            f.seek(offset)
            with gzip.GzipFile(fileobj=f) as gz:
                shutil.copyfileobj(gz, out)
        archive = tarfile.open(raw_tar, 'r:')
        records = validate_members(archive.getmembers())
    else:
        if not input_path.is_dir():
            fail('--source must be an offline directory')
        records = validate_members(tree_members(input_path))
        work = None
    # Static archives can list only files, leaving directories implicit.
    def present(path):
        return path in records or any(p.startswith(path + '/') for p in records)

    if args.keep != 'all':
        for name in selected:
            primary = CAPS[name][0][0]
            if not present(primary):
                fail(f'requested capability {name} is absent ({primary})')
    available = {name for name in selected if present(CAPS[name][0][0])}
    for name in available:
        for companion in REQUIRED_COMPANIONS.get(name, []):
            if not present(companion):
                fail(f'incomplete capability {name}: missing {companion}')
    omitted = [p for name, (paths, _) in CAPS.items() if name not in selected for p in paths]
    # Agent startup validates the web directory even when no dashboard is installed.
    required_empty_dirs = {'usr/share/netdata/web'}
    removed = sorted(p for p in records
                     if not (p in required_empty_dirs and records[p].isdir())
                     and any(p == prefix or p.startswith(prefix + '/') for prefix in omitted))
    removed_set = set(removed)
    original_bytes = sum(m.size for m in records.values() if m.isfile())
    removed_bytes = sum(records[p].size for p in removed if records[p].isfile())
    report = dict(schema_version=1, requested=requested, resolved=sorted(available),
                  strip_mode=args.strip_mode, source_sha256=digest(input_path) if args.input else None,
                  original_regular_bytes=original_bytes, removed_regular_bytes=removed_bytes,
                  removed_paths=removed, stripped_files=[])
    print(json.dumps(report, indent=2))
    if not args.apply:
        print('Preview only. Stripping savings are measured with --apply; no output published.', file=sys.stderr)
        if archive:
            archive.close()
        return
    if work is None:
        work = Path(tempfile.mkdtemp(prefix='.netdata-fleet-', dir=parent))
        print(f'Workspace retained for inspection: {work}', file=sys.stderr)
    stage = work / 'tree'
    stage.mkdir()
    kept = {p: copy.copy(m) for p, m in records.items() if p not in removed_set and p != MANIFEST}
    for name in required_empty_dirs:
        if name not in kept:
            directory = tarfile.TarInfo(name)
            directory.type, directory.mode = tarfile.DIRTYPE, 0o755
            kept[name] = directory
    # Create regular files before links, and never traverse an input symlink.
    for name, m in sorted(kept.items()):
        target = stage / name
        if m.isdir():
            target.mkdir(parents=True, exist_ok=True)
        elif m.isfile():
            target.parent.mkdir(parents=True, exist_ok=True)
            src = archive.extractfile(m) if archive else (input_path / name).open('rb')
            with src, target.open('xb') as dest:
                shutil.copyfileobj(src, dest)
            if name == 'system/install-or-update.sh':
                target.write_text(patch_permissions(target.read_text()))
            if args.strip_mode != 'none' and name in STOCK_ELF:
                before = elf_contract(target)
                if before is not None:
                    stripped = work / 'stripped-file'
                    command([args.objcopy, '--strip-' + args.strip_mode, str(target), str(stripped)])
                    if elf_contract(stripped) != before:
                        fail(f'stripping changed allocated sections or ELF identity: {name}')
                    old_size = target.stat().st_size
                    os.replace(stripped, target)
                    report['stripped_files'].append(dict(path=name, before=old_size, after=target.stat().st_size))
            m.size = target.stat().st_size
            os.chmod(target, m.mode)
            os.utime(target, (m.mtime, m.mtime))
    for name, m in kept.items():
        if m.issym():
            target = stage / name
            target.parent.mkdir(parents=True, exist_ok=True)
            os.symlink(m.linkname, target)
    report['output_regular_bytes_without_manifest'] = sum(m.size for m in kept.values() if m.isfile())
    manifest = stage / MANIFEST
    manifest.parent.mkdir(parents=True, exist_ok=True)
    manifest.write_text(json.dumps(report, indent=2) + '\n')
    mm = tarfile.TarInfo(MANIFEST)
    mm.size, mm.mode = manifest.stat().st_size, 0o644
    kept[MANIFEST] = mm
    if archive:
        archive.close()
    if args.source:
        # Restore directory permissions last so read-only directories can be populated.
        for name, m in sorted(kept.items(), reverse=True):
            if m.isdir():
                os.chmod(stage / name, m.mode)
                os.utime(stage / name, (m.mtime, m.mtime))
        # mkdir reserves a fresh output; copytree never overwrites somebody else's directory.
        output.mkdir()
        shutil.copytree(stage, output, dirs_exist_ok=True, symlinks=True, copy_function=shutil.copy2)
        print('Tree output retains modes and links; assign deployment ownership in the image pipeline.', file=sys.stderr)
    else:
        tar_path = work / 'output.tar'
        with tarfile.open(tar_path, 'w', format=tarfile.PAX_FORMAT) as tf:
            for name, m in kept.items():
                m.name = './' + name
                if m.isfile():
                    with (stage / name).open('rb') as f:
                        tf.addfile(m, f)
                else:
                    tf.addfile(m)
        payload = work / 'payload.gz'
        with tar_path.open('rb') as src, payload.open('xb') as dest:
            with gzip.GzipFile(filename='', fileobj=dest, mode='wb', mtime=0) as gz:
                shutil.copyfileobj(src, gz)
        md5, sha = hashlib.md5(), hashlib.sha256()
        with payload.open('rb') as f:
            for block in iter(lambda: f.read(1024 * 1024), b''):
                md5.update(block)
                sha.update(block)
        crc = command(['cksum', str(payload)]).decode().split()[0]
        values = dict(filesizes=str(payload.stat().st_size), totalsize=str(payload.stat().st_size),
                      CRCsum=crc, MD5=md5.hexdigest(), SHA=sha.hexdigest())
        for key, value in values.items():
            header = re.sub(r'^' + key + r'="[^"\n]*"$', key + '="' + value + '"', header, flags=re.M)
        usize = (tar_path.stat().st_size + 1023) // 1024
        header, count = re.subn(r'(?:(?<=extract )|(?<=size: )|(?<="\$leftspace" -lt )|(?<=\())\d+(?= KB|; then)', str(usize), header)
        if count != 4:
            fail('unrecognized Makeself uncompressed-space declarations')
        prepared = work / 'installer.gz.run'
        with prepared.open('xb') as dest, payload.open('rb') as src:
            dest.write(header.encode())
            shutil.copyfileobj(src, dest)
        os.chmod(prepared, 0o755)
        checksum_path = Path(str(output) + '.sha256')
        if checksum_path.exists() or checksum_path.is_symlink():
            fail('checksum output already exists')
        checksum = work / 'installer.sha256'
        checksum.write_text(f'{digest(prepared)}  {output.name}\n')
        publish_file(checksum, checksum_path)
        publish_file(prepared, output)
    print(f'Prepared {output}: {report["output_regular_bytes_without_manifest"]} regular-file bytes before manifest', file=sys.stderr)


try:
    main()
except (OSError, ValueError, tarfile.TarError, EOFError, struct.error) as error:
    print(f'ERROR: {error}', file=sys.stderr)
    sys.exit(1)
PY
