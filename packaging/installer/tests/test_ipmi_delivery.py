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
    def configure(self, platform='Linux', *options, success=True):
        temporary = tempfile.TemporaryDirectory(prefix='netdata-ipmi-delivery-')
        self.addCleanup(temporary.cleanup)
        source = Path(temporary.name)
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
            self.assertIn('netdata-plugin-ipmi', line)
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
                                            ('Linux', ('--enable-plugin-ipmi',), '1')):
            with self.subTest(platform=platform, options=options):
                result = subprocess.run(['bash', '-c', script, 'installer-fixture', *options],
                                        capture_output=True, text=True,
                                        env=dict(os.environ, TEST_PLATFORM=platform))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.strip(), expected)

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

    def test_static_check_rejects_dynamic_missing_and_unreadable_binaries(self):
        with tempfile.TemporaryDirectory(prefix='netdata-ipmi-static-check-') as temporary:
            root = Path(temporary)
            (root / 'functions.sh').write_text('run() { "$@"; }\n')
            commands = root / 'commands'
            commands.mkdir()
            readelf = commands / 'readelf'
            readelf.write_text('#!/bin/sh\nprintf "%s\\n" "$ELF_HEADERS"\nexit "$ELF_STATUS"\n')
            readelf.chmod(0o755)
            ldd = commands / 'ldd'
            ldd.write_text('#!/bin/sh\nexit 0\n')
            ldd.chmod(0o755)
            for path in ('bin/netdata', 'usr/libexec/netdata/plugins.d/ipmi.plugin'):
                binary = root / path
                binary.parent.mkdir(parents=True, exist_ok=True)
                binary.touch()
                binary.chmod(0o755)
            environment = dict(os.environ, NETDATA_MAKESELF_PATH=str(root),
                               NETDATA_INSTALL_PATH=str(root), GITHUB_ACTIONS='false',
                               PATH=str(commands) + ':' + os.environ['PATH'])
            job = ROOT / 'packaging/makeself/jobs/80-netdata-static-check.sh'
            for headers, status, expected in (('LOAD', '0', 0), ('INTERP', '0', 1), ('', '7', 7)):
                with self.subTest(headers=headers, status=status):
                    result = subprocess.run(['bash', str(job)], capture_output=True, text=True,
                                            env=dict(environment, ELF_HEADERS=headers, ELF_STATUS=status))
                    self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
            (root / 'usr/libexec/netdata/plugins.d/ipmi.plugin').chmod(0o644)
            result = subprocess.run(['bash', str(job)], capture_output=True, text=True,
                                    env=dict(environment, ELF_HEADERS='LOAD', ELF_STATUS='0'))
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)

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
