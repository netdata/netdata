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
        result = subprocess.run(['bash', '-c', setup], cwd=self.work, env=env,
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

    def source_update(self, mode, channel="stable"):
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w:gz') as tar:
            data = (f'#!/bin/sh\necho installed > "{self.root}/installed"\n'
                    f'echo \'RELEASE_CHANNEL="nightly"\' > "{self.root}/prefix/etc/netdata/.environment"\n').encode()
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
RELEASE_CHANNEL={channel}
set_tarball_urls {channel} no
update_build
''')
        if mode == 'cached':
            self.assertFalse((self.root / 'installed').exists())
        else:
            self.assertEqual((self.root / 'installed').read_text().strip(), 'installed')
            if channel == 'preview':
                self.assertEqual(envfile.read_text(), 'RELEASE_CHANNEL="preview"\n')
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
        self.run_shell('NETDATA_OFFLINE_INSTALL_SOURCE=/nonexistent-updater-fixture\n'
                       'set_tarball_urls stable no', success=False)

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

    def test_preview_hosting_and_no_github_fallback(self):
        for static in ('yes', 'no'):
            self.run_shell(f'''
NETDATA_ARTIFACT_URL_PREFIX=
NETDATA_BASE_URL=https://legacy.example
check_for_remote_file() {{ [ "$1" = https://artifacts.netdata.cloud/preview/latest/Manifest ]; }}
set_tarball_urls preview {static}
[ "$NETDATA_MANIFEST_INSTALL" = 1 ]
[ "$NETDATA_TARBALL_URL_PREFIX" = https://artifacts.netdata.cloud/preview/latest ]
''')
            result = self.run_shell(f'''
NETDATA_ARTIFACT_URL_PREFIX=
get_netdata_latest_tag() {{ echo github-must-not-be-used >&2; exit 9; }}
check_for_remote_file() {{ return 1; }}
set_tarball_urls preview {static}
''', success=False)
            self.assertNotIn('github-must-not-be-used', result.stderr)
        # Native version discovery also uses only hosted preview metadata.
        self.run_shell('''
NETDATA_ARTIFACT_URL_PREFIX=
NETDATA_BASE_URL=https://legacy.example
get_netdata_latest_tag() { echo github-must-not-be-used >&2; exit 9; }
download() {
  [ "$1" = https://artifacts.netdata.cloud/preview/latest/Version ]
  echo v2.9.0-rc.2 > "$2"
}
[ "$(get_latest_tag preview)" = v2.9.0-rc.2 ]
''')
        result = self.run_shell('''
download() { return 1; }
get_netdata_latest_tag() { echo github-must-not-be-used >&2; exit 9; }
get_latest_tag preview
''', success=False)
        self.assertNotIn('github-must-not-be-used', result.stderr)

    def test_preview_explicit_source_requires_manifest(self):
        result = self.run_shell("""
check_for_remote_file() { return 1; }
set_tarball_urls preview yes
""", success=False)
        self.assertIn('preview release manifest', result.stderr)

    def test_preview_environment_replacement(self):
        envfile = self.root / '.environment'
        original = 'RELEASE_CHANNEL="nightly"\nOTHER="preserved"\n'
        for failure in ('', 'cp', 'sed', 'mv'):
            with self.subTest(failure=failure):
                envfile.write_text(original)
                envfile.chmod(0o640)
                before = envfile.stat()
                stub = ''
                if failure == 'sed':
                    stub = 'sed() { printf partial; return 1; }'
                elif failure:
                    stub = f'{failure}() {{ return 1; }}'
                self.run_shell(f'''
RELEASE_CHANNEL=preview
ENVIRONMENT_FILE='{envfile}'
{stub}
restore_preview_channel
''', success=not failure)
                expected = original if failure else original.replace('nightly', 'preview')
                self.assertEqual(envfile.read_text(), expected)
                after = envfile.stat()
                self.assertEqual((after.st_mode, after.st_uid, after.st_gid),
                                 (before.st_mode, before.st_uid, before.st_gid))
                if failure:
                    self.assertEqual(after.st_ino, before.st_ino)
                self.assertEqual(list(self.root.glob('.environment.*')), [])

    def test_preview_environment_symlinks(self):
        target = self.root / 'config' / 'environment'
        target.parent.mkdir()
        link = self.root / '.environment'
        intermediate = self.root / 'config' / 'link'
        intermediate.symlink_to('environment')
        original = 'RELEASE_CHANNEL="nightly"\nOTHER="preserved"\n'
        for destination in (str(target), 'config/environment', 'config/link'):
            for failure in ('', 'cp', 'sed', 'mv', 'readlink'):
                with self.subTest(destination=destination, failure=failure):
                    target.write_text(original)
                    target.chmod(0o640)
                    before = target.stat()
                    link.unlink(missing_ok=True)
                    link.symlink_to(destination)
                    stub = f'{failure}() {{ return 1; }}' if failure else ''
                    self.run_shell(f'''
RELEASE_CHANNEL=preview
ENVIRONMENT_FILE='{link}'
{stub}
restore_preview_channel
''', success=not failure)
                    self.assertTrue(link.is_symlink())
                    self.assertEqual(os.readlink(link), destination)
                    self.assertTrue(intermediate.is_symlink())
                    self.assertEqual(target.read_text(), original if failure else
                                     original.replace('nightly', 'preview'))
                    after = target.stat()
                    self.assertEqual((after.st_mode, after.st_uid, after.st_gid),
                                     (before.st_mode, before.st_uid, before.st_gid))
                    if failure:
                        self.assertEqual(after.st_ino, before.st_ino)
                    self.assertEqual(list(target.parent.glob('environment.*')), [])
                    link.unlink()

    def test_preview_environment_invalid_symlinks(self):
        link = self.root / '.environment'
        for destination in ('missing', '.environment'):
            with self.subTest(destination=destination):
                link.symlink_to(destination)
                self.run_shell(f'''
RELEASE_CHANNEL=preview
ENVIRONMENT_FILE='{link}'
restore_preview_channel
''', success=False)
                self.assertTrue(link.is_symlink())
                self.assertEqual(os.readlink(link), destination)
                self.assertEqual(list(self.root.glob('*.??????')), [])
                self.assertEqual(list(self.root.glob('.environment.*')), [])
                link.unlink()

    def test_current_version_without_binary(self):
        result = self.run_shell('''
command() { return 1; }
get_current_version
''')
        self.assertEqual(result.stdout.strip(), '0')

    def test_preview_self_update_executes_hosted_script(self):
        result = self.run_shell('''
RELEASE_CHANNEL=preview
NETDATA_FORCE_UPDATE=1
INTERACTIVE=0
_safe_download() {
  [ "$1" = https://artifacts.netdata.cloud/installer/netdata-updater.sh ] || exit 8
  printf '#!/bin/sh\\nprintf "hosted-updater %%s\\n" "$*"\\n' > "$2"
}
self_update
''')
        self.assertIn(
            'hosted-updater --not-running-from-cron --no-updater-self-update --force-update --non-interactive',
            result.stdout)

    def test_preview_follows_publication_identity(self):
        # Preview follows the channel's publication, without imposing an ordering
        # on prerelease identifiers or keeping a version absent from that channel.
        for current, latest, needs_update in [
            ('v2.9.0-alpha.1', 'v2.9.0-beta.1', True),
            ('v2.9.0-rc.1', 'v2.9.0-rc.2', True),
            ('v2.9.0-rc.999', 'v2.9.0-rc.1000', True),
            ('v2.999.0-alpha.1', 'v2.1000.0-alpha.1', True),
            ('v2.9.0-rc1', 'v2.9.0', True),
            ('v2.9.0', 'v2.10.0-alpha.1+build.8', True),
            ('v2.10.0', 'v2.9.0-rc1', True),
            ('v2.9.0-rc.1+build.8', 'v2.9.0-rc.1+build.8', False),
            ('2.9.0', 'v2.9.0', False),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1', False),
            ('v2.9.0-rc1', 'v2.9.0-rc1-3-gabc1234', False),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1-8-gdef5678', False),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc2', True),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1-3-gxyz', True),
            ('v2.9.0-rc1+build.8', 'v2.9.0-rc1+build.9', True),
            ('v2.9.0-rc1-3-gabc1234+build.8', 'v2.9.0-rc1', True),
        ]:
            with self.subTest(current=current, latest=latest):
                self.run_shell(f'''
RELEASE_CHANNEL=preview
NETDATA_FORCE_UPDATE=0
NETDATA_ACCEPT_MAJOR_VERSIONS=2
get_current_version() {{ echo legacy-encoding-must-not-be-used >&2; exit 8; }}
warn_major_update() {{ exit 7; }}
get_current_tag() {{ echo {current}; }}
get_latest_tag() {{ echo {latest}; }}
if update_available; then result=yes; else result=no; fi
[ "$result" = {'yes' if needs_update else 'no'} ]
''')
        self.run_shell('''
RELEASE_CHANNEL=preview
NETDATA_FORCE_UPDATE=0
NETDATA_ACCEPT_MAJOR_VERSIONS=2
get_current_tag() { echo v2.9.0; }
get_latest_tag() { echo v3.0.0-rc1; }
warn_major_update() { exit 7; }
update_available
''', success=False)

    def test_prerelease_numeric_core_and_metadata(self):
        self.run_shell('''
[ "$(parse_version v2.9.0-rc.10+build.8)" = "$(parse_version v2.9.0-rc10)" ]
[ "$(parse_version v2.9.0+build.8)" = "$(parse_version v2.9.0)" ]
''')

    def test_preview_native_repository_and_update(self):
        for distro, version, codename, repo_path in [
            ('debian', '12', 'bookworm', 'debian/bookworm'),
            ('fedora', '43', '', 'fedora/43/fixture-arch'),
            ('opensuse-leap', '15.6', '', 'opensuse/15.6/fixture-arch'),
        ]:
            with self.subTest(distro=distro):
                release_file = self.root / 'os-release'
                release_file.write_text(f'ID={distro}\nVERSION_ID={version}\nVERSION_CODENAME={codename}\n')
                functions = FUNCTIONS.replace('/etc/os-release', str(release_file))
                result = self.run_shell('''
RELEASE_CHANNEL=stable
INSTALL_TYPE=binpkg-deb
installed_tag=v2.9.0-alpha.1
NETDATA_PREVIEW_REPO_URL=https://repository.netdata.cloud/repos/preview
NETDATA_NO_SYSTEMD_JOURNAL=1
INTERACTIVE=0
uname() { echo fixture-arch; }
env() {
  while [ "$#" -gt 0 ]; do
    case "$1" in *=*) shift ;; *) break ;; esac
  done
  "$@"
}
fixture_pm() {
  echo "$*" >> pm-calls
  for arg do [ "$arg" != netdata ] || installed_tag=v2.9.0-rc.1000; done
  return 0
}
apt-get() { fixture_pm "$@"; }
dnf() { fixture_pm "$@"; }
yum() { fixture_pm "$@"; }
zypper() { fixture_pm "$@"; }
dpkg-query() { [ "$2" = netdata-repo-preview ]; }
rpm() { [ "$2" = netdata-repo-preview ]; }
get_current_tag() { echo "$installed_tag"; }
get_new_binpkg_major() { echo 2; }
get_current_version() { echo legacy-encoding-must-not-be-used >&2; exit 8; }
warn_major_update() { exit 7; }
check_for_remote_file() { echo "$1"; return 0; }
get_latest_tag() { [ "$1" = preview ]; echo v2.9.0-rc.1000; }
update_binpkg
[ "$RELEASE_CHANNEL" = preview ]
[ "$repopkg" = netdata-repo-preview ]
[ "$installed_tag" = v2.9.0-rc.1000 ]
''', functions=functions)
                self.assertIn(f'https://repository.netdata.cloud/repos/preview/{repo_path}/.currently.published',
                              result.stdout)
                self.assertIn('netdata-repo-preview', (self.work / 'pm-calls').read_text())
                (self.work / 'pm-calls').unlink()

    def test_preview_native_update_progress_and_publication_skew(self):
        release_file = self.root / 'os-release'
        release_file.write_text('ID=debian\nVERSION_ID=12\nVERSION_CODENAME=bookworm\n')
        functions = FUNCTIONS.replace('/etc/os-release', str(release_file))
        for initial, post_transaction, latest, succeeds in [
            ('v2.9.0-rc1', 'v2.9.0-rc1', 'v2.9.0-rc3', False),
            ('v2.9.0-rc1', 'v2.9.0-rc2', 'v2.9.0-rc3', True),
            ('v2.9.0-rc1', 'v2.9.0-rc2', 'v2.9.0-rc1', True),
            ('v2.9.0-rc1', 'v2.9.0-rc1', 'v2.9.0-rc1', True),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1', True),
            ('v2.9.0-rc1', 'v2.9.0-rc1', 'v2.9.0-rc1-3-gabc1234', True),
            ('v2.9.0-rc1', '2.9.0-rc1', 'v2.9.0-rc3', False),
            ('v2.9.0-rc1-3-gabc1234', 'v2.9.0-rc1-4-gdef5678', 'v2.9.0-rc3', True),
            ('v2.9.0-rc3', 'v2.9.0-rc2', 'v2.9.0-rc1', True),
        ]:
            with self.subTest(initial=initial, installed=post_transaction, latest=latest):
                result = self.run_shell(f'''
INTERACTIVE=1
NETDATA_NO_SYSTEMD_JOURNAL=1
NETDATA_PREVIEW_REPO_URL=https://repository.netdata.cloud/repos/preview
installed_tag={initial}
env() {{ "$@"; }}
apt-get() {{
  for arg do [ "$arg" != netdata ] || installed_tag={post_transaction}; done
  return 0
}}
dpkg-query() {{ [ "$2" = netdata-repo-preview ]; }}
get_current_tag() {{ echo "$installed_tag"; }}
get_latest_tag() {{ echo {latest}; }}
get_new_binpkg_major() {{ echo 2; }}
check_for_remote_file() {{ return 0; }}
update_binpkg
''', functions=functions, success=succeeds)
                if not succeeds:
                    self.assertIn('Package manager did not fully update Netdata', result.stderr)

    def test_preview_missing_version_fails_in_update_check(self):
        result = self.run_shell('''
RELEASE_CHANNEL=preview
NETDATA_FORCE_UPDATE=0
get_current_tag() { echo v2.9.0; }
download() { return 1; }
if update_available; then echo must-not-install; fi
''', success=False)
        self.assertNotIn('must-not-install', result.stdout)

    def test_preview_offline_source_takes_precedence(self):
        self.artifact('netdata-aarch64-v2.9.0-rc1.gz.run')
        self.run_shell(f'''
NETDATA_OFFLINE_INSTALL_SOURCE='{self.release}'
NETDATA_ARTIFACT_URL_PREFIX=https://invalid.example
set_tarball_urls preview yes
[ "$NETDATA_TARBALL_URL_PREFIX" = 'file://{self.release}' ]
[ "$(get_latest_tag preview)" = v2.8.0 ]
''')

    def test_preview_self_update_never_uses_github(self):
        for install_type in ('kickstart-build', 'binpkg-deb', 'binpkg-rpm'):
            result = self.run_shell(f'''
INSTALL_TYPE={install_type}
RELEASE_CHANNEL={'preview' if install_type == 'kickstart-build' else 'stable'}
dpkg-query() {{ [ "$2" = netdata-repo-preview ]; }}
rpm() {{ [ "$2" = netdata-repo-preview ]; }}
newer_commit_date() {{ echo github-freshness-must-not-be-used >&2; exit 8; }}
_safe_download() {{ echo "$1"; return 1; }}
self_update
''')
            self.assertEqual(result.stdout.strip(), 'https://artifacts.netdata.cloud/installer/netdata-updater.sh')
            self.assertNotIn('github-freshness', result.stderr)
        # The existing fallback remains available to stable installations.
        result = self.run_shell('''
RELEASE_CHANNEL=stable
newer_commit_date() { return 0; }
_safe_download() { echo "$1"; return 1; }
self_update
''')
        self.assertIn('raw.githubusercontent.com', result.stdout)

    def test_preview_source_update_preserves_channel(self):
        self.source_update('preview', channel='preview')

    def test_preview_static_update_preserves_channel(self):
        prefix = self.root / 'opt/netdata'
        (prefix / 'etc/netdata').mkdir(parents=True)
        envfile = prefix / 'etc/netdata/.environment'
        envfile.write_text('RELEASE_CHANNEL="preview"\n')
        functions = FUNCTIONS.replace('/opt/netdata', str(prefix))
        payload = f'#!/bin/sh\necho \'RELEASE_CHANNEL="nightly"\' > "{envfile}"\n'.encode()
        self.artifact('netdata-aarch64-v2.9.0-rc.2.gz.run', payload)
        self.run_shell(f'''
RELEASE_CHANNEL=preview
ENVIRONMENT_FILE='{envfile}'
PREBUILT_ARCH=aarch64
NETDATA_FORCE_UPDATE=1
set_tarball_urls preview yes
update_static
''', functions=functions)
        self.assertEqual(envfile.read_text(), 'RELEASE_CHANNEL="preview"\n')


if __name__ == '__main__':
    unittest.main()
