# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise actual kickstart functions with isolated Agent/privilege/PM callouts."""
import json
import os
import pwd
import sys
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

KICKSTART = Path(__file__).resolve().parents[1] / 'kickstart.sh'
FUNCTIONS = (
    'main', 'handle_existing_install', 'install_dem_runtime', 'setup_dem_runtime',
    'dem_runtime_config', 'dem_runtime_plugin', 'dem_runtime_cli',
    'configure_native_package_manager', 'common_rpm_opts', 'common_dnf_opts',
    'run', 'run_as_root', '_get_run_info', '_log_run_cmd', '_log_run_result',
    'escaped_print', 'parse_args', 'validate_args',
)

STUB = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
root = pathlib.Path(os.environ['FIXTURE'])
name, args = pathlib.Path(sys.argv[0]).name, sys.argv[1:]
settings = json.loads((root / 'settings.json').read_text())
with (root / 'calls.jsonl').open('a') as log:
    log.write(json.dumps({'name': name, 'args': args,
                         'uid': os.environ.get('FIXTURE_UID', '0'),
                         'lib': os.environ.get('NETDATA_LIB_DIR'),
                         'config': os.environ.get('NETDATA_USER_CONFIG_DIR'),
                         'stock': os.environ.get('NETDATA_STOCK_CONFIG_DIR')}) + '\n')
if name == 'id':
    print(settings.get('service_uid', '999') if len(args) > 1 else os.environ.get('FIXTURE_UID', '0'))
elif name == 'netdata':
    if settings.get('query_exit'): sys.exit(settings['query_exit'])
    assert args[:2] == ['-W', 'get'], args
    print(settings['config'][args[2] + '/' + args[3]])
elif name == 'runuser':
    assert args[:2] == ['-u', settings['config']['global/run as user']]
    assert args[2] == '--'
    os.environ['FIXTURE_UID'] = settings.get('service_uid', '999')
    os.execvpe(args[3], args[3:], os.environ)
elif name == 'dem.plugin':
    assert os.environ.get('FIXTURE_UID') == settings.get('service_uid', '999') != '0'
    if settings.get('cli_exit'):
        print('fixture: missing publisher trust or failed qualification', file=sys.stderr)
        sys.exit(settings['cli_exit'])
    print(json.dumps({'mode': settings.get('mode', 'native'), 'state': 'installed'}, indent=2))
elif name in ('apt-get', 'dnf', 'yum', 'zypper'):
    sys.exit(settings.get('pm_exit', 0))
elif name != 'event':
    raise SystemExit('unexpected fixture call')
'''


class DemRuntimeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="dem kickstart 'test ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.plugins = self.root / 'plugins $(not-executed)'
        self.plugins.mkdir()
        for name in ('id', 'netdata', 'runuser', 'apt-get', 'dnf', 'event'):
            self.executable(self.bin / name)
        self.executable(self.plugins / 'dem.plugin')
        self.settings = {
            'config': {'global/run as user': 'monitor-account',
                       'directories/config': str(self.root / 'custom config'),
                       'directories/stock config': str(self.root / 'stock config'),
                       'directories/lib': str(self.root / 'state ; literal'),
                       'directories/plugins': '"' + str(self.plugins) + '"'},
        }
        source = KICKSTART.read_text()
        extracted = []
        for name in FUNCTIONS:
            matches = re.findall(r'^' + name + r'\(\) \{\n.*?^\}\n', source, re.M | re.S)
            self.assertEqual(len(matches), 1, name)
            extracted.append(matches[0])
        self.script = self.root / 'functions.sh'
        self.script.write_text('\n'.join(extracted) + r'''
progress() { printf '%s\n' "$*" >&2; }
warning() { printf '%s\n' "$*" >&2; }
fatal() { printf '%s\n' "$1" >&2; exit 1; }
confirm_root_support() { :; }
cleanup() { :; }
set_tmpdir() { :; }
deferred_warnings() { :; }
success_banner() { :; }
telemetry_event() { :; }
set_auto_updates() { :; }
check_claim_opts() { :; }
handle_major_version() { :; }
detect_existing_install() {
  ndpath="$FIXTURE/bin/netdata"
  INSTALL_TYPE="$FIXTURE_TYPE"
  if [ "$FIXTURE_INSTALLED" = 1 ]; then ndprefix="$FIXTURE"; else ndprefix=; fi
}
install_on_linux() { FIXTURE_INSTALLED=1; INSTALL_TYPE="$FIXTURE_TYPE"; event agent-install; }
update() { event agent-update; }
claim() { return "${FIXTURE_CLAIM_STATUS:-0}"; }
''')

    def executable(self, path):
        path.write_text(STUB)
        path.chmod(0o755)

    def calls(self, name=None):
        path = self.root / 'calls.jsonl'
        calls = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        return [call for call in calls if name is None or call['name'] == name]

    def invoke(self, body='install_dem_runtime', **changes):
        (self.root / 'settings.json').write_text(json.dumps(self.settings))
        env = dict(os.environ, FIXTURE=str(self.root), USER='fixture',
                   PATH=str(self.bin) + os.pathsep + os.environ['PATH'], ROOTCMD='',
                   NETDATA_INSTALL_DEM_RUNTIME='1', DRY_RUN='0', SYSTYPE='Linux', SYSARCH='x86_64',
                   FIXTURE_INSTALLED='1', FIXTURE_TYPE='kickstart-build', INSTALL_TYPE='kickstart-build',
                   INTERACTIVE='0', DISTRO_COMPAT_NAME='debian', DISTRO='debian', SYSCODENAME='trixie',
                   SYSVERSION='13', NETDATA_REQUESTED_INSTALL_TYPE='auto', NETDATA_AUTO_UPDATES='default',
                   DEFAULT_RELEASE_CHANNEL='nightly', RELEASE_CHANNEL='default', ACTION='',
                   NETDATA_OFFLINE_INSTALL_SOURCE='', NETDATA_CLAIM_TOKEN='', run_logfile='/dev/null')
        env.update({key: str(value) for key, value in changes.items()})
        return subprocess.run(['sh', '-c', '. "$1"; ' + body, 'fixture', str(self.script)],
                              env=env, cwd=self.root, capture_output=True, text=True)

    def test_option_is_explicit_and_parsed(self):
        result = self.invoke(NETDATA_INSTALL_DEM_RUNTIME=0)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [])
        for installed in (0, 1):
            result = self.invoke('main', NETDATA_INSTALL_DEM_RUNTIME=0, FIXTURE_INSTALLED=installed)
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls('dem.plugin'), [])
        self.assertEqual(self.calls('apt-get'), [])
        result = self.invoke('parse_args --install-dem-runtime; printf "%s" "$NETDATA_INSTALL_DEM_RUNTIME"',
                             NETDATA_INSTALL_DEM_RUNTIME=0)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, '1')

    def test_fresh_and_existing_managed_install_use_service_identity_and_paths(self):
        for installed in (0, 1):
            with self.subTest(installed=installed):
                result = self.invoke('main', FIXTURE_INSTALLED=installed)
                self.assertEqual(result.returncode, 0, result.stderr)
                call = self.calls('dem.plugin')[-1]
                self.assertEqual(call['args'], ['runtime', 'install', '--allow-download', '--config-dir',
                                                self.settings['config']['directories/config']])
                self.assertEqual(call['uid'], '999')
                for field, key in [('lib', 'lib'), ('config', 'config'), ('stock', 'stock config')]:
                    self.assertEqual(call[field], self.settings['config']['directories/' + key])
                self.assertIn('Restart the Agent explicitly', result.stderr)
        self.assertEqual([call['args'] for call in self.calls('event')], [['agent-install'], ['agent-update']])

    def test_existing_claim_failure_is_not_hidden_by_runtime_success(self):
        result = self.invoke('handle_existing_install', NETDATA_CLAIM_TOKEN='fixture', FIXTURE_CLAIM_STATUS=19)
        self.assertEqual(result.returncode, 19, result.stderr)
        self.assertEqual(len(self.calls('dem.plugin')), 1)

    def test_static_install_as_existing_service_user_needs_no_privilege_switch(self):
        result = self.invoke(FIXTURE_TYPE='manual-static', SYSARCH='aarch64', FIXTURE_UID='999')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls('runuser'), [])
        self.assertEqual(self.calls('dem.plugin')[0]['args'][1:3], ['install', '--allow-download'])

    def test_plugin_search_keeps_quoted_paths_as_data_and_rejects_escaping(self):
        self.settings['config']['directories/plugins'] = '\t"/missing path"  "' + str(self.plugins) + '"'
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.calls('dem.plugin')), 1)
        self.settings['config']['directories/plugins'] = '"/escaped\\path"'
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.calls('dem.plugin')), 1)

    def test_native_deb_and_rpm_keep_package_policy_and_verify_readiness(self):
        self.settings['config']['global/run as user'] = 'netdata'
        for distro, kind, manager in [('debian', 'binpkg-deb', 'apt-get'), ('centos', 'binpkg-rpm', 'dnf')]:
            with self.subTest(distro=distro):
                result = self.invoke(FIXTURE_TYPE=kind, INSTALL_TYPE=kind, DISTRO_COMPAT_NAME=distro,
                                     INSTALL_VERSION='99.0.0')
                self.assertEqual(result.returncode, 0, result.stderr)
                args = self.calls(manager)[-1]['args']
                self.assertEqual(args[-1], 'netdata-dem-runtime')
                self.assertIn('-y', args)
                self.assertNotIn('99.0.0', ' '.join(args))
                self.assertEqual(self.calls('dem.plugin')[-1]['args'][1], 'status')
        self.assertIn('Dpkg::Options::=--force-confold', self.calls('apt-get')[0]['args'])

    def test_native_interactive_policy_does_not_add_yes(self):
        self.settings['config']['global/run as user'] = 'netdata'
        result = self.invoke(FIXTURE_TYPE='binpkg-deb', INTERACTIVE=1)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls('apt-get')[0]['args'], ['install', 'netdata-dem-runtime'])

    def test_native_failure_never_falls_back_and_preserves_status(self):
        self.settings['config']['global/run as user'] = 'netdata'
        self.settings['pm_exit'] = 23
        result = self.invoke(FIXTURE_TYPE='binpkg-deb')
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(self.calls('dem.plugin'), [])
        self.settings.update(pm_exit=0, cli_exit=17)
        result = self.invoke(FIXTURE_TYPE='binpkg-rpm', DISTRO_COMPAT_NAME='centos')
        self.assertEqual(result.returncode, 17, result.stderr)
        self.assertIn('no managed fallback', result.stderr)
        self.assertEqual(self.calls('dem.plugin')[-1]['args'][1], 'status')

    def test_native_rejects_successful_status_for_managed_mode(self):
        self.settings['config']['global/run as user'] = 'netdata'
        self.settings['mode'] = 'managed'
        result = self.invoke(FIXTURE_TYPE='binpkg-deb')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('does not report native mode', result.stderr)

    def test_native_requires_named_netdata_account_before_acquisition(self):
        result = self.invoke(FIXTURE_TYPE='binpkg-deb')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('named netdata service account', result.stderr)
        self.assertEqual(self.calls('apt-get'), [])
        self.assertEqual(self.calls('dem.plugin'), [])

    def test_managed_failure_preserves_status_and_does_not_claim_success(self):
        self.settings['cli_exit'] = 29
        result = self.invoke('main', FIXTURE_INSTALLED=0)
        self.assertEqual(result.returncode, 29, result.stderr)
        self.assertIn('publisher release_url/verification_keys', result.stderr)
        self.assertNotIn('Successfully installed', result.stderr)

    def test_root_account_missing_plugin_and_bad_config_refuse(self):
        self.settings['service_uid'] = '0'
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls('dem.plugin'), [])
        self.settings['service_uid'] = '999'
        self.settings['config']['directories/plugins'] = '"/missing/plugin path"'
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Install an Agent with DEM support', result.stderr)
        self.settings['query_exit'] = 31
        result = self.invoke()
        self.assertEqual(result.returncode, 31, result.stderr)

    def test_unsupported_platform_and_conflicting_actions_do_not_acquire(self):
        for values in ({'SYSARCH': 'armv7l'}, {'SYSTYPE': 'Darwin'}):
            result = self.invoke(**values)
            self.assertNotEqual(result.returncode, 0)
        for action in ('uninstall', 'claim', 'repositories-only', 'prepare-offline'):
            result = self.invoke('validate_args', ACTION=action)
            self.assertNotEqual(result.returncode, 0)
        result = self.invoke('validate_args', NETDATA_OFFLINE_INSTALL_SOURCE=str(self.root))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), [])

    def test_dry_run_never_queries_executes_or_downloads(self):
        for kind in ('kickstart-static', 'binpkg-deb'):
            result = self.invoke(DRY_RUN=1, INSTALL_TYPE=kind, FIXTURE_INSTALLED=0)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('Would run', result.stderr)
        self.assertEqual(self.calls('dem.plugin'), [])
        self.assertEqual(self.calls('netdata'), [])
        self.assertEqual(self.calls('apt-get'), [])


@unittest.skipUnless(sys.platform == 'linux' and os.geteuid() == 0 and
                     os.environ.get('NETDATA_TEST_PRIVILEGED_CONTAINER') == '1',
                     'requires an explicitly opted-in disposable root Linux container')
class RestrictedPluginTests(unittest.TestCase):
    def test_unrelated_invoker_finds_restricted_plugin_and_executes_as_service(self):
        # This setuid fixture exists only in the disposable container, never on
        # the test host. It stands in for the installer's existing sudo command.
        service = pwd.getpwnam('netdata')
        invoker = pwd.getpwnam('nobody')
        with tempfile.TemporaryDirectory(prefix='dem-permission-test-') as directory:
            root = Path(directory)
            root.chmod(0o755)
            plugins = root / 'restricted plugins'
            plugins.mkdir()
            os.chown(plugins, 0, service.pw_gid)
            plugins.chmod(0o750)
            plugin = plugins / 'dem.plugin'
            plugin.write_text('#!/bin/sh\nprintf "service_uid=%s\\n" "$(id -u)"\n')
            os.chown(plugin, 0, service.pw_gid)
            plugin.chmod(0o750)
            wrapper_source = root / 'rootwrap.c'
            wrapper_source.write_text('#include <unistd.h>\n'
                                      'int main(int argc,char **argv) { '
                                      'if(argc<2||setuid(0)) return 127; '
                                      'execvp(argv[1],argv+1); return 126; }\n')
            wrapper = root / 'rootwrap'
            subprocess.run(['cc', str(wrapper_source), '-o', str(wrapper)], check=True)
            wrapper.chmod(0o4755)
            source = KICKSTART.read_text()
            functions = []
            for name in ('dem_runtime_plugin', 'dem_runtime_cli', 'run_as_root'):
                matches = re.findall(r'^' + name + r'\(\) \{\n.*?^\}\n', source, re.M | re.S)
                self.assertEqual(len(matches), 1, name)
                functions.append(matches[0])
            script = root / 'lookup.sh'
            script.write_text('\n'.join(functions) + r'''
run() { "$@"; }
confirm_root_support() { :; }
warning() { printf '%s\n' "$*" >&2; }
ROOTCMD=$1
dem_user=netdata
dem_uid=$2
dem_lib=/tmp
dem_config=/tmp
dem_stock_config=/tmp
dem_runtime_plugin "\"$3\"" || exit 41
dem_runtime_cli status
''')
            # Prove real filesystem denial for an identity outside service groups.
            credentials = dict(user=invoker.pw_uid, group=invoker.pw_gid, extra_groups=[])
            denied = subprocess.run(['/usr/bin/test', '-x', str(plugin)], **credentials)
            self.assertEqual(denied.returncode, 1)
            env = dict(os.environ, PATH='/usr/sbin:/usr/bin:/sbin:/bin')
            result = subprocess.run(['sh', str(script), str(wrapper), str(service.pw_uid), str(plugins)],
                                    env=env, capture_output=True, text=True, **credentials)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.strip(), f'service_uid={service.pw_uid}')


if __name__ == '__main__':
    unittest.main()
