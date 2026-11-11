# SPDX-License-Identifier: GPL-3.0-or-later
"""Offline kickstart channel tests; network and privileged operations are intercepted."""
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'kickstart.sh'
SOURCE = SCRIPT.read_text()
FUNCTION_NAMES = (
    'release_channel_for_version', 'main', 'parse_args', 'try_package_install',
    'configure_native_package_manager', 'common_rpm_opts', 'common_dnf_opts',
    'cleanup_apt_state', 'cleanup_dnf_state', 'cleanup_zypper_state', 'netdata_avail_check',
    'set_static_archive_urls', 'set_source_archive_urls',
    'save_release_channel', 'install_on_linux',
)
FUNCTIONS = '\n'.join(re.search(r'^' + name + r'\(\) \{\n.*?^}', SOURCE, re.M | re.S).group()
                      for name in FUNCTION_NAMES)
CONSTANTS = '\n'.join(re.findall(
    r'^(?:REPOCONFIG_(?:DEB|RPM)_VERSION|(?:STABLE|NIGHTLY|PREVIEW)_REPO_URL_PREFIX|'
    r'REPOCONFIG_URL_PREFIX|RELEASE_ARTIFACT_URL_PREFIX)=.*$', SOURCE, re.M))


class KickstartChannels(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='kickstart-channels-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def shell(self, body, success=True):
        setup = f'''
{CONSTANTS}
{FUNCTIONS}
fatal() {{ echo "$*" >&2; exit 1; }}
warning() {{ echo "$*" >&2; }}
progress() {{ :; }}
set_tmpdir() {{ :; }}
run_as_root() {{ printf '%s\\n' "$*" >> calls; }}
INTERACTIVE=0
DRY_RUN=0
SYSARCH=x86_64
DEFAULT_RELEASE_CHANNEL=nightly
SELECTED_RELEASE_CHANNEL=preview
NETDATA_TARBALL_BASEURL=https://github.com/netdata/netdata-nightlies/releases
tmpdir="$PWD"
{body}
'''
        env = {k: v for k, v in os.environ.items() if not k.startswith('NETDATA_')}
        result = subprocess.run(['/bin/sh', '-c', setup], cwd=self.root, env=env,
                                text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def test_channel_arguments(self):
        for channel in ('preview', 'stable', 'nightly', 'default'):
            for arg in (f'--release-channel={channel}', f'--release-channel {channel}'):
                with self.subTest(arg=arg):
                    self.shell(f'''
validate_args() {{ :; }}
parse_args {arg} --dry-run
[ "$RELEASE_CHANNEL" = {channel} ] && [ "$DRY_RUN" = 1 ] || exit 9
''')
        self.shell('validate_args() { :; }; parse_args --release-channel=unknown', success=False)
        self.shell('validate_args() { :; }; parse_args --preview-channel', success=False)

    def test_version_channel_inference(self):
        cases = [('2.13.0', 'stable'), ('2.13.0+build.1', 'stable'),
                 ('2.13.0-rc1', 'preview'), ('2.13.0-rc.2+build.1', 'preview'),
                 ('2.13.0-beta.1', 'preview'), ('2.13.0~rc1', 'preview'),
                 ('2.13.0-123-nightly', 'nightly'), ('2.13.0-123-gabcdef', 'nightly'),
                 ('2.13.0.123.gabcdef', 'nightly')]
        for version, expected in cases:
            for requested in ('stable', 'nightly', 'preview'):
                with self.subTest(version=version, requested=requested):
                    wanted = 'preview' if requested == 'preview' else expected
                    result = self.shell(f'release_channel_for_version {version} {requested}')
                    self.assertEqual(result.stdout.strip(), wanted)

    def test_main_uses_version_channel_for_offline_preparation(self):
        self.shell('''
INSTALL_VERSION=2.13.0-rc1
SELECTED_RELEASE_CHANNEL=nightly
ACTION=prepare-offline
prepare_offline_install_source() { [ "$SELECTED_RELEASE_CHANNEL" = preview ] || exit 5; }
deferred_warnings() { :; }
main
''')

    def test_native_repository_and_version_selection(self):
        for distro, version_id, codename, suffix in [
            ('debian', '12', 'bookworm', '_all.deb'),
            ('fedora', '43', '', '.noarch.rpm'),
            ('opensuse', '15.6', '', '.noarch.rpm'),
        ]:
            for channel, repo_suffix, repo_path in [
                ('preview', '-preview', 'preview'),
                ('stable', '', 'stable'), ('nightly', '-edge', 'edge'),
            ]:
                with self.subTest(distro=distro, channel=channel):
                    result = self.shell(f'''
DISTRO_COMPAT_NAME={distro}
SYSVERSION={version_id}
SYSCODENAME={codename}
SELECTED_RELEASE_CHANNEL={channel}
INSTALL_VERSION=2.13.0-rc1
check_for_remote_file() {{ echo "$1" >> urls; return 0; }}
download() {{ echo "$1" >> urls; return 0; }}
cleanup_apt_state() {{ :; }}
cleanup_dnf_state() {{ :; }}
cleanup_zypper_state() {{ :; }}
check_special_native_deps() {{ return 0; }}
netdata_avail_check() {{ return 0; }}
try_package_install >/dev/null
printf '%s\\n' "$repoconfig_name" "$REPO_URL_PREFIX" "$NATIVE_VERSION" "$repoconfig_url"
''')
                    lines = result.stdout.splitlines()
                    self.assertEqual(lines[0], f'netdata-repo{repo_suffix}')
                    self.assertEqual(lines[1], f'https://repository.netdata.cloud/repos/{repo_path}')
                    native_version = '2.13.0~rc1' if channel == 'preview' else '2.13.0-rc1'
                    expected_version = f'={native_version}' if distro == 'debian' else f'-{native_version}.x86_64'
                    self.assertEqual(lines[2], expected_version)
                    self.assertIn(f'netdata-repo{repo_suffix}', lines[3])
                    self.assertTrue(lines[3].endswith(suffix))
                    self.assertIn('5-6', lines[3])
                    calls = (self.root / 'calls').read_text()
                    self.assertIn(f'netdata{lines[2]}', calls)
                    urls = (self.root / 'urls').read_text()
                    self.assertIn(f'/repos/{repo_path}/', urls)
                    self.assertIn('.currently.published', urls)
                    (self.root / 'calls').unlink()
                    (self.root / 'urls').unlink()

    def test_repository_cleanup(self):
        for function in ('cleanup_apt_state', 'cleanup_dnf_state', 'cleanup_zypper_state'):
            self.shell(f'''
pkg_installed() {{ return 0; }}
pm_cmd=fixture-pm
uninstall_subcmd=remove
{function}
''')
            calls = (self.root / 'calls').read_text()
            for package in ('netdata-repo', 'netdata-repo-edge', 'netdata-repo-preview'):
                self.assertIn(package, calls)
            (self.root / 'calls').unlink()

    def test_repository_availability(self):
        for repo in ('netdata', 'netdata-edge', 'netdata-preview'):
            for distro in ('fedora', 'opensuse'):
                with self.subTest(repo=repo, distro=distro):
                    self.shell(f'''
DISTRO_COMPAT_NAME={distro}
pm_cmd=fixture_pm
fixture_pm() {{ echo 'From repo : {repo}'; }}
zypper() {{
  case "$1" in
    repos) echo '1 | {repo} | Netdata | Yes' ;;
    packages) [ "$3" = 1 ] && echo 'netdata ' ;;
  esac
}}
netdata_avail_check
''')

    def test_manifest_and_pinned_archive_sources(self):
        for kind in ('static', 'source'):
            for channel in ('preview', 'stable', 'nightly'):
                for version in ('', '2.13.0-rc1'):
                    with self.subTest(kind=kind, channel=channel, version=version):
                        release = f'v{version}' if version else 'latest'
                        result = self.shell(f'''
INSTALL_VERSION={version}
check_for_remote_file() {{ echo "$1"; return 0; }}
set_{kind}_archive_urls {channel}
[ "$NETDATA_MANIFEST_INSTALL" = 1 ] || exit 9
''')
                        self.assertEqual(result.stdout.strip(),
                                         f'https://artifacts.netdata.cloud/{channel}/{release}/Manifest')

    def test_archive_fallback_stays_in_selected_channel(self):
        for kind in ('static', 'source'):
            for channel, repository, tag in [('stable', 'netdata', 'v2.12.0'),
                                             ('nightly', 'netdata-nightlies', 'v2.13.0-123-nightly')]:
                with self.subTest(kind=kind, channel=channel):
                    self.shell(f'''
check_for_remote_file() {{ case "$1" in */Manifest) return 1 ;; *) return 0 ;; esac; }}
get_redirect() {{ echo {tag}; }}
set_{kind}_archive_urls {channel}
[ "$NETDATA_MANIFEST_INSTALL" = 0 ] || exit 9
expected=https://github.com/netdata/{repository}/releases/download/{tag}
[ "$NETDATA_{kind.upper()}_ARCHIVE_URL_PREFIX" = "$expected" ] || exit 9
''')
        self.shell('''
check_for_remote_file() { return 1; }
download() { return 1; }
set_source_archive_urls preview
''', success=False)

    def test_preview_never_falls_back_to_github(self):
        for kind in ('static', 'source'):
            for version in ('', '2.13.0-rc1', '2.13.0'):
                for remote_status in (1, 2):
                    with self.subTest(kind=kind, version=version, status=remote_status):
                        result = self.shell(f'''
INSTALL_VERSION={version}
check_for_remote_file() {{ echo "$1"; return {remote_status}; }}
get_redirect() {{ echo 'unexpected GitHub redirect' >&2; exit 8; }}
set_{kind}_archive_urls preview
''', success=False)
                        release = f'v{version}' if version else 'latest'
                        self.assertEqual(result.stdout.strip(),
                                         f'https://artifacts.netdata.cloud/preview/{release}/Manifest')
                        self.assertNotIn('GitHub', result.stderr)
            self.shell(f'''
INSTALL_VERSION=2.13.0-rc1
NETDATA_ARTIFACT_URL_PREFIX=https://mirror.example/preview
check_for_remote_file() {{ case "$1" in */Manifest) return 1 ;; *) return 0 ;; esac; }}
set_{kind}_archive_urls preview
[ "$NETDATA_{kind.upper()}_ARCHIVE_URL_PREFIX" = https://mirror.example/preview ] || exit 9
''')

    def test_preview_native_latest_is_not_pinned(self):
        for distro in ('debian', 'fedora', 'opensuse'):
            self.shell(f'''
DISTRO_COMPAT_NAME={distro}
SYSVERSION=12
SYSCODENAME=bookworm
INSTALL_VERSION=
configure_native_package_manager
[ -z "$NATIVE_VERSION" ] || exit 9
''')

    def test_channel_persistence(self):
        for layout in ('etc/netdata', 'netdata/etc/netdata'):
            config = self.root / layout / '.environment'
            config.parent.mkdir(parents=True, exist_ok=True)
            config.write_text('RELEASE_CHANNEL="nightly"\nOTHER="preserved"\n')
            self.shell('''
run_as_root() { "$@"; }
INSTALL_PREFIX="$PWD"
SELECTED_INSTALL_METHOD=build
save_release_channel
''')
            self.assertEqual(config.read_text(), 'RELEASE_CHANNEL="preview"\nOTHER="preserved"\n')
            config.unlink()
        self.shell('''
run_as_root() { "$@"; }
SELECTED_INSTALL_METHOD=static
INSTALL_PREFIX="$PWD"
save_release_channel
''', success=False)

    def test_unpublished_native_preview_fails(self):
        self.shell('''
NETDATA_REQUESTED_INSTALL_TYPE=native
try_package_install() { return 4; }
install_on_linux
''', success=False)


if __name__ == '__main__':
    unittest.main()
