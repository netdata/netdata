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
    def configure(self, *, static=False, netflow=False, dem=False, bits=64, staged=True, missing=None):
        temporary = tempfile.TemporaryDirectory(prefix='netdata-topology-stock-')
        self.addCleanup(temporary.cleanup)
        source = Path(temporary.name)
        stock = source / 'stock'
        stock.mkdir()
        for name in STOCK_FILES:
            if name != missing:
                (stock / name).write_text('stock fixture: ' + name)
        (source / 'src').symlink_to(ROOT / 'src', target_is_directory=True)
        (source / 'CMakeLists.txt').write_text('''cmake_minimum_required(VERSION 3.20)
project(TopologyStockDelivery NONE)
list(APPEND CMAKE_MODULE_PATH "''' + str(ROOT / 'packaging/cmake/Modules') + '''")
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
function(netdata_add_deb_copyright)
endfunction()
include(NetdataPluginNetflow)
include(NetdataTopologyIPIntel)
''')
        build = source / 'build'
        result = subprocess.run([
            'cmake', '-S', str(source), '-B', str(build),
            '-DSTATIC_BUILD=' + ('ON' if static else 'OFF'),
            '-DENABLE_PLUGIN_NETFLOW=' + ('ON' if netflow else 'OFF'),
            '-DENABLE_PLUGIN_DEM=' + ('ON' if dem else 'OFF'),
            '-DCMAKE_SIZEOF_VOID_P=' + str(bits // 8),
            '-DNETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR=' + (str(stock) if staged else ''),
        ], capture_output=True, text=True)
        if missing is None:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
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

    def test_static_netflow_keeps_stock_without_dem(self):
        for bits in (32, 64):
            with self.subTest(bits=bits):
                self.assert_stock(self.install(self.configure(static=True, netflow=True, bits=bits)))

    def test_native_stock_remains_owned_only_by_netflow(self):
        build = self.configure(netflow=True, dem=True)
        self.assert_stock(self.install(build, 'plugin-netflow'))
        for component in ('plugin-dem', 'netdata'):
            with self.subTest(component=component):
                self.assertFalse((self.install(build, component) / STOCK_DEST).exists())

    def test_disabled_consumers_and_native_dem_do_not_add_stock(self):
        for static, dem in ((True, False), (False, False), (False, True)):
            with self.subTest(static=static, dem=dem):
                destination = self.install(self.configure(static=static, dem=dem))
                self.assertFalse((destination / STOCK_DEST).exists())

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


if __name__ == '__main__':
    unittest.main()
