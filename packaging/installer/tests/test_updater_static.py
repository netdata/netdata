# SPDX-License-Identifier: GPL-3.0-or-later
"""update_static() exit status in netdata-updater.sh, using a stub static installer."""
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'netdata-updater.sh'


def extract_function(name):
    match = re.search(r'^%s\(\) \{\n.*?^\}\n' % name, SCRIPT.read_text(), re.S | re.M)
    assert match, name
    return match.group(0)


class UpdateStaticTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.prefix = self.root / 'opt' / 'netdata'
        (self.prefix / 'etc' / 'netdata').mkdir(parents=True)

    def run_update_static(self, installer_exit):
        func = extract_function('update_static').replace('/opt/netdata', str(self.prefix))
        body = '''
set -u
exec 3>&2
info() { echo "INFO: $1" >&2; }
fatal() { echo "FATAL: $1" >&2; exit 1; }
update_available() { return 0; }
printf 'exit %(rc)d\\n' > "%(root)s/stub.run"
download() {
  case "$2" in
    *sha256sum.txt) printf '%%s  netdata-x86_64-latest.gz.run\\n' "$(sha256sum "%(root)s/stub.run" | cut -d' ' -f1)" > "$2" ;;
    *) cp "%(root)s/stub.run" "$2" ;;
  esac
}
safe_sha256sum() { sha256sum "$@"; }
create_exec_tmp_directory() { ndtmpdir="$(mktemp -d "%(root)s/tmp.XXXXXX")"; }
NETDATA_TARBALL_CHECKSUM_URL=sum NETDATA_TARBALL_URL=run PREBUILT_ARCH=x86_64
REINSTALL_OPTIONS= logfile=
%(func)s
update_static
''' % {'rc': installer_exit, 'root': self.root, 'func': func}
        return subprocess.run(['sh', '-c', body], text=True, capture_output=True, timeout=30,
                              env=dict(os.environ, TMPDIR=str(self.root)), cwd=str(self.root))

    def test_failed_installer_fails_update(self):
        result = self.run_update_static(7)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('failed with exit status 7', result.stderr)
        self.assertFalse((self.prefix / 'etc' / 'netdata' / '.install-type').exists())

    def test_successful_installer_succeeds(self):
        result = self.run_update_static(0)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((self.prefix / 'etc' / 'netdata' / '.install-type').exists())


if __name__ == '__main__':
    unittest.main()
