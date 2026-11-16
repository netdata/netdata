#!/usr/bin/env python3
"""Validate IPMI platform defaults and native component staging without a full build."""
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
MODULES = ROOT / 'packaging/cmake/Modules'


@unittest.skipUnless(shutil.which('cmake'), 'CMake is required')
class IPMIDeliveryTests(unittest.TestCase):
    def configure(self, platform='Linux', *options, success=True, os_release=None):
        temporary = tempfile.TemporaryDirectory(prefix='netdata-ipmi-delivery-')
        self.addCleanup(temporary.cleanup)
        source = Path(temporary.name)
        if os_release is not None:
            release_file = source / 'os-release'
            release_file.write_text(os_release)
            options += ('-DOS_LINUX=ON', '-DOS_RELEASE_PATH=' + str(release_file))
        (source / 'src').symlink_to(ROOT / 'src', target_is_directory=True)
        for name in ('LICENSE', 'README.md'):
            (source / name).symlink_to(ROOT / name)
        (source / 'CMakeLists.txt').write_text('''cmake_minimum_required(VERSION 3.20)
project(IPMIDelivery NONE)
list(APPEND CMAKE_MODULE_PATH "''' + str(MODULES) + '''")
include(NetdataOptions)
set(NETDATA_PACKAGE_VERSION 1.2.3)
set(NETDATA_STAGE_HOST_FILES TRUE)
set(PKG_FILES_PATH "''' + str(ROOT / 'packaging/cmake/pkg-files') + '''")
set(PKG_DOC_DEST usr/share/doc)
set(PLUGINS_DEST usr/libexec/netdata/plugins.d)
set(LIBCONFIG_DEST usr/lib/netdata/conf.d)
include(NetdataUtil)
function(add_go_target target binary)
  file(WRITE "${CMAKE_BINARY_DIR}/${binary}" "fixture binary")
endfunction()
include(NetdataPluginIPMI)
include(Packaging)
file(WRITE "${CMAKE_BINARY_DIR}/ipmi-enabled" "${ENABLE_PLUGIN_IPMI}")
''')
        build = source / 'build'
        result = subprocess.run(['cmake', '-S', str(source), '-B', str(build),
                                 '-DCMAKE_SYSTEM_NAME=' + platform, *options],
                                capture_output=True, text=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('The Go IPMI plugin requires Linux', result.stdout + result.stderr)
        return build

    def test_platform_and_feature_defaults(self):
        for platform, options, expected in (
                ('Linux', (), 'ON'),
                ('Linux', ('-DDEFAULT_FEATURE_STATE=OFF',), 'OFF'),
                ('Linux', ('-DENABLE_PLUGIN_IPMI=OFF',), 'OFF'),
                ('Linux', ('-DDEFAULT_FEATURE_STATE=OFF', '-DENABLE_PLUGIN_IPMI=ON'), 'ON'),
                ('Darwin', (), 'False'),
                ('FreeBSD', (), 'False')):
            with self.subTest(platform=platform, options=options):
                build = self.configure(platform, *options)
                self.assertEqual((build / 'ipmi-enabled').read_text(), expected)
                config = (build / 'CPackConfig.cmake').read_text()
                components = next(line for line in config.splitlines()
                                  if line.startswith('set(CPACK_COMPONENTS_ALL '))
                self.assertEqual('plugin-ipmi' in components, expected == 'ON')

    def test_explicit_enable_on_non_linux_still_fails(self):
        for platform in ('Darwin', 'FreeBSD'):
            with self.subTest(platform=platform):
                self.configure(platform, '-DENABLE_PLUGIN_IPMI=ON', success=False)

    def test_native_metadata_and_component_install(self):
        build = self.configure('Linux', '-DNETDATA_PACKAGE_KIND=deb')
        config = (build / 'CPackConfig.cmake').read_text()
        for fragment in (
                'set(CPACK_DEBIAN_PLUGIN-IPMI_PACKAGE_NAME "netdata-plugin-ipmi")',
                'set(CPACK_DEBIAN_PLUGIN-IPMI_PACKAGE_PREDEPENDS "netdata-user")',
                'set(CPACK_RPM_PLUGIN-IPMI_PACKAGE_REQUIRES "netdata = 1.2.3")',
                '%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/ipmi.plugin',
                '/deb/plugin-ipmi/postinst'):
            self.assertIn(fragment, config)
        for key in ('CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS', 'CPACK_RPM_NETDATA_PACKAGE_REQUIRES'):
            line = next(line for line in config.splitlines() if line.startswith('set(' + key + ' '))
            self.assertNotIn('netdata-plugin-ipmi', line)
        suggests = next(line for line in config.splitlines()
                        if line.startswith('set(CPACK_DEBIAN_NETDATA_PACKAGE_SUGGESTS '))
        self.assertIn('netdata-plugin-ipmi', suggests)
        destination = build / 'stage'
        result = subprocess.run(['cmake', '--install', str(build), '--prefix', str(destination),
                                 '--component', 'plugin-ipmi'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        for path in ('usr/libexec/netdata/plugins.d/ipmi.plugin',
                     'usr/lib/netdata/conf.d/ipmi.conf',
                     'usr/lib/netdata/conf.d/ipmi/ipmi.conf',
                     'usr/share/doc/netdata-plugin-ipmi/copyright'):
            self.assertTrue((destination / path).is_file(), path)
        self.assertTrue(os.access(destination / 'usr/libexec/netdata/plugins.d/ipmi.plugin', os.X_OK))

    def test_rpm_ipmi_dependency_is_optional(self):
        for distro, version, weak_deps in (
                ('centos', '7', False), ('centos', '10', True),
                ('amzn', '2', False), ('amzn', '2023', True), ('fedora', '43', True)):
            for enabled in (True, False):
                with self.subTest(distro=distro, version=version, enabled=enabled):
                    build = self.configure('Linux', '-DNETDATA_PACKAGE_KIND=rpm',
                                           '-DENABLE_PLUGIN_IPMI=' + ('ON' if enabled else 'OFF'),
                                           os_release=f'ID={distro}\nVERSION_ID={version}\n')
                    config = (build / 'CPackConfig.cmake').read_text().splitlines()
                    requires = next(line for line in config
                                    if line.startswith('set(CPACK_RPM_NETDATA_PACKAGE_REQUIRES '))
                    suggests = next((line for line in config
                                     if line.startswith('set(CPACK_RPM_NETDATA_PACKAGE_SUGGESTS ')), '')
                    self.assertNotIn('netdata-plugin-ipmi', requires)
                    self.assertEqual('netdata-plugin-ipmi = 1.2.3' in suggests, enabled and weak_deps)

    def test_rpm_account_creation_and_payload_ordering(self):
        for distro, version, post, predep in (
                ('centos', '9', 'post.create-user', True),
                ('centos', '10', 'post.create-user', True),
                ('rocky', '10', 'post.create-user', True),
                ('ol', '10', 'post.create-user', True),
                ('fedora', '42', 'post.create-user', True),
                ('fedora', '43', 'post.groups-only', False),
                ('opensuse-leap', '15.6', 'post.create-user', False)):
            with self.subTest(distro=distro, version=version):
                build = self.configure('Linux', '-DNETDATA_PACKAGE_KIND=rpm',
                                       os_release=f'ID={distro}\nVERSION_ID={version}\n')
                config = (build / 'CPackConfig.cmake').read_text()
                script = ROOT / 'packaging/cmake/pkg-files/rpm/user' / post
                self.assertIn(f'set(CPACK_RPM_USER_POST_INSTALL_SCRIPT_FILE "{script}")', config)
                for component in ('NETDATA', 'PLUGIN-IPMI', 'PLUGIN-FREEIPMI', 'PLUGIN-GO'):
                    dependency = f'set(CPACK_RPM_{component}_PACKAGE_REQUIRES_PRE "netdata-user >= 1.2.3")'
                    self.assertEqual(dependency in config, predep, component)
                requires = '/usr/sbin/useradd, /usr/sbin/groupadd' if predep else 'systemd'
                self.assertIn(f'set(CPACK_RPM_USER_PACKAGE_REQUIRES "{requires}")', config)


class IPMIPermissionTests(unittest.TestCase):
    def test_docker_permissions_with_ipmi_installed_or_disabled(self):
        source = (ROOT / 'packaging/docker/Dockerfile').read_text()
        for directory, boundary in (
                ('/app/usr/libexec/netdata/plugins.d', '# Group write permissions'),
                ('/usr/libexec/netdata/plugins.d', 'cp -va /etc/netdata')):
            end = source.index(boundary)
            begin = source.rindex('done &&', 0, end) + len('done &&')
            block = source[begin:end].replace('\\\n', '').strip()
            self.assertTrue(block.endswith('&&'))
            block = block[:-2].strip()
            for installed in (False, True):
                with self.subTest(directory=directory, installed=installed), \
                        tempfile.TemporaryDirectory(prefix='netdata-ipmi-docker-') as temporary:
                    plugin = Path(temporary) / 'ipmi.plugin'
                    if installed:
                        plugin.touch()
                        plugin.chmod(0o755)
                    script = 'chown() { printf "%s\\n" "$*"; }\n'
                    if sys.platform == 'darwin':
                        stat_mode = 'import os, sys; print(format(os.stat(sys.argv[1]).st_mode & 0o7777, "o"))'
                        script += ('stat() { ' + shlex.quote(sys.executable) + ' -c '
                                   + shlex.quote(stat_mode) + ' "$3"; }\n')
                    script += block.replace(directory, temporary)
                    result = subprocess.run(['sh', '-c', script], capture_output=True, text=True,
                                            env=dict(os.environ, NETDATA_GID='1234'))
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    if installed:
                        self.assertEqual(plugin.stat().st_mode & 0o7777, 0o4750)
                        if directory.startswith('/usr/'):
                            self.assertEqual(result.stdout.strip(), 'root:1234 ' + str(plugin))
                    else:
                        self.assertFalse(plugin.exists())
                        self.assertEqual(result.stdout, '')

    def test_source_installer_platform_default_and_overrides(self):
        source = (ROOT / 'netdata-installer.sh').read_text()
        begin = source.index('DONOTSTART=0')
        end = source.index('\ndone', source.index('while [ -n "${1}" ]; do', begin)) + len('\ndone')
        script = 'uname() { printf "%s\\n" "$TEST_PLATFORM"; }\n' + source[begin:end]
        script += '\nprintf "%s\\n" "$ENABLE_IPMI"\n'
        for platform, options, expected in (('Linux', (), '1'), ('FreeBSD', (), '0'),
                                            ('Darwin', (), '0'),
                                            ('Linux', ('--disable-plugin-ipmi',), '0'),
                                            ('Linux', ('--enable-plugin-ipmi',), '1'),
                                            ('Linux', ('--disable-go',), '0'),
                                            ('Linux', ('--enable-plugin-ipmi', '--disable-go'), '0'),
                                            ('Linux', ('--disable-go', '--enable-plugin-ipmi'), '1'),
                                            ('Linux', ('--disable-plugin-go',), '1'),
                                            ('Linux', ('--enable-plugin-ipmi', '--disable-plugin-ipmi'), '0'),
                                            ('Linux', ('--disable-plugin-ipmi', '--enable-plugin-ipmi'), '1')):
            with self.subTest(platform=platform, options=options):
                result = subprocess.run(['bash', '-c', script, 'installer-fixture', *options],
                                        capture_output=True, text=True,
                                        env=dict(os.environ, TEST_PLATFORM=platform))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.strip(), expected)

    def test_disable_go_clears_all_components_and_skips_toolchain(self):
        source = (ROOT / 'netdata-installer.sh').read_text()
        begin = source.index('DONOTSTART=0')
        end = source.index('\ndone', source.index('while [ -n "${1}" ]; do', begin)) + len('\ndone')
        parser = 'uname() { echo Linux; }\n' + source[begin:end]
        flags = ('ENABLE_GO', 'ENABLE_IBM', 'ENABLE_SCRIPTS', 'ENABLE_STATSD', 'ENABLE_IPMI', 'ENABLE_DEM')
        output = '\nprintf "%s\\n" ' + ' '.join('"$' + flag + '"' for flag in flags)
        for options, expected in (
                (('--disable-go',), ['0'] * 6),
                (('--enable-plugin-ibm', '--enable-plugin-scripts', '--enable-plugin-statsd',
                  '--enable-plugin-ipmi', '--enable-plugin-dem', '--disable-go'), ['0'] * 6),
                (('--enable-plugin-ipmi', '--disable-go'), ['0'] * 6),
                (('--disable-go', '--enable-plugin-ipmi'), ['0', '0', '0', '0', '1', '0']),
                (('--disable-go', '--enable-plugin-scripts'), ['0', '0', '1', '0', '0', '0']),
                (('--disable-go', '--enable-plugin-go', '--enable-plugin-ibm', '--enable-plugin-scripts',
                  '--enable-plugin-statsd', '--enable-plugin-ipmi', '--enable-plugin-dem'), ['1'] * 6),
                (('--disable-plugin-go',), ['0', '0', '1', '0', '1', '1'])):
            with self.subTest(options=options):
                result = subprocess.run(['bash', '-c', parser + output, 'installer-fixture', *options],
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.splitlines(), expected)
        begin = source.index('NEED_GO_TOOLCHAIN=0')
        end = source.index('# If we\'re installing the IBM plugin', begin)
        script = parser + '\n' + source[begin:end] + '\nprintf "%s\\n" "$NEED_GO_TOOLCHAIN"\n'
        result = subprocess.run(['bash', '-e', '-c', script, 'installer-fixture', '--disable-go'],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), '0')

    def test_static_installer_assigns_dedicated_privileges(self):
        with tempfile.TemporaryDirectory(prefix='netdata-ipmi-permissions-') as temporary:
            root = Path(temporary)
            plugins = root / 'usr/libexec/netdata/plugins.d'
            plugins.mkdir(parents=True)
            for name in ('ipmi.plugin', 'freeipmi.plugin'):
                (plugins / name).touch()
            source = (ROOT / 'packaging/makeself/install-or-update.sh').read_text()
            begin = source.index('progress "changing plugins ownership and permissions"')
            end = source.index('for x in otel-plugin', begin)
            script = 'progress() { :; }; run() { printf "%s\\n" "$*"; }; NETDATA_GROUP=netdata\n'
            result = subprocess.run(['bash', '-c', script + source[begin:end]],
                                    cwd=root, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            operations = result.stdout.splitlines()
            for name in ('ipmi.plugin', 'freeipmi.plugin'):
                path = 'usr/libexec/netdata/plugins.d/' + name
                owner = operations.index('chown root:netdata ' + path)
                mode = operations.index('chmod 4750 ' + path)
                self.assertLess(owner, mode)
            self.assertFalse(any('go.d.plugin' in line for line in operations))

    def test_static_check_scans_installed_elf_executables(self):
        with tempfile.TemporaryDirectory(prefix='netdata-ipmi-static-check-') as temporary:
            root = Path(temporary)
            (root / 'functions.sh').write_text('set -euo pipefail\nrun() { "$@"; }\n')
            commands = root / 'commands'
            commands.mkdir()
            readelf = commands / 'readelf'
            readelf.write_text('#!/bin/sh\nprintf "%s\\0" "$2" >> "$ELF_LOG"\n'
                               'if [ "$2" = "$DYNAMIC_ELF" ]; then echo INTERP; else echo LOAD; fi\n'
                               'exit "$ELF_STATUS"\n')
            readelf.chmod(0o755)
            ldd = commands / 'ldd'
            ldd.write_text('#!/bin/sh\nexit 0\n')
            ldd.chmod(0o755)
            binaries = [root / path for path in ('bin/netdata', 'usr/libexec/netdata/plugins.d/ipmi.plugin',
                                                 'other tools/extra\nplugin')]
            for binary in binaries:
                binary.parent.mkdir(parents=True, exist_ok=True)
                binary.write_bytes(b'\x7fELFfixture')
                binary.chmod(0o755)
            (root / 'non-executable').write_bytes(b'\x7fELFdata')
            (root / 'non-executable').chmod(0o644)
            (root / 'alias').symlink_to(binaries[0])
            log = root / 'inspected'
            environment = dict(os.environ, NETDATA_MAKESELF_PATH=str(root),
                               NETDATA_INSTALL_PATH=str(root), GITHUB_ACTIONS='false',
                               PATH=str(commands) + ':' + os.environ['PATH'], ELF_LOG=str(log),
                               ELF_STATUS='0', DYNAMIC_ELF='')
            job = ROOT / 'packaging/makeself/jobs/80-netdata-static-check.sh'

            def run_check(**overrides):
                log.write_text('')
                return subprocess.run(['bash', str(job)], capture_output=True, text=True,
                                      env=dict(environment, **overrides))

            result = run_check()
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(set(log.read_text().strip('\0').split('\0')), set(map(str, binaries)))
            for binary in binaries:
                with self.subTest(dynamic=binary):
                    result = run_check(DYNAMIC_ELF=str(binary))
                    self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                    self.assertIn(str(binary) + ' is not a statically linked binary', result.stderr)
            result = run_check(ELF_STATUS='7')
            self.assertEqual(result.returncode, 7, result.stdout + result.stderr)
            self.assertIn('Failed to inspect ELF program headers', result.stderr)

            # File presence belongs to the runtime check, not this linkage scan.
            binaries[1].unlink()
            result = run_check()
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = run_check(NETDATA_INSTALL_PATH=str(root / 'missing-tree'))
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_runtime_check_requires_ipmi_plugin(self):
        source = (ROOT / 'packaging/runtime-check.sh').read_text()
        checks = source[source.index('NETDATA_LIBEXEC_PARTS="'):]
        parts = checks.split('"', 2)[1].splitlines()
        with tempfile.TemporaryDirectory(prefix='netdata-ipmi-runtime-check-') as temporary:
            root = Path(temporary)
            for part in filter(None, parts):
                if part == 'plugins.d/ipmi.plugin':
                    continue
                binary = root / part
                binary.parent.mkdir(parents=True, exist_ok=True)
                binary.touch()
                binary.chmod(0o755)
            script = 'check_systemd_capability_bounding_set() { return 0; }\n' + checks
            for skip, expected in (('freeipmi|xenstat|cups', 1), ('freeipmi|xenstat|cups|ipmi', 0)):
                with self.subTest(skip=skip):
                    result = subprocess.run(['sh', '-c', script], capture_output=True, text=True,
                                            env=dict(os.environ, NETDATA_LIBEXEC_PREFIX=str(root),
                                                     NETDATA_SKIP_LIBEXEC_PARTS=skip))
                    self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                    if expected:
                        self.assertIn(str(root / 'plugins.d/ipmi.plugin') + ' is missing', result.stdout)

    def test_debian_postinst_assigns_dedicated_privileges(self):
        with tempfile.TemporaryDirectory(prefix='netdata-ipmi-postinst-') as temporary:
            commands = Path(temporary)
            for name in ('chown', 'chmod'):
                command = commands / name
                command.write_text('#!/bin/sh\nprintf "%s" "' + name + '"\nprintf " %s" "$@"\nprintf "\\n"\n')
                command.chmod(0o755)
            result = subprocess.run(['sh', str(ROOT / 'packaging/cmake/pkg-files/deb/plugin-ipmi/postinst'),
                                     'configure'], capture_output=True, text=True,
                                    env=dict(os.environ, PATH=str(commands) + ':' + os.environ['PATH']))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), [
                'chown root:netdata /usr/libexec/netdata/plugins.d/ipmi.plugin',
                'chmod -f 4750 /usr/libexec/netdata/plugins.d/ipmi.plugin'])


if __name__ == '__main__':
    unittest.main()
