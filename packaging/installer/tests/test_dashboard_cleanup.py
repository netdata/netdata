# SPDX-License-Identifier: GPL-3.0-or-later
import io
import os
from pathlib import Path
import shutil
import stat
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

    @unittest.skipUnless(shutil.which('unshare') and os.geteuid() == 0, 'Linux mount namespace required')
    def test_mounted_web_root_directory_and_file(self):
        namespace = ['unshare', '--mount', '--propagation', 'private']
        if subprocess.run([*namespace, 'true'], capture_output=True).returncode:
            self.skipTest('cannot create a private mount namespace')
        self.put(self.incoming / 'index.html')
        self.put(self.v3 / 'old.js')
        self.put(self.v3 / 'mounted file')
        (self.v3 / 'mounted dir').mkdir()
        outside = self.root / 'outside'
        self.put(outside / 'v3/keep')
        script = '''set -eu
mount --bind "$4" "$3"
sh "$1" "$2" "$5" usr/share/netdata/web
test -f "$4/v3/keep"
umount "$3"
mount --bind "$4/v3" "$3/v3/mounted dir"
mount --bind "$4/v3/keep" "$3/v3/mounted file"
sh "$1" "$2" "$5" usr/share/netdata/web
test -f "$3/v3/mounted dir/keep"
test -f "$3/v3/mounted file"
test ! -e "$3/v3/old.js"
'''
        subprocess.run([*namespace, 'sh', '-c', script, 'test', str(CLEANUP), str(self.incoming),
                        str(self.web), str(outside), str(self.prefix)], check=True)

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
    def test_cmake_symlink_install_retains_its_targets(self):
        version = subprocess.check_output(['cmake', '--version'], text=True).split()[2]
        if tuple(map(int, version.split('.')[:2])) < (3, 22):
            self.skipTest('CMAKE_INSTALL_MODE requires CMake 3.22')
        (self.root / 'CMakeLists.txt').write_text(f'''cmake_minimum_required(VERSION 3.16)
project(dashboard NONE)
list(APPEND CMAKE_MODULE_PATH "{MODULES}")
include(NetdataDashboard)
set(WEB_DEST "web")
bundle_dashboard()
''')
        archive = self.root / 'bundle.tar.gz'
        build = self.root / 'build'
        for release in (b'first', b'second'):
            with tarfile.open(archive, 'w:gz') as tar:
                entry = tarfile.TarInfo('dist/agent/v3/index.html')
                entry.size = len(release)
                tar.addfile(entry, io.BytesIO(release))
            subprocess.run(['cmake', '-S', str(self.root), '-B', str(build),
                            '-DDASHBOARD_URL=' + archive.as_uri()], check=True, capture_output=True)
            for mode in ('REL_SYMLINK', 'ABS_SYMLINK'):
                prefix = self.prefix / mode
                for _ in range(2):
                    subprocess.run(['cmake', '--install', str(build), '--prefix', str(prefix)],
                                   env=dict(os.environ, CMAKE_INSTALL_MODE=mode),
                                   check=True, capture_output=True)
                    index = prefix / 'web/v3/index.html'
                    self.assertTrue(index.is_symlink())
                    self.assertEqual(index.readlink().is_absolute(), mode == 'ABS_SYMLINK')
                    self.assertEqual(index.read_bytes(), release)

    @unittest.skipUnless(shutil.which('cmake'), 'CMake required')
    def test_reconfigure_and_install_two_bundles(self):
        project = self.root / 'CMakeLists.txt'
        system_files = (MODULES / 'NetdataSystemFiles.cmake').read_text()
        begin = system_files.index('if(ENABLE_DASHBOARD)')
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
        begin = installer.index('dashboard_upgrade=0')
        end = installer.index('\n# ---', begin)
        script = ('set -eu\nrun() { "$@"; }\nfatal() { exit 1; }\n'
                  'cmake_install() { cmake --install "$1" --component dashboard; }\n'
                  + installer[begin:end])
        # Run the source installer's actual completion path, with service and UI calls isolated.
        script += '''
DP_ENABLED=1; DP_WAS_RUNNING=1; DP_DONT_START=0; started=0
dem_pair_finish() { echo finish; return "${ACTIVATION_STATUS:-0}"; }
dem_pair_cleanup_stage() { :; }
print_deferred_errors() { :; }
progress() { :; }
netdata_banner() { :; }
''' + installer[installer.rindex('if [ "${DP_ENABLED:-0}" = 1 ]; then'):]
        build = self.root / 'build'
        archive = self.root / 'bundle.tar.gz'
        env = dict(os.environ, NETDATA_PREFIX=str(self.prefix), NETDATA_BUILD_DIR=str(build),
                   NETDATA_SOURCE_DIR=str(ROOT), DESTDIR='', NETDATA_WEB_DIR=str(self.root / 'user-web'))
        self.put(self.root / 'user-web/v3/keep.js')
        installed = self.prefix / 'custom web/v3'
        self.put(installed / 'before-first-install.css')
        for version, asset in [('v2', 'old/chunk.js'), ('v3', 'new/chunk.js')]:
            if version == 'v3':
                agent = self.prefix / 'usr/sbin/netdata'
                self.put(agent)
                agent.chmod(0o755)
            with tarfile.open(archive, 'w:gz') as tar:
                for name, content in [('index.html', b'dashboard'), ('manifest.txt', asset.encode()),
                                      (asset, b'chunk'), ('.well-known/id', b'id')]:
                    entry = tarfile.TarInfo(f'dist/agent/{version}/{name}')
                    entry.mtime = 1700000000
                    entry.size = len(content)
                    tar.addfile(entry, io.BytesIO(content))
                entry = tarfile.TarInfo(f'dist/agent/{version}/linked-assets')
                entry.type = tarfile.SYMTYPE
                entry.linkname = asset.split('/')[0]
                tar.addfile(entry)
            subprocess.run(['cmake', '-S', str(self.root), '-B', str(build),
                            '-DCMAKE_INSTALL_PREFIX=' + str(self.prefix),
                            '-DDASHBOARD_URL=' + archive.as_uri()], check=True, capture_output=True)
            subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
            self.assertEqual((installed / 'manifest.txt').read_text(), asset)
            self.assertEqual((installed / 'linked-assets/chunk.js').read_bytes(), b'chunk')
            self.assertEqual(os.readlink(installed / 'linked-assets'), asset.split('/')[0])
            expected = {'index.html', 'manifest.txt', asset, '.well-known/id'}
            if version == 'v2':
                expected.add('before-first-install.css')
            self.assertEqual({str(p.relative_to(installed)) for p in installed.rglob('*') if p.is_file()},
                             expected)
        # Fail only the unlink of a stale asset in the real cleanup helper.
        rm = self.root / 'commands/rm'
        self.put(rm, '#!/bin/sh\nif [ "$2" = ./obsolete.js ]; then exit 37; fi\nexec /bin/rm "$@"\n')
        rm.chmod(0o755)
        self.put(installed / 'obsolete.js')
        for activation in (0, 43):
            with self.subTest(activation=activation):
                result = subprocess.run(['sh', '-c', script], text=True, capture_output=True,
                                        env=dict(env, PATH=str(rm.parent) + ':' + env['PATH'],
                                                 ACTIVATION_STATUS=str(activation)))
                self.assertIn('finish', result.stdout.splitlines(), result.stderr)
                self.assertIn('Dashboard cleanup failed: 37', result.stderr)
                self.assertEqual(result.returncode, activation, result.stderr)
                self.assertTrue((installed / 'obsolete.js').is_file())
        subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
        self.assertFalse((installed / 'obsolete.js').exists())
        # A source upgrade still installs packaged files through an administrator's
        # directory link. The target keeps its permissions and skips stale-file cleanup.
        relocated = self.root / 'relocated dashboard'
        installed.rename(relocated)
        installed.symlink_to(relocated, target_is_directory=True)
        relocated.chmod(0o750)
        asset_dir = relocated / 'new'
        directory_mode = 0o710
        group = None
        if os.uname().sysname == 'Linux':
            # A setgid web directory assigns its group to newly installed assets.
            group = next((g for g in os.getgroups() if g != build.stat().st_gid), None)
            if group is None and os.geteuid() == 0:
                group = build.stat().st_gid + 1
            if group is not None:
                os.chown(asset_dir, -1, group)
                directory_mode |= stat.S_ISGID
        asset_dir.chmod(directory_mode)
        (asset_dir / 'chunk.js').unlink()
        outside = self.root / 'operator file'
        self.put(outside, 'untouched')
        (relocated / 'index.html').unlink()
        (relocated / 'index.html').symlink_to(outside)
        (relocated / 'manifest.txt').unlink()
        os.link(outside, relocated / 'manifest.txt')
        self.put(relocated / 'local.css')
        subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
        self.assertTrue(installed.is_symlink())
        self.assertEqual(stat.S_IMODE(relocated.stat().st_mode), 0o750)
        self.assertEqual(stat.S_IMODE(asset_dir.stat().st_mode), directory_mode)
        if group is not None:
            self.assertEqual((asset_dir / 'chunk.js').stat().st_gid, group)
        self.assertEqual(outside.read_text(), 'untouched')
        self.assertEqual((relocated / 'index.html').read_bytes(), b'dashboard')
        self.assertEqual((relocated / 'manifest.txt').read_text(), 'new/chunk.js')
        self.assertTrue((relocated / 'local.css').is_file())
        # A payload file cannot replace an existing directory or chmod its target.
        index = relocated / 'index.html'
        index.unlink()
        index.symlink_to(relocated / 'new', target_is_directory=True)
        result = subprocess.run(['sh', '-c', script], env=env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Cannot install dashboard file over directory:', result.stderr)
        self.assertEqual(stat.S_IMODE(asset_dir.stat().st_mode), directory_mode)
        self.assertEqual((relocated / 'new/chunk.js').read_bytes(), b'chunk')
        installed.unlink()
        relocated.rename(installed)
        subprocess.run(['cmake', '-S', str(self.root), '-B', str(build), '-DENABLE_DASHBOARD=OFF'],
                       check=True, capture_output=True)
        self.put(installed / 'local.css')
        subprocess.run(['sh', '-c', script], check=True, env=env, capture_output=True)
        self.assertTrue((installed / 'local.css').is_file())
        self.assertTrue((self.root / 'user-web/v3/keep.js').is_file())


if __name__ == '__main__':
    unittest.main()
