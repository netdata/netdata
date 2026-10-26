#!/usr/bin/env python3
"""Offline tests of the standalone updater's artifact resolution and install flow."""
import hashlib
import io
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'netdata-updater.sh'
# Load function definitions without running the updater's host discovery/main.
FUNCTIONS = '\n'.join(re.findall(r'^[a-zA-Z_][a-zA-Z_0-9]*\s*\(\)\s*\{\n.*?^}',
                                 SCRIPT.read_text(), re.M | re.S))


class UpdaterArtifacts(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='updater-artifacts-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.release = self.root / 'release'
        self.work = self.root / 'work'
        self.release.mkdir()
        self.work.mkdir()
        (self.release / 'Version').write_text('v2.8.0\n')

    def artifact(self, name, content=b'archive', append=False):
        (self.release / name).write_bytes(content)
        with (self.release / 'Manifest').open('a' if append else 'w') as manifest:
            manifest.write(f'{name} {len(content)} {hashlib.sha256(content).hexdigest()}\n')
        return hashlib.sha256(content).hexdigest()

    def run_shell(self, body, success=True, extra='', functions=FUNCTIONS):
        setup = f'''
set -e
exec 3>/dev/null
{functions}
fatal() {{ echo "$1" >&2; exit 1; }}
create_exec_tmp_directory() {{ :; }}
NETDATA_ARTIFACT_URL_PREFIX='file://{self.release}'
RELEASE_ARTIFACT_URL_PREFIX=https://artifacts.netdata.cloud
NETDATA_STABLE_BASE_URL=https://github.com/netdata/netdata/releases
NETDATA_NIGHTLY_BASE_URL=https://github.com/netdata/netdata-nightlies/releases
ndtmpdir='{self.work}'
RELEASE_CHANNEL=stable
{extra}
{body}
'''
        env = {k: v for k, v in os.environ.items() if not k.startswith('NETDATA_')}
        result = subprocess.run(['/bin/sh', '-c', setup], cwd=self.work, env=env,
                                text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def test_manifest_selection_and_version(self):
        for arch in ('x86_64', 'aarch64', 'armv6l', 'armv7l'):
            self.artifact(f'netdata-{arch}-latest.gz.run')
            self.artifact(f'netdata-{arch}-v2.8.0.gz.run', append=True)
            result = self.run_shell(f'''
PREBUILT_ARCH={arch}
set_tarball_urls stable yes
fetch_release_metadata
name="$(manifest_archive_name '^netdata-{arch}-.*\\.run ')"
[ "$name" = netdata-{arch}-v2.8.0.gz.run ]
download "$NETDATA_TARBALL_URL_PREFIX/$name" "$name"
verify_manifest "$name"
get_latest_tag stable
''')
            self.assertEqual(result.stdout.strip().splitlines()[-1], 'v2.8.0')

    def test_size_hash_and_missing_entry_fail_closed(self):
        name = 'netdata-v2.8.0.tar.gz'
        for mutation in ('size', 'hash', 'missing'):
            self.artifact(name)
            (self.work / name).write_bytes(b'wrong!!' if mutation == 'hash' else b'wrong')
            if mutation == 'missing':
                (self.release / 'Manifest').write_text('other 5 abc\n')
            self.run_shell(f'set_tarball_urls stable no\nfetch_release_metadata\nverify_manifest {name}', success=False)

    def test_legacy_checksum_exact_matching(self):
        name = 'netdata-latest.tar.gz'
        digest = self.artifact(name)
        (self.release / 'Manifest').unlink()
        (self.release / 'sha256sums.txt').write_text(f'{digest} *./{name}\n')
        self.run_shell(f'''
set_tarball_urls stable no
[ "$NETDATA_MANIFEST_INSTALL" = 0 ]
fetch_release_metadata
download "$NETDATA_TARBALL_URL_PREFIX/{name}" {name}
verify_manifest {name}
''')
        (self.release / 'sha256sums.txt').write_text(f'{digest} *netdata-latestXtarXgz\n')
        self.run_shell(f'set_tarball_urls stable no\nfetch_release_metadata\nverify_manifest {name}', success=False)

    def test_offline_source_overrides_remote_prefix(self):
        self.artifact('netdata-x86_64-v2.8.0.gz.run')
        self.run_shell(f'''
NETDATA_OFFLINE_INSTALL_SOURCE='{self.release}'
NETDATA_ARTIFACT_URL_PREFIX=https://invalid.example
set_tarball_urls nightly yes
[ "$NETDATA_TARBALL_URL_PREFIX" = 'file://{self.release}' ]
[ "$NETDATA_MANIFEST_INSTALL" = 1 ]
''')

    def test_default_channels_and_github_fallback(self):
        for channel, repo in [('stable', 'netdata'), ('nightly', 'netdata-nightlies')]:
            self.run_shell(f'''
NETDATA_ARTIFACT_URL_PREFIX=
check_for_remote_file() {{ [ "$1" = 'https://artifacts.netdata.cloud/{channel}/latest/Manifest' ]; }}
set_tarball_urls {channel} no
[ "$NETDATA_MANIFEST_INSTALL" = 1 ]
check_for_remote_file() {{ return 1; }}
get_netdata_latest_tag() {{ echo v2.8.0; }}
set_tarball_urls {channel} no
[ "$NETDATA_MANIFEST_INSTALL" = 0 ]
[ "$NETDATA_TARBALL_URL_PREFIX" = 'https://github.com/netdata/{repo}/releases/download/v2.8.0' ]
''')

    def test_legacy_base_override_and_manifest_at_fallback(self):
        self.run_shell('''
NETDATA_BASE_URL=https://mirror.example
NETDATA_STABLE_BASE_URL=$NETDATA_BASE_URL
NETDATA_ARTIFACT_URL_PREFIX=
get_netdata_latest_tag() { echo v2.8.0; }
check_for_remote_file() { [ "$1" = https://mirror.example/download/v2.8.0/Manifest ]; }
set_tarball_urls stable no
[ "$NETDATA_MANIFEST_INSTALL" = 1 ]
[ "$NETDATA_TARBALL_URL_PREFIX" = https://mirror.example/download/v2.8.0 ]
''')

    def test_source_update_executes_verified_archive(self):
        self.source_update('fallback')

    @unittest.skipUnless(shutil.which('zstd'), 'optional zstd compressor unavailable')
    def test_source_zstd_update(self):
        self.source_update('zstd')

    def test_source_legacy_update(self):
        self.source_update('legacy')

    def test_source_matching_checksum_skips_install(self):
        self.source_update('cached')

    def source_update(self, mode):
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w:gz') as tar:
            data = f'#!/bin/sh\necho installed > "{self.root}/installed"\n'.encode()
            info = tarfile.TarInfo('netdata-v2.8.0/netdata-installer.sh')
            info.size, info.mode = len(data), 0o755
            tar.addfile(info, io.BytesIO(data))
        name = 'netdata-latest.tar.gz' if mode == 'legacy' else 'netdata-v2.8.0.tar.gz'
        digest = self.artifact(name, archive.getvalue())
        if mode == 'legacy':
            (self.release / 'Manifest').unlink()
            (self.release / 'sha256sums.txt').write_text(f'{digest} *{name}\n')
        if mode == 'zstd':
            import gzip
            compressed = subprocess.run(['zstd', '-q', '-c'], input=gzip.decompress(archive.getvalue()),
                                        capture_output=True, check=True).stdout
            digest = self.artifact('netdata-v2.8.0.tar.zst', compressed, append=True)
        # A declared but unavailable zstd archive must fall back to gzip.
        if mode == 'fallback':
            with (self.release / 'Manifest').open('a') as manifest:
                manifest.write('netdata-v2.8.0.tar.zst 10 abc\n')
        prefix = self.root / 'prefix'
        (prefix / 'etc/netdata').mkdir(parents=True)
        (prefix / 'var/lib/netdata').mkdir(parents=True)
        envfile = prefix / 'etc/netdata/.environment'
        envfile.touch()
        self.run_shell(f'''
install_build_dependencies() {{ :; }}
pidof() {{ return 0; }}
NETDATA_FORCE_UPDATE={'0' if mode == 'cached' else '1'}
NETDATA_TARBALL_CHECKSUM={'"' + digest + '"' if mode == 'cached' else ''}
get_netdata_latest_tag() {{ echo v2.8.0; }}
get_current_version() {{ echo 0; }}
NETDATA_PREFIX='{prefix}'
NETDATA_LIB_DIR='{prefix}/var/lib/netdata'
ENVIRONMENT_FILE='{envfile}'
set_tarball_urls stable no
update_build
''')
        if mode == 'cached':
            self.assertFalse((self.root / 'installed').exists())
        else:
            self.assertEqual((self.root / 'installed').read_text().strip(), 'installed')
            self.assertEqual((prefix / 'var/lib/netdata/netdata.tarball.checksum').read_text().strip(), digest)

    def test_static_update_and_corruption(self):
        # Redirect only the static install root into the fixture. All resolution,
        # verification, and installer invocation use the actual shell functions.
        prefix = self.root / 'opt/netdata'
        (prefix / 'etc/netdata').mkdir(parents=True)
        functions = FUNCTIONS.replace('/opt/netdata', str(prefix))
        name = 'netdata-aarch64-v2.8.0.gz.run'
        payload = f'#!/bin/sh\necho "$*" > "{self.root}/static-installed"\n'.encode()
        self.artifact(name, payload)
        body = """
PREBUILT_ARCH=aarch64
NETDATA_FORCE_UPDATE=1
set_tarball_urls stable yes
update_static
"""
        self.run_shell(body, functions=functions)
        self.assertEqual((self.root / 'static-installed').read_text().strip(), '--accept --')
        self.assertIn('legacy-static', (prefix / 'etc/netdata/.install-type').read_text())
        self.work.mkdir()
        (self.root / 'static-installed').unlink()
        (self.release / name).write_bytes(b'corrupt')
        self.run_shell(body, success=False, functions=functions)
        self.assertFalse((self.root / 'static-installed').exists())

    def test_unknown_channel_and_invalid_offline_source(self):
        self.run_shell('set_tarball_urls invalid no', success=False)
        self.run_shell('NETDATA_OFFLINE_INSTALL_SOURCE=/nonexistent-updater-fixture\nset_tarball_urls stable no', success=False)

    def test_missing_architecture_fails_before_execution(self):
        self.artifact('netdata-x86_64-v2.8.0.gz.run')
        self.run_shell('''
PREBUILT_ARCH=armv7l
NETDATA_FORCE_UPDATE=1
set_tarball_urls stable yes
update_static
''', success=False)

    def test_native_metadata_directory_owned_by_caller(self):
        release_file = self.root / 'os-release'
        release_file.write_text('ID=debian\nVERSION_ID=12\nVERSION_CODENAME=bookworm\n')
        functions = FUNCTIONS.replace('/etc/os-release', str(release_file))
        self.run_shell(f"""
ndtmpdir=
create_exec_tmp_directory() {{
  if [ -z "$ndtmpdir" ]; then ndtmpdir='{self.work}'; fi
}}
INTERACTIVE=1
env() {{ "$@"; }}
apt-get() {{ :; }}
dpkg-query() {{ :; }}
get_current_version() {{ echo 002008000300000000; }}
get_new_binpkg_major() {{ echo 2; }}
check_for_remote_file() {{ return 0; }}
NETDATA_NO_SYSTEMD_JOURNAL=1
update_binpkg
[ -n "$ndtmpdir" ]
""", functions=functions)

    def test_package_version_lookup_uses_hosting(self):
        self.run_shell('''
[ "$(get_latest_tag stable)" = v2.8.0 ]
''')

    def test_manifest_version_failure_does_not_use_github(self):
        self.artifact('netdata-v2.8.0.tar.gz')
        (self.release / 'Version').unlink()
        self.run_shell('set_tarball_urls stable no\nget_latest_tag stable', success=False)
        self.run_shell('''
get_current_version() { echo 002007000300000000; }
set_tarball_urls stable no
if update_available; then echo must-not-install; fi
''', success=False)


if __name__ == '__main__':
    unittest.main()
