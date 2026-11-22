#!/usr/bin/env python3
"""Exercise shared IP intelligence data installation through real CMake rules."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
STOCK_FILES = ('README.md', 'topology-ip-asn.mmdb', 'topology-ip-geo.mmdb', 'topology-ip-intel.json')
STOCK_DEST = Path('usr/share/netdata/topology-ip-intel')


@unittest.skipUnless(shutil.which('cmake'), 'CMake is required')
class TopologyStockDeliveryTests(unittest.TestCase):
    def configure(self, *, static=False, netflow=False, dem=False, bits=64, staged=True, missing=None,
                  kind='bundle', metadata=False, version='2.13.1', success=True):
        temporary = tempfile.TemporaryDirectory(prefix='netdata-topology-stock-')
        self.addCleanup(temporary.cleanup)
        source = Path(temporary.name)
        stock = source / 'stock'
        stock.mkdir()
        for name in STOCK_FILES:
            if name != missing:
                (stock / name).write_text('stock fixture: ' + name)
        (source / 'src').symlink_to(ROOT / 'src', target_is_directory=True)
        for name in ('packaging', 'LICENSE', 'README.md'):
            (source / name).symlink_to(ROOT / name)
        (source / 'CMakeLists.txt').write_text('''cmake_minimum_required(VERSION 3.20)
project(TopologyStockDelivery NONE)
list(APPEND CMAKE_MODULE_PATH "''' + str(ROOT / 'packaging/cmake/Modules') + '''")
set(PKG_FILES_PATH "${CMAKE_SOURCE_DIR}/packaging/cmake/pkg-files")
set(PKG_DOC_DEST usr/share/doc)
set(NETDATA_STAGE_HOST_FILES TRUE)
set(OS_LINUX TRUE)
set(LIBEXEC_DEST usr/libexec/netdata)
set(PLUGINS_DEST usr/libexec/netdata/plugins.d)
set(LIBCONFIG_DEST usr/lib/netdata/conf.d)
set(STOCK_DATA_DEST usr/share/netdata)
set(BINDIR usr/sbin)
# Executable compilation is outside this data installation test.
function(corrosion_install)
endfunction()
function(add_go_target target binary)
  file(WRITE "${CMAKE_BINARY_DIR}/${binary}" "fixture binary")
endfunction()
include(NetdataUtil)
include(NetdataPluginDem)
include(NetdataPluginNetflow)
include(NetdataTopologyIPIntel)
if(TEST_METADATA)
  set(NETDATA_PACKAGE_VERSION "${TEST_PACKAGE_VERSION}")
  include(Packaging)
  # Read evaluated CPack properties, not the spelling of the CMake source.
  foreach(key COMPONENTS_ALL COMPONENT_PLUGIN-DEM_DEPENDS COMPONENT_PLUGIN-NETFLOW_DEPENDS
              DEBIAN_IP-INTEL-DATA_PACKAGE_NAME DEBIAN_IP-INTEL-DATA_PACKAGE_BREAKS
              DEBIAN_IP-INTEL-DATA_PACKAGE_REPLACES DEBIAN_PLUGIN-DEM_PACKAGE_SUGGESTS
              RPM_PLUGIN-DEM_PACKAGE_REQUIRES RPM_PLUGIN-NETFLOW_PACKAGE_REQUIRES
              RPM_IP-INTEL-DATA_PACKAGE_CONFLICTS RPM_IP-INTEL-DATA_PACKAGE_REQUIRES
              RPM_IP-INTEL-DATA_PACKAGE_OBSOLETES RPM_IP-INTEL-DATA_PACKAGE_REQUIRES_PRE
              RPM_IP-INTEL-DATA_DEFAULT_USER RPM_IP-INTEL-DATA_DEFAULT_GROUP
              DEBIAN_PLUGIN-NETFLOW_PACKAGE_CONTROL_EXTRA
              RPM_PLUGIN-NETFLOW_POST_UNINSTALL_SCRIPT_FILE)
    file(APPEND "${CMAKE_BINARY_DIR}/package-properties" "${key}=${CPACK_${key}}\\n")
  endforeach()
endif()
''')
        build = source / 'build'
        result = subprocess.run([
            'cmake', '-S', str(source), '-B', str(build),
            '-DSTATIC_BUILD=' + ('ON' if static else 'OFF'),
            '-DENABLE_PLUGIN_NETFLOW=' + ('ON' if netflow else 'OFF'),
            '-DENABLE_PLUGIN_DEM=' + ('ON' if dem else 'OFF'),
            '-DCMAKE_SIZEOF_VOID_P=' + str(bits // 8),
            '-DNETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR=' + (str(stock) if staged else ''),
            '-DNETDATA_PACKAGE_KIND=' + kind,
            '-DTEST_METADATA=' + ('ON' if metadata else 'OFF'),
            '-DTEST_PACKAGE_VERSION=' + version,
        ], capture_output=True, text=True)
        if missing is None and success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            if missing:
                self.assertIn(missing, result.stdout + result.stderr)
        return build

    def install(self, build, component=None):
        destination = build / ('stage-' + (component or 'all'))
        args = ['cmake', '--install', str(build), '--prefix', str(destination)]
        if component:
            args += ['--component', component]
        env = os.environ.copy()
        env.pop('DESTDIR', None)
        result = subprocess.run(args, env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return destination

    def assert_stock(self, destination):
        self.assertEqual({p.name for p in (destination / STOCK_DEST).iterdir()}, set(STOCK_FILES))
        for name in STOCK_FILES:
            self.assertEqual((destination / STOCK_DEST / name).read_text(), 'stock fixture: ' + name)

    def test_static_dem_without_netflow_installs_stock(self):
        for bits in (32, 64):
            with self.subTest(bits=bits):
                destination = self.install(self.configure(static=True, dem=True, bits=bits))
                self.assert_stock(destination)
                self.assertFalse((destination / 'usr/sbin/topology-ip-intel-downloader').exists())
                self.assertFalse((destination / 'usr/share/doc/netdata-ip-intel-data').exists())

    def test_static_netflow_keeps_stock_without_dem(self):
        for bits in (32, 64):
            with self.subTest(bits=bits):
                self.assert_stock(self.install(self.configure(static=True, netflow=True, bits=bits)))

    def test_native_stock_has_one_shared_owner_with_either_consumer(self):
        for kind in ('deb', 'rpm'):
            for netflow, dem in ((True, False), (False, True), (True, True)):
                with self.subTest(kind=kind, netflow=netflow, dem=dem):
                    build = self.configure(kind=kind, netflow=netflow, dem=dem)
                    destination = self.install(build, 'ip-intel-data')
                    self.assert_stock(destination)
                    copyright_file = destination / 'usr/share/doc/netdata-ip-intel-data/copyright'
                    self.assertEqual(copyright_file.read_bytes(),
                                     (ROOT / 'packaging/cmake/pkg-files/ip-intel-data/copyright').read_bytes())
                    self.assertFalse((destination / 'usr/sbin/topology-ip-intel-downloader').exists())
                    for component in ('plugin-netflow', 'plugin-dem', 'netdata'):
                        self.assertFalse((self.install(build, component) / STOCK_DEST).exists())

    def test_disabled_consumers_do_not_add_stock(self):
        for static in (True, False):
            with self.subTest(static=static):
                destination = self.install(self.configure(static=static))
                self.assertFalse((destination / STOCK_DEST).exists())

    def test_native_consumers_require_staged_data(self):
        for kind in ('deb', 'rpm'):
            for netflow, dem in ((True, False), (False, True)):
                with self.subTest(kind=kind, netflow=netflow, dem=dem):
                    self.configure(kind=kind, netflow=netflow, dem=dem, staged=False, success=False)

    def test_unstaged_source_build_remains_optional(self):
        for netflow in (False, True):
            with self.subTest(netflow=netflow):
                destination = self.install(self.configure(static=True, netflow=netflow, dem=True, staged=False))
                self.assertFalse((destination / STOCK_DEST).exists())

    def test_incomplete_staged_payload_fails_configuration(self):
        for netflow in (False, True):
            for name in STOCK_FILES:
                with self.subTest(netflow=netflow, missing=name):
                    self.configure(static=True, netflow=netflow, dem=True, missing=name)

    def test_package_relationships_coordinate_the_shared_owner(self):
        for version in ('2.13.1', '2.13.0.20.nightly'):
            with self.subTest(version=version):
                build = self.configure(netflow=True, dem=True, kind='deb', metadata=True, version=version)
                props = dict(line.split('=', 1) for line in (build / 'package-properties').read_text().splitlines())
                self.assertIn('ip-intel-data', props['COMPONENTS_ALL'].split(';'))
                for plugin in ('DEM', 'NETFLOW'):
                    self.assertEqual(set(props[f'COMPONENT_PLUGIN-{plugin}_DEPENDS'].split(';')),
                                     {'netdata', 'ip-intel-data'})
                    self.assertEqual(set(props[f'RPM_PLUGIN-{plugin}_PACKAGE_REQUIRES'].split(', ')),
                                     {f'netdata = {version}', f'netdata-ip-intel-data = {version}'})
                for relationship in ('BREAKS', 'REPLACES'):
                    self.assertEqual(props[f'DEBIAN_IP-INTEL-DATA_PACKAGE_{relationship}'],
                                     f'netdata-plugin-netflow (<< {version})')
                self.assertEqual(props['RPM_IP-INTEL-DATA_PACKAGE_CONFLICTS'],
                                 f'netdata-plugin-netflow < {version}')
                self.assertEqual(props['DEBIAN_PLUGIN-DEM_PACKAGE_SUGGESTS'], 'netdata-dem-runtime')
                for relationship in ('REQUIRES', 'REQUIRES_PRE', 'OBSOLETES'):
                    self.assertEqual(props[f'RPM_IP-INTEL-DATA_PACKAGE_{relationship}'], '')
                for field in ('USER', 'GROUP'):
                    self.assertEqual(props[f'RPM_IP-INTEL-DATA_DEFAULT_{field}'], 'root')

    def removal_hook(self, kind):
        build = self.configure(netflow=True, kind=kind, metadata=True)
        props = dict(line.split('=', 1) for line in (build / 'package-properties').read_text().splitlines())
        if kind == 'deb':
            paths = props['DEBIAN_PLUGIN-NETFLOW_PACKAGE_CONTROL_EXTRA'].split(';')
            hooks = [Path(path) for path in paths if Path(path).name == 'postrm']
            self.assertEqual(len(hooks), 1, 'NetFlow must register a final-removal hook')
            hook = hooks[0]
        else:
            path = props['RPM_PLUGIN-NETFLOW_POST_UNINSTALL_SCRIPT_FILE']
            self.assertTrue(path, 'NetFlow must register a final-erasure hook')
            hook = Path(path)
        # Run the shipped hook with an isolated live-systemd directory and service helper.
        live = build / 'systemd'
        live.mkdir()
        script = build / 'removal-hook'
        script.write_text(hook.read_text().replace('/run/systemd/system', str(live)))
        tools = build / 'tools'
        tools.mkdir()
        helper = tools / ('deb-systemd-invoke' if kind == 'deb' else 'systemctl')
        helper.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$CALLS"\nexit "${SERVICE_STATUS:-0}"\n')
        helper.chmod(0o755)
        return script, live, tools, helper

    def invoke_removal(self, script, tools, phase, *, root='', helper_status=0):
        calls = tools.parent / 'calls'
        calls.write_text('')
        env = dict(os.environ, PATH=str(tools), CALLS=str(calls), DPKG_ROOT=root,
                   SERVICE_STATUS=str(helper_status))
        result = subprocess.run(['/bin/sh', str(script), phase], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return calls.read_text().splitlines()

    def test_netflow_removal_restarts_only_at_final_removal(self):
        for kind, final, inactive in (
            ('deb', 'remove', ('purge', 'upgrade', 'failed-upgrade', 'abort-install', 'abort-upgrade', 'disappear')),
            ('rpm', '0', ('1', '2')),
        ):
            with self.subTest(kind=kind):
                script, _, tools, _ = self.removal_hook(kind)
                self.assertEqual(self.invoke_removal(script, tools, final), ['try-restart netdata.service'])
                for phase in inactive:
                    with self.subTest(phase=phase):
                        self.assertEqual(self.invoke_removal(script, tools, phase), [])
                # A failed service action must not leave the package half-removed.
                self.assertEqual(self.invoke_removal(script, tools, final, helper_status=1),
                                 ['try-restart netdata.service'])

    def test_netflow_removal_skips_non_systemd_and_missing_helpers(self):
        for kind, final in (('deb', 'remove'), ('rpm', '0')):
            with self.subTest(kind=kind):
                script, live, tools, helper = self.removal_hook(kind)
                live.rmdir()
                self.assertEqual(self.invoke_removal(script, tools, final), [])
                live.mkdir()
                helper.unlink()
                self.assertEqual(self.invoke_removal(script, tools, final), [])

    def test_netflow_removal_does_not_control_host_from_dpkg_root(self):
        script, _, tools, _ = self.removal_hook('deb')
        self.assertEqual(self.invoke_removal(script, tools, 'remove', root='/target'), [])

    def test_package_registry_tracks_stock_production(self):
        for netflow, dem, staged in ((True, False, True), (False, True, True),
                                     (False, False, True), (True, True, False)):
            with self.subTest(netflow=netflow, dem=dem, staged=staged):
                build = self.configure(netflow=netflow, dem=dem, staged=staged, metadata=True)
                props = dict(line.split('=', 1) for line in (build / 'package-properties').read_text().splitlines())
                self.assertEqual('ip-intel-data' in props['COMPONENTS_ALL'].split(';'), staged and (netflow or dem))


if __name__ == '__main__':
    unittest.main()
