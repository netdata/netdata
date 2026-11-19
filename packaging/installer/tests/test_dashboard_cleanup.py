# SPDX-License-Identifier: GPL-3.0-or-later
import io
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]
MODULES = ROOT / 'packaging/cmake/Modules'
CLEANUP = ROOT / 'packaging/installer/cleanup-dashboard.sh'


class DashboardUpdateTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix='netdata-dashboard-')
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name).resolve()
        self.prefix = self.root / 'install prefix'
        self.web = self.prefix / 'usr/share/netdata/web'
        self.v3 = self.web / 'v3'
        self.incoming = self.root / 'incoming/v3'

    def put(self, path, content='keep'):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def cleanup(self):
        subprocess.run(['sh', str(CLEANUP), str(self.incoming), str(self.prefix),
                        'usr/share/netdata/web'], check=True)

    def test_replaces_only_stale_regular_files(self):
        keep = ['index.html', 'nested/current.css', '.well-known/data', 'assets with spaces/a [1].js']
        stale = ['custom.css', 'nested/.hidden', 'retired/old.gif', '-f', 'two\nlines', 'odd\ndir/obsolete.js']
        stale.extend(f'batch/{n}.chunk.js' for n in range(130))
        outside = self.root / 'outside'
        for name in keep:
            self.put(self.incoming / name)
        for name in keep + stale:
            self.put(self.v3 / name)
        for path in (outside / 'data', self.web / 'custom.html', self.web / 'v2/previous'):
            self.put(path)
        (self.v3 / 'linked-dir').symlink_to(outside)
        (self.v3 / 'linked-file').symlink_to(outside / 'data')
        (self.v3 / 'dangling').symlink_to(outside / 'absent')
        for _ in range(2):
            self.cleanup()
            for name in keep:
                self.assertEqual((self.v3 / name).read_text(), 'keep')
            for name in stale:
                self.assertFalse((self.v3 / name).exists(), name)
            for name in ('linked-file', 'linked-dir', 'dangling'):
                self.assertTrue((self.v3 / name).is_symlink())
            for path in (outside / 'data', self.web / 'custom.html', self.web / 'v2/previous'):
                self.assertEqual(path.read_text(), 'keep')

    def test_without_incoming_dashboard(self):
        self.put(self.v3 / 'old.js')
        self.cleanup()
        self.assertTrue((self.v3 / 'old.js').exists())

    def test_build_tree_inside_installed_directory(self):
        self.incoming = self.v3 / 'build/dashboard-src/dist/agent/v3'
        self.put(self.incoming / 'index.html')
        self.put(self.v3 / 'old.js')
        self.cleanup()
        self.assertTrue((self.incoming / 'index.html').exists())
        self.assertTrue((self.v3 / 'old.js').exists())

    def test_directory_links_in_install_path(self):
        self.put(self.incoming / 'index.html')
        for component in ('v3', 'web', 'netdata', 'share', 'usr'):
            relative = self.v3.relative_to(self.prefix)
            link = self.prefix.joinpath(*relative.parts[:relative.parts.index(component) + 1])
            with self.subTest(component=component):
                self.put(self.v3 / 'old.js')
                target = self.root / ('relocated-' + component)
                link.rename(target)
                link.symlink_to(target)
                self.cleanup()
                self.assertTrue((self.v3 / 'old.js').is_file())
                link.unlink()
                target.rename(link)

    @unittest.skipUnless(shutil.which('cmake'), 'CMake required')
    def test_reconfigure_and_install_two_bundles(self):
        project = self.root / 'CMakeLists.txt'
        system_files = (MODULES / 'NetdataSystemFiles.cmake').read_text()
        begin = system_files.index('set(dashboard_web_dest "")')
        end = system_files.index('\n#\n', begin)
        project.write_text(f'''cmake_minimum_required(VERSION 3.16)
project(dashboard NONE)
list(APPEND CMAKE_MODULE_PATH "{MODULES}")
set(WEB_DEST "custom web")
option(ENABLE_DASHBOARD "Include the dashboard" ON)
function(netdata_add_deb_copyright)
endfunction()
''' + system_files[begin:end])
        installer = (ROOT / 'netdata-installer.sh').read_text()
        begin = installer.index('if ! cmake_install "${NETDATA_BUILD_DIR}"')
        end = installer.index('\n# ---', begin)
        script = ('set -eu\nrun() { "$@"; }\nfatal() { exit 1; }\n'
                  'cmake_install() { cmake --install "$1" --component dashboard; }\n'
                  + installer[begin:end])
        build = self.root / 'build'
        archive = self.root / 'bundle.tar.gz'
        env = dict(os.environ, NETDATA_PREFIX=str(self.prefix), NETDATA_BUILD_DIR=str(build),
                   NETDATA_SOURCE_DIR=str(ROOT), DESTDIR='', NETDATA_WEB_DIR=str(self.root / 'user-web'))
        self.put(self.root / 'user-web/v3/keep.js')
        installed = self.prefix / 'custom web/v3'
        for version, asset in [('v2', 'retired/old.gif'), ('v3', 'nested/current.js')]:
            with tarfile.open(archive, 'w:gz') as tar:
                for name in ('index.html', asset, '.well-known/id'):
                    entry = tarfile.TarInfo(f'dist/agent/{version}/{name}')
                    entry.size = 2
                    tar.addfile(entry, io.BytesIO(b'ok'))
            subprocess.run(['cmake', '-S', str(self.root), '-B', str(build),
                            '-DCMAKE_INSTALL_PREFIX=' + str(self.prefix),
                            '-DDASHBOARD_URL=' + archive.as_uri()], check=True, capture_output=True)
            subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
            self.assertEqual({str(p.relative_to(installed)) for p in installed.rglob('*') if p.is_file()},
                             {'index.html', asset, '.well-known/id'})
        subprocess.run(['cmake', '-S', str(self.root), '-B', str(build), '-DENABLE_DASHBOARD=OFF'],
                       check=True, capture_output=True)
        self.put(installed / 'local.css')
        subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
        self.assertTrue((installed / 'local.css').is_file())
        self.assertTrue((self.root / 'user-web/v3/keep.js').is_file())


if __name__ == '__main__':
    unittest.main()
