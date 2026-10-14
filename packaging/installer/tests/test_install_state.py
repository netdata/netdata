#!/usr/bin/env python3
"""Exercise real installer directory setup and makeself extraction in isolated trees."""
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
INSTALLER = ROOT / 'packaging/makeself/install-or-update.sh'
FUNCTIONS = ROOT / 'packaging/installer/functions.sh'
INVENTORY = ROOT / 'packaging/makeself/create-package-manifest.sh'


@unittest.skipUnless(sys.platform == 'linux', 'Linux installer permission semantics')
class InstallStateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-install-state-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.prefix = self.base / 'opt/netdata'
        self.prefix.mkdir(parents=True)
        # Root runs establish preservation of owners different from the service account.
        self.service_uid = 65534 if os.geteuid() == 0 else os.geteuid()
        self.service_gid = 65534 if os.geteuid() == 0 else os.getegid()
        self.environment = dict(os.environ, NETDATA_USER=str(self.service_uid),
                                NETDATA_GROUP=str(self.service_gid), TEST_PARENT=str(self.base / 'opt'))
        source = INSTALLER.read_text()
        settings = source.split('NETDATA_PREFIX="/opt/netdata"', 1)[1].split('# ---', 1)[0]
        begin = source.index('progress "fix permissions"')
        end = source.index('# The Go helper replaces', begin)
        self.script = self.base / 'setup.sh'
        self.script.write_text('''#!/usr/bin/env bash
. "$2"
NETDATA_PREFIX="$1"
''' + settings + '''
# Keep account/service installation outside this directory-setup fixture.
install_netdata_tmpfiles() { return 1; }
progress() { :; }
warning() { printf '%s\\n' "$*" >&2; }
run() {
  local argument
  local -a arguments=()
  for argument in "$@"; do
    case "$argument" in
      /opt) argument="$TEST_PARENT" ;;
      /opt/netdata*) argument="$NETDATA_PREFIX${argument#/opt/netdata}" ;;
    esac
    arguments+=("$argument")
  done
  "${arguments[@]}"
}
''' + source[begin:end].replace('/opt/netdata', '${NETDATA_PREFIX}'))
        self.script.chmod(0o755)
        # Inventory is produced before any application state exists.
        package = self.base / 'package'
        for root in ('system', 'usr/libexec/netdata/plugins.d', 'var/lib/netdata',
                     'var/cache/netdata', 'var/log/netdata'):
            (package / root).mkdir(parents=True)
        self.generate_inventory(package)
        (self.prefix / 'system').mkdir()
        shutil.copyfile(package / 'system/package-manifest', self.prefix / 'system/package-manifest')

    def generate_inventory(self, tree):
        result = subprocess.run(['bash', str(INVENTORY), str(tree)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def run_setup(self):
        result = subprocess.run(['bash', str(self.script), str(self.prefix), str(FUNCTIONS)],
                                env=self.environment, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def state_fixture(self):
        paths = []
        for root in ('var/lib/netdata', 'var/cache/netdata', 'var/log/netdata',
                     'custom-runtime', 'usr/libexec/custom-runtime',
                     'usr/libexec/netdata/plugins.d/ebpf.d/custom-runtime'):
            private = self.prefix / root / 'application/private/completion'
            private.mkdir(parents=True)
            for directory in (private.parent.parent, private.parent, private):
                directory.chmod(0o700)
                paths.append(directory)
            receipt = private / 'receipt'
            receipt.write_text('existing completion proof\n')
            receipt.chmod(0o600)
            paths.append(receipt)
            executable = private.parent / 'executable'
            executable.write_text('immutable executable fixture\n')
            executable.chmod(0o500)
            paths.append(executable)
            if os.geteuid() == 0 and '/ebpf.d/' in root:
                for entry in (private.parent.parent, private.parent, private, receipt, executable):
                    os.chown(entry, self.service_uid, self.service_gid)
        return paths

    @staticmethod
    def snapshot(paths):
        return {str(p): (stat.S_IMODE(p.stat().st_mode), p.stat().st_uid, p.stat().st_gid,
                         p.read_bytes() if p.is_file() else None) for p in paths}

    def test_update_preserves_private_state_and_package_access(self):
        paths = self.state_fixture()
        before = self.snapshot(paths)
        package_dir = self.prefix / 'usr/libexec/netdata/plugins.d'
        package_dir.mkdir(parents=True, exist_ok=True)
        package_dir.chmod(0o700)
        self.run_setup()
        self.assertEqual(self.snapshot(paths), before)
        self.assertEqual(stat.S_IMODE(package_dir.stat().st_mode) & 0o055, 0o055)

    @unittest.skipUnless(os.geteuid() == 0, 'root is required for package ownership')
    def test_ebpf_ownership_applies_only_to_shipped_entries(self):
        relative = Path('usr/libexec/netdata/plugins.d/ebpf.d')
        package = self.base / 'package'
        (package / relative).mkdir()
        (package / relative / 'program.o').write_text('shipped object')
        link_target = self.prefix / 'custom-runtime/application/private/completion'
        os.symlink(link_target, package / relative / 'private-link')
        self.generate_inventory(package)
        shutil.copyfile(package / 'system/package-manifest', self.prefix / 'system/package-manifest')
        paths = self.state_fixture()
        before = self.snapshot(paths)
        shipped = self.prefix / relative / 'program.o'
        shutil.copyfile(package / relative / 'program.o', shipped)
        os.chown(shipped, self.service_uid, self.service_gid)
        link = self.prefix / relative / 'private-link'
        os.symlink(link_target, link)
        os.lchown(link, self.service_uid, self.service_gid)
        self.run_setup()
        self.assertEqual(self.snapshot(paths), before)
        self.assertEqual((shipped.stat().st_uid, shipped.stat().st_gid), (0, self.service_gid))
        self.assertEqual((shipped.parent.stat().st_uid, shipped.parent.stat().st_gid), (0, self.service_gid))
        self.assertEqual((link.lstat().st_uid, link.lstat().st_gid), (0, self.service_gid))

    def test_missing_inventory_does_not_walk_existing_state(self):
        paths = self.state_fixture()
        before = self.snapshot(paths)
        (self.prefix / 'system/package-manifest').rename(self.base / 'saved-inventory')
        result = subprocess.run(['bash', str(self.script), str(self.prefix), str(FUNCTIONS)],
                                env=self.environment, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.snapshot(paths), before)

    def test_fallback_creates_usable_service_roots(self):
        self.run_setup()
        roots = ['var/lib/netdata', 'var/cache/netdata', 'var/log/netdata', 'var/lib/netdata/cloud.d']
        if os.geteuid() == 0:
            roots.append('var/log/netdata/traps')
        for path in roots:
            with self.subTest(path=path):
                self.assertTrue((self.prefix / path).is_dir(), path)
                info = (self.prefix / path).stat()
                self.assertEqual((info.st_uid, info.st_gid), (self.service_uid, self.service_gid))
                self.assertEqual(info.st_mode & 0o700, 0o700)
        claiming = self.prefix / 'var/lib/netdata/cloud.d'
        self.assertTrue(claiming.is_dir())
        self.assertEqual(stat.S_IMODE(claiming.stat().st_mode), 0o770)

    def test_shared_fallback_preserves_descendant_ownership(self):
        paths = self.state_fixture()
        child = self.prefix / 'var/lib/netdata/cloud.d/private/record'
        child.parent.mkdir(parents=True, mode=0o700)
        child.write_text('claiming fixture\n')
        child.chmod(0o600)
        paths += [child.parent, child]
        before = self.snapshot(paths)
        # Invoke the shared helper directly, also covering source-install callers.
        result = subprocess.run(['bash', '-c', '''
set -eu
. "$1"
run() { "$@"; }
install_netdata_tmpfiles() { return 1; }
NETDATA_LIB_DIR="$2/var/lib/netdata"
NETDATA_CACHE_DIR="$2/var/cache/netdata"
NETDATA_LOG_DIR="$2/var/log/netdata"
NETDATA_CLAIMING_DIR="$NETDATA_LIB_DIR/cloud.d"
install_netdata_dirs
''', 'test', str(FUNCTIONS), str(self.prefix)], env=self.environment,
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.snapshot(paths), before)

    @unittest.skipUnless(os.geteuid() == 0, 'root is required for historical ownership repair')
    def test_historical_otel_ownership_repair_keeps_private_modes(self):
        path = self.prefix / 'var/log/netdata/otel/private/state'
        path.parent.mkdir(parents=True, mode=0o700)
        path.write_text('historical builder-owned state\n')
        path.chmod(0o600)
        self.run_setup()
        self.assertEqual((path.stat().st_uid, path.stat().st_gid),
                         (self.service_uid, self.service_gid))
        self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)

    @unittest.skipUnless(os.geteuid() == 0 and shutil.which('systemd-tmpfiles'),
                         'root and systemd-tmpfiles are required')
    def test_tmpfiles_preserves_unrelated_state_and_keeps_explicit_policy(self):
        paths = self.state_fixture()
        before = self.snapshot(paths)
        config = (ROOT / 'system/systemd/tmpfiles.conf.in').read_text()
        replacements = {'@netdata_user_POST@': str(self.service_uid),
                        '@netdata_group_POST@': str(self.service_gid),
                        '@cachedir_POST@': str(self.prefix / 'var/cache/netdata'),
                        '@varlibdir_POST@': str(self.prefix / 'var/lib/netdata'),
                        '@logdir_POST@': str(self.prefix / 'var/log/netdata'),
                        '/run/netdata': str(self.prefix / 'run/netdata')}
        for token, value in replacements.items():
            config = config.replace(token, value)
        tmpfiles = self.base / 'tmpfiles.conf'
        tmpfiles.write_text(config)
        api_key = self.prefix / 'var/lib/netdata/netdata.api.key'
        api_key.write_text('synthetic api key\n')
        api_key.chmod(0o600)
        os.chown(api_key, self.service_uid, self.service_gid)
        script = self.script.read_text().replace('install_netdata_tmpfiles() { return 1; }',
            'install_netdata_tmpfiles() { return 0; }\nsystemd-tmpfiles() { command systemd-tmpfiles --create "$TEST_TMPFILES"; }')
        self.script.write_text(script)
        self.environment['TEST_TMPFILES'] = str(tmpfiles)
        self.run_setup()
        self.assertEqual(self.snapshot(paths), before)
        self.assertEqual(stat.S_IMODE(api_key.stat().st_mode), 0o660)
        self.assertEqual(api_key.stat().st_uid, self.service_uid)

    def test_actual_archive_overlay_preserves_state(self):
        paths = self.state_fixture()
        before = self.snapshot(paths)
        payload = self.base / 'payload'
        (payload / 'system').mkdir(parents=True)
        shutil.copyfile(self.script, payload / 'system/setup.sh')
        shutil.copyfile(FUNCTIONS, payload / 'system/functions.sh')
        for root in ('var/lib/netdata', 'var/cache/netdata', 'var/log/netdata'):
            (payload / root).mkdir(parents=True)
            (payload / root / '.keep').touch()
        (payload / 'usr/libexec/netdata').mkdir(parents=True)
        (payload / 'usr/libexec/netdata/component').write_text('package component\n')
        self.generate_inventory(payload)
        archive = self.base / 'installer.run'
        build = subprocess.run(['sh', str(ROOT / 'packaging/makeself/makeself.sh'),
                                '--gzip', '--notemp', '--target', str(self.prefix),
                                str(payload), str(archive), 'Installer state fixture',
                                'bash', './system/setup.sh', str(self.prefix), './system/functions.sh'],
                               capture_output=True, text=True)
        self.assertEqual(build.returncode, 0, build.stdout + build.stderr)
        update = subprocess.run(['sh', str(archive), '--quiet'], env=self.environment,
                                capture_output=True, text=True)
        self.assertEqual(update.returncode, 0, update.stdout + update.stderr)
        self.assertEqual(self.snapshot(paths), before)
        self.assertEqual(stat.S_IMODE((self.prefix / 'usr/libexec/netdata').stat().st_mode) & 0o055, 0o055)


if __name__ == '__main__':
    unittest.main()
