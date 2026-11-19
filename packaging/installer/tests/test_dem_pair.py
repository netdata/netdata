# SPDX-License-Identifier: GPL-3.0-or-later
"""Bounded owner fixtures. These do not certify real Agent/service transactions."""
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import unittest

HELPER = Path(__file__).resolve().parents[1] / "netdata-dem-pair.sh"
GNU_LINUX = os.uname().sysname == "Linux" and all(shutil.which(x) for x in ("tar", "flock", "sync"))
# BusyBox hosts such as Alpine have su but no runuser.
SWITCH_USER = bool(shutil.which('runuser') or shutil.which('su'))


class CommandTests(unittest.TestCase):
    def shell(self, body):
        return subprocess.run(["sh", "-c", '. ' + shlex.quote(str(HELPER)) + '\n' + body],
                              text=True, capture_output=True)

    def test_success_and_data_quoting(self):
        arguments = ['printf', '%s\\n', 'space ; $(false) `false`', 'a b', "it's 'quoted'"]
        result = self.shell('dem_pair_run ' + shlex.join(arguments))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "space ; $(false) `false`\na b\nit's 'quoted'\n")
        self.assertEqual(shlex.split(result.stderr.removeprefix('netdata-dem-pair: run')), arguments)

    def test_original_status_and_secret_redaction(self):
        result = self.shell(
            "dem_pair_run sh -c 'exit 37' 'https://user:synthetic-secret@example.invalid/path?token=secret'")
        self.assertEqual(result.returncode, 37)
        self.assertIn('status 37', result.stderr)
        self.assertIn('operation failed in ', result.stderr)
        self.assertNotIn('synthetic-secret', result.stderr)
        self.assertNotIn('token=secret', result.stderr)

    def test_no_enrollment_has_no_fetch(self):
        result = self.shell("dem_pair_plan /not/a/helper /not/an/agent /not/a/prefix source; [ \"$DP_ENABLED\" = 0 ]")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_fresh_install_does_not_query_target_or_require_service_account(self):
        result = self.shell('dem_pair_plan /missing/helper /missing/agent /missing/prefix source /bin/false; '
                            '[ "$DP_ENABLED" = 0 ]')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_target_agent_can_inspect_configuration_when_installed_agent_is_broken(self):
        with tempfile.TemporaryDirectory(prefix='netdata-retry-') as directory:
            root = Path(directory)
            (root / 'etc/netdata').mkdir(parents=True)
            target = root / 'target-agent'
            target.write_text('#!/bin/sh\nfor argument do :; done\n'
                              'if [ "$argument" = netdata ]; then id -un; else printf "%s\\n" "$argument"; fi\n')
            target.chmod(0o755)
            body = ('dem_pair_runtime() { printf "%s\\n" '
                    'netdata-dem-pair-plan-v1 managed enrolled automatic /runtime; };\n')
            body += 'dem_pair_plan ' + ' '.join(shlex.quote(str(x)) for x in (target, root / 'broken-agent', root))
            body += ' source ' + shlex.quote(str(target)) + ' || exit $?; [ "$DP_ENABLED" = 1 ]'
            result = self.shell(body)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_target_helper_is_allowed_for_unenrolled_installation(self):
        with tempfile.TemporaryDirectory(prefix='netdata-no-runtime-') as directory:
            root = Path(directory)
            plugin = root / 'usr/libexec/netdata/plugins.d/dem.plugin'
            plugin.parent.mkdir(parents=True)
            plugin.write_text(
                '#!/bin/sh\nprintf "%s\\n" netdata-dem-pair-plan-v1 managed absent manual /runtime/cache\n')
            plugin.chmod(0o755)
            agent = root / 'usr/sbin/netdata'
            agent.parent.mkdir(parents=True)
            agent.write_text(
                '#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; '
                'then id -un; else printf "%s\\n" "$argument"; fi\n')
            agent.chmod(0o755)
            body = 'dem_pair_plan ' + ' '.join(shlex.quote(str(x)) for x in (root / 'absent-dem.plugin', agent, root))
            result = self.shell(body + ' source || exit $?; [ "$DP_ENABLED" = 0 ]')
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_source_unenrolled_install_needs_no_staging_with_or_without_target_dem(self):
        with tempfile.TemporaryDirectory(prefix='netdata-disabled-dem-') as directory:
            root = Path(directory)
            plugin = root / 'prefix/usr/libexec/netdata/plugins.d/dem.plugin'
            plugin.parent.mkdir(parents=True)
            plugin.write_text(
                '#!/bin/sh\nprintf "%s\\n" netdata-dem-pair-plan-v1 managed absent manual /runtime/cache\n')
            plugin.chmod(0o755)
            agent = root / 'prefix/usr/sbin/netdata'
            agent.parent.mkdir(parents=True)
            agent.write_text(
                '#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; '
                'then id -un; else printf "%s\\n" "$argument"; fi\n')
            agent.chmod(0o755)
            installer = (HELPER.parents[2] / 'netdata-installer.sh').read_text()
            seam = installer.split('# Enrolled installations verify the companion', 1)[1]
            seam = '# Enrolled installations verify the companion' + \
                seam.split('progress "Creating standard user', 1)[0]
            calls = root / 'install calls'
            setup = 'NETDATA_SOURCE_DIR=' + shlex.quote(str(HELPER.parents[2])) + '\n'
            setup += 'NETDATA_PREFIX=' + shlex.quote(str(root / 'prefix')) + '\n'
            setup += 'NETDATA_BUILD_DIR=' + shlex.quote(str(root / 'build-with-dem-disabled')) + '\nDONOTSTART=1\n'
            setup += 'fatal() { exit 69; }; cmake_install() { printf ordinary > ' + shlex.quote(str(calls)) + '; }\n'
            result = subprocess.run(['sh', '-c', setup + seam], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls.read_text(), 'ordinary')
            target = root / 'build-with-dem-disabled/dem.plugin'
            target.parent.mkdir()
            target.write_text('#!/bin/sh\nexit 93\n')
            target.chmod(0o755)
            result = subprocess.run(['sh', '-c', setup + '\nmktemp() { return 42; };\n' + seam],
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls.read_text(), 'ordinary')

    def test_canonical_state_path_resolves_existing_alias_without_creation(self):
        with tempfile.TemporaryDirectory(prefix='netdata-state-alias-') as directory:
            root = Path(directory)
            real = root / 'real'
            real.mkdir()
            alias = root / 'alias'
            alias.symlink_to(real, target_is_directory=True)
            result = self.shell('dem_pair_canonical_path ' + shlex.quote(str(alias / 'future/cache')))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.strip(), str(real.resolve() / 'future/cache'))
            self.assertFalse((real / 'future').exists())

    def test_source_install_failure_keeps_the_installer_error_protocol(self):
        installer = (HELPER.parents[2] / 'netdata-installer.sh').read_text()
        seam = '# Enrolled installations verify the companion' + installer.split(
            '# Enrolled installations verify the companion', 1)[1].split('progress "Creating standard user', 1)[0]
        with tempfile.TemporaryDirectory(prefix='netdata-install-failure-') as directory:
            body = 'NETDATA_SOURCE_DIR=' + shlex.quote(str(HELPER.parents[2])) + '\n'
            body += 'NETDATA_PREFIX=' + shlex.quote(directory) + '\nDONOTSTART=1\n'
            body += ('fatal() { printf "%s\\n" "$*" >&2; exit 1; }; '
                     'cmake_install() { return 2; };\n')
            result = subprocess.run(['sh', '-c', body + seam], capture_output=True, text=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertIn('Failed to install Netdata. I000C', result.stderr)

    def test_root_prefix_aliases_select_one_owner_identity(self):
        with tempfile.TemporaryDirectory(prefix='netdata-prefix-alias-') as directory:
            root = Path(directory).resolve()
            real = root / 'real'
            real.mkdir()
            alias = root / 'alias'
            alias.symlink_to(real, target_is_directory=True)
            states = []
            for prefix in (real, alias):
                body = 'dem_pair_plan /missing/helper /missing/agent ' + \
                    shlex.quote(str(prefix)) + ' source || exit $?\n'
                # Select the root owner's path without creating system state.
                body += ('id() { printf "0\\n"; }; dem_pair_state_location || exit $?\n'
                         'printf "%s\\n%s\\n" "$DP_PREFIX" "$DP_STATE"')
                result = self.shell(body)
                self.assertEqual(result.returncode, 0, result.stderr)
                states.append(result.stdout.splitlines())
            self.assertEqual(states[0], states[1])
            self.assertEqual(states[0][0], str(real))

    def test_source_stage_cleanup_preserves_success_failure_and_signal_status(self):
        for ending, expected in [('exit 0', 0), ('exit 37', 37), ('kill -TERM $$', 143)]:
            result = self.shell('dem_pair_source_stage_new || exit $?; printf "%s\\n" "$DP_DEST"; ' + ending)
            self.assertEqual(result.returncode, expected, result.stderr)
            stage = Path(result.stdout.strip())
            self.assertTrue(stage.name.startswith('netdata-pair-stage.'))
            self.assertFalse(stage.exists(), result.stderr)

    def test_source_stage_cleanup_refuses_unrelated_directory(self):
        with tempfile.TemporaryDirectory(prefix='unrelated-stage-') as directory:
            result = self.shell('DP_DEST=' + shlex.quote(directory) +
                                '; DP_STAGE_CREATED=$DP_DEST; dem_pair_cleanup_stage')
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertTrue(Path(directory).is_dir())

    def test_source_stage_uses_verified_executable_tmpdir(self):
        with tempfile.TemporaryDirectory(prefix='executable stage parent ') as directory:
            probe = Path(directory) / 'exec-check'
            probe.write_text('#!/bin/sh\nexit 0\n')
            probe.chmod(0o755)
            try:
                checked = subprocess.run([str(probe)], capture_output=True, text=True)
            except PermissionError:
                self.skipTest('temporary parent does not permit execution')
            self.assertEqual(checked.returncode, 0, checked.stderr)
            result = self.shell('TMPDIR=' + shlex.quote(directory) + '\n'
                                'dem_pair_source_stage_new || exit $?; printf "%s\\n" "$DP_DEST"; '
                                'printf "#!/bin/sh\\nexit 0\\n" > "$DP_DEST/helper"; '
                                'chmod 755 "$DP_DEST/helper"; "$DP_DEST/helper"')
            self.assertEqual(result.returncode, 0, result.stderr)
            stage = Path(result.stdout.strip())
            self.assertEqual(stage.parent.resolve(), Path(directory).resolve())
            self.assertFalse(stage.exists(), result.stderr)

    def test_source_stage_rejected_tmpdir_falls_back_and_cleans_both_stages(self):
        with tempfile.TemporaryDirectory(prefix='rejected stage parent ') as directory:
            parent = Path(directory).resolve()
            result = self.shell('TMPDIR=' + shlex.quote(str(parent)) + '\n'
                                'dem_pair_source_stage_probe() { '
                                'if [ "$DP_STAGE_PARENT" = "$TMPDIR" ]; then return 126; fi; '
                                '"$DP_DEST/exec-check"; }; '
                                'dem_pair_source_stage_new || exit $?; printf "%s\\n" "$DP_DEST"')
            self.assertEqual(result.returncode, 0, result.stderr)
            stage = Path(result.stdout.strip())
            self.assertIn(stage.parent.resolve(), {Path('/tmp').resolve(), Path('/var/tmp').resolve()})
            self.assertFalse(stage.exists(), result.stderr)
            self.assertEqual(list(parent.iterdir()), [])

    @unittest.skipUnless(os.geteuid() == 0 and SWITCH_USER, 'service boundary requires private root scope')
    def test_source_stage_private_tmpdir_preserves_service_access_and_parent_permissions(self):
        import pwd
        try:
            pwd.getpwnam('nobody')
        except KeyError:
            self.skipTest('service boundary requires an unprivileged nobody account')
        with tempfile.TemporaryDirectory(prefix='private-executable-parent-') as directory:
            mode = Path(directory).stat().st_mode
            result = self.shell('TMPDIR=' + shlex.quote(directory) + '\n'
                                'DP_USER=nobody; DP_LIB=/tmp; DP_CONFIG=/tmp; DP_PREFIX=; '
                                'dem_pair_source_stage_new || exit $?; printf "%s\\n" "$DP_DEST"; '
                                'dem_pair_as_user test -x "$DP_DEST"')
            self.assertEqual(result.returncode, 0, result.stderr)
            stage = Path(result.stdout.strip())
            self.assertNotEqual(stage.parent.resolve(), Path(directory).resolve())
            self.assertFalse(stage.exists(), result.stderr)
            self.assertEqual(Path(directory).stat().st_mode, mode)

    def test_preload_plan_requires_explicit_optin_and_dont_start(self):
        with tempfile.TemporaryDirectory(prefix='netdata-preload-plan-') as directory:
            agent = Path(directory) / 'agent'
            agent.write_text('#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; '
                             'then id -un; else printf "%s\\n" "$argument"; fi\n')
            agent.chmod(0o755)
            setup = ('dem_pair_runtime() { printf "%s\\n" '
                     'netdata-dem-pair-plan-v1 preloaded enrolled manual /sealed/target; }; ')
            plan = 'dem_pair_plan ' + ' '.join(shlex.quote(str(x)) for x in (agent, agent, directory)) + ' source'
            for flags in ('DP_DONT_START=1;', 'NETDATA_DEM_PRELOAD_UPDATE=1; DP_DONT_START=0;'):
                result = self.shell(setup + flags + plan)
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertIn('--dont-start-it', result.stderr)
            result = self.shell(setup + 'NETDATA_DEM_PRELOAD_UPDATE=1; DP_DONT_START=1;' +
                                plan + '; [ "$DP_ENABLED" = 1 ]')
            self.assertEqual(result.returncode, 0, result.stderr)

    @unittest.skipUnless(os.geteuid() == 0 and SWITCH_USER, 'service boundary requires root')
    def test_plan_stages_target_as_resolved_service_user_on_both_install_routes(self):
        import pwd
        try:
            pwd.getpwnam('nobody')
        except KeyError:
            self.skipTest('service boundary requires nobody')
        with tempfile.TemporaryDirectory(prefix='netdata-plan-stage-') as directory:
            root = Path(directory)
            root.chmod(0o755)
            private = root / 'private'
            private.mkdir(mode=0o700)
            prefix = root / 'prefix'
            (prefix / 'etc/netdata').mkdir(parents=True)
            target = private / 'dem.plugin'
            target.write_text('#!/bin/sh\nif [ "$2" = pair-plan ]; then printf "%s\\n" '
                              'netdata-dem-pair-plan-v1 managed enrolled automatic /runtime; else id -un; fi\n')
            target.chmod(0o755)
            agent = private / 'netdata'
            agent.write_text('#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; '
                             'then echo nobody; else printf "%s\\n" "$argument"; fi\n')
            agent.chmod(0o755)
            installed = prefix / 'usr/libexec/netdata/plugins.d/dem.plugin'
            installed.parent.mkdir(parents=True)
            for installed_plan in (False, True):
                if installed_plan:
                    shutil.copyfile(target, installed)
                    installed.chmod(0o755)
                for flavor in ('source', 'static'):
                    with self.subTest(installed_plan=installed_plan, flavor=flavor):
                        body = 'TMPDIR=' + shlex.quote(str(private)) + '\n'
                        body += 'dem_pair_plan ' + shlex.join([str(target), str(agent), str(prefix), flavor])
                        body += ' || exit $?\n[ "$DP_ENABLED" = 1 ] || exit 92\n'
                        body += 'dem_pair_runtime probe || exit $?\nprintf "%s\\n" "$DP_TOOL"\n'
                        result = self.shell(body)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        user, staged = result.stdout.splitlines()
                        self.assertEqual(user, 'nobody')
                        self.assertNotEqual(Path(staged), target)
                        self.assertFalse(Path(staged).exists())
                        self.assertEqual(private.stat().st_mode & 0o777, 0o700)

    @unittest.skipUnless(os.geteuid() == 0, 'missing service account bootstrap requires root')
    def test_missing_service_account_accepts_only_truly_absent_managed_storage(self):
        import pwd
        missing_user = 'netdata-pair-test-absent-user'
        try:
            pwd.getpwnam(missing_user)
        except KeyError:
            pass
        else:
            self.skipTest('fixture account unexpectedly exists')
        with tempfile.TemporaryDirectory(prefix='netdata-plan-bootstrap-') as directory:
            root = Path(directory)
            (root / 'etc/netdata').mkdir(parents=True)
            agent = root / 'netdata'
            agent.write_text('#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; then echo ' +
                             missing_user + '; else printf "%s\\n" "$argument"; fi\n')
            agent.chmod(0o755)
            target = root / 'dem.plugin'
            cache = root / 'runtime'
            existing = root / 'existing'
            existing.mkdir()
            alias = root / 'dangling'
            alias.symlink_to(cache)
            for mode, enrollment, path, accepted in (
                ('managed', 'absent', cache, True),
                ('managed', 'absent', existing, False),
                ('managed', 'absent', alias, False),
                ('managed', 'enrolled', cache, False),
                ('preloaded', 'absent', cache, False),
            ):
                with self.subTest(mode=mode, enrollment=enrollment, path=path):
                    target.write_text('#!/bin/sh\nprintf "%s\\n" ' + shlex.join([
                        'netdata-dem-pair-plan-v1', mode, enrollment, 'manual', str(path)]) + '\n')
                    target.chmod(0o755)
                    body = 'dem_pair_plan ' + shlex.join([str(target), str(agent), str(root), 'source'])
                    body += ' || exit $?\n[ "$DP_ENABLED" = 0 ] && [ -z "$DP_STAGE_CREATED" ]'
                    result = self.shell(body)
                    if accepted:
                        self.assertEqual(result.returncode, 0, result.stderr)
                    else:
                        self.assertNotEqual(result.returncode, 0, result.stderr)
                    self.assertFalse(cache.exists())


def path_without(directory, hidden):
    """A PATH directory linking every command on PATH except the hidden ones."""
    for entry in os.environ.get('PATH', '').split(os.pathsep):
        if not os.path.isabs(entry) or not os.path.isdir(entry):
            continue
        for name in os.listdir(entry):
            link = Path(directory) / name
            source = Path(entry) / name
            if name in hidden or link.exists() or link.is_symlink():
                continue
            if source.is_file() and os.access(source, os.X_OK):
                link.symlink_to(source)
    return str(directory)


@unittest.skipUnless(GNU_LINUX and os.geteuid() == 0, 'switching to the service account requires Linux root')
class SwitchUserTests(unittest.TestCase):
    """dem_pair_as_user with runuser hidden, as on BusyBox hosts."""

    def setUp(self):
        import pwd
        try:
            pwd.getpwnam('nobody')
        except KeyError:
            self.skipTest('requires an unprivileged nobody account')
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-dem-switch-')
        self.addCleanup(self.temp.cleanup)
        # The service account resolves commands through the PATH directory below.
        Path(self.temp.name).chmod(0o755)

    def run_as_user(self, hidden, arguments, stdin=''):
        bin_dir = Path(self.temp.name) / ('bin-' + '-'.join(sorted(hidden)))
        bin_dir.mkdir()
        body = ('PATH=' + shlex.quote(path_without(bin_dir, hidden)) + '\n. ' + shlex.quote(str(HELPER)) + '\n'
                'NETDATA_RUN_DIR=/run-dir; export NETDATA_RUN_DIR\n'
                'DP_USER=nobody DP_LIB=/lib-dir DP_CONFIG=/config-dir DP_STOCK=/stock-dir DP_PREFIX=\n'
                'dem_pair_as_user ' + shlex.join(arguments))
        return subprocess.run(['sh', '-c', body], text=True, capture_output=True, input=stdin)

    @unittest.skipUnless(shutil.which('su'), 'su unavailable')
    def test_su_fallback_preserves_identity_arguments_environment_stdin_and_status(self):
        script = ('id -un; printf "%s\\n" "$PATH" "$NETDATA_RUN_DIR"; '
                  'printf "%s|%s|%s\\n" "$NETDATA_LIB_DIR" "$NETDATA_USER_CONFIG_DIR" '
                  '"$NETDATA_STOCK_CONFIG_DIR"; for a do printf "<%s>\\n" "$a"; done; cat; exit 37')
        arguments = ['', '-', '--config-dir', '-c', 'a b', "it's", '$(false)']
        result = self.run_as_user({'runuser'}, ['sh', '-c', script, 'sh'] + arguments, stdin='from-stdin\n')
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertIn("'su' '-s' '/bin/sh'", result.stderr)
        self.assertEqual(result.stdout.splitlines(),
                         ['nobody', str(Path(self.temp.name) / 'bin-runuser'), '/run-dir',
                          '/lib-dir|/config-dir|/stock-dir'] +
                         ['<' + a + '>' for a in arguments] + ['from-stdin'])

    def test_missing_switch_tools_report_the_requirement(self):
        result = self.run_as_user({'runuser', 'su'}, ['true'])
        self.assertEqual(result.returncode, 127, result.stderr)
        self.assertIn('runuser or su is required', result.stderr)


class ServiceBoundaryTests(unittest.TestCase):
    def test_target_cli_observes_service_when_installed_cli_is_damaged(self):
        for flavor in ('source', 'static'):
            for damaged in ('missing', 'not-executable', 'invalid'):
                for ping_status, expected in ((0, 0), (1, 0), (255, 1), (37, 37)):
                    with self.subTest(flavor=flavor, damaged=damaged, ping=ping_status), \
                            tempfile.TemporaryDirectory(prefix='netdata-cli-repair-') as directory:
                        root = Path(directory)
                        installed = root / ('bin/netdatacli' if flavor == 'static' else 'usr/sbin/netdatacli')
                        installed.parent.mkdir(parents=True)
                        if damaged != 'missing':
                            installed.write_text('#!/missing/interpreter\n')
                            installed.chmod(0o755 if damaged == 'invalid' else 0o644)
                        target = root / 'target-netdatacli'
                        target.write_text('#!/bin/sh\n[ "$1" = ping ] || exit 99\nexit ' + str(ping_status) + '\n')
                        target.chmod(0o755)
                        script = '. ' + shlex.quote(str(HELPER)) + '\n'
                        script += 'DP_PREFIX=' + shlex.quote(str(root)) + '\nDP_FLAVOR=' + flavor + '\n'
                        script += 'DP_TARGET_CLI=' + shlex.quote(str(target)) + '\ndem_pair_owner_running\n'
                        result = subprocess.run(['sh', '-c', script], text=True, capture_output=True)
                        self.assertEqual(result.returncode, expected, result.stderr)

    def exercise_commands(self, root_dispatch=False, retained_record=True, ping_status=0, shutdown_status=0):
        with tempfile.TemporaryDirectory(prefix='netdata-service-boundary-') as directory:
            root = Path(directory)
            prefix = root / 'private prefix'
            work = root / 'retained'
            work.mkdir()
            trace = root / 'commands'
            q = shlex.quote
            for name in ('system-stop', 'system-start', 'inherited-stop', 'inherited-start'):
                script = root / name
                script.write_text('#!/bin/sh\nprintf "%s\\n" ' + q(name) + ' >> ' + q(str(trace)) + '\n')
                script.chmod(0o755)
            cli = prefix / 'usr/sbin/netdatacli'
            cli.parent.mkdir(parents=True)
            cli.write_text('#!/bin/sh\nprintf "cli:%s:%s\\n" "$1" "$NETDATA_PIPENAME" >> ' + q(str(trace))
                           + '\nif [ "$1" = ping ]; then exit ' + str(ping_status) + '; fi\n'
                           + 'exit ' + str(shutdown_status) + '\n')
            cli.chmod(0o755)
            agent = prefix / 'usr/sbin/netdata'
            agent.write_text('#!/bin/sh\nprintf "agent:%s\\n" "$NETDATA_PIPENAME" >> ' + q(str(trace)) + '\n')
            agent.chmod(0o755)
            # Represents a service record retained by an earlier helper. Reading
            # it itself is observable, independently of command execution.
            if retained_record:
                (work / 'service').write_text('printf "record-read\\n" >> ' + q(str(trace)) + '\n'
                                              + 'NETDATA_STOP_CMD=' + q(str(root / 'system-stop')) + '\n'
                                              + 'NETDATA_START_CMD=' + q(str(root / 'system-start')) + '\n')
            script = '. ' + q(str(HELPER)) + '\n'
            if root_dispatch:
                script += 'id() { if [ "$1" = -u ]; then printf "0\\n"; else command id "$@"; fi; }\n'
            script += 'DP_PREFIX=' + q(str(prefix)) + '\nDP_WORK=' + q(str(work)) + '\nDP_FLAVOR=source\n'
            script += ('DP_AGENT=' + q(str(agent)) + '\nNETDATA_PIPENAME=' +
                       q(str(root / 'task.pipe')) + '; export NETDATA_PIPENAME\n')
            script += 'NETDATA_STOP_CMD=' + q(str(root / 'inherited-stop')) + '\n'
            script += 'NETDATA_START_CMD=' + q(str(root / 'inherited-start')) + '\n'
            script += 'dem_pair_owner_stop || exit $?; dem_pair_owner_start\n'
            result = subprocess.run(['sh', '-c', script], text=True, capture_output=True)
            return result, trace.read_text().splitlines() if trace.exists() else [], str(root / 'task.pipe')

    @unittest.skipIf(os.getuid() == 0, 'nonroot replay boundary requires an unprivileged caller')
    def test_nonroot_ignores_retained_and_inherited_system_commands(self):
        for retained in (False, True):
            with self.subTest(retained_record=retained):
                result, commands, pipe = self.exercise_commands(retained_record=retained)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(commands, ['cli:ping:' + pipe, 'cli:shutdown-agent:' + pipe, 'agent:' + pipe])

    @unittest.skipIf(os.getuid() == 0, 'nonroot replay boundary requires an unprivileged caller')
    def test_nonroot_cli_inspection_failure_preserves_status_without_starting(self):
        result, commands, pipe = self.exercise_commands(ping_status=37)
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(commands, ['cli:ping:' + pipe])

    @unittest.skipIf(os.getuid() == 0, 'nonroot CLI shutdown requires an unprivileged caller')
    def test_nonroot_cli_shutdown_error_preserves_status_without_starting(self):
        result, commands, pipe = self.exercise_commands(shutdown_status=37)
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(commands, ['cli:ping:' + pipe, 'cli:shutdown-agent:' + pipe])

    @unittest.skipUnless(GNU_LINUX and os.getuid() != 0, 'requires Linux process identity and nonroot CLI shutdown')
    def test_shutdown_without_reply_requires_actual_process_exit_before_switching(self):
        for stops in (True, False):
            with self.subTest(stops=stops), tempfile.TemporaryDirectory(prefix='netdata-shutdown-reply-') as directory:
                root = Path(directory)
                # Multicall tools may select sleep from the kernel executable path.
                agent = root / 'usr/sbin/sleep'
                agent.parent.mkdir(parents=True)
                shutil.copyfile(shutil.which('sleep'), agent)
                agent.chmod(0o755)
                process = subprocess.Popen([str(agent), '30'])
                try:
                    self.assertEqual(os.stat('/proc/' + str(process.pid) + '/exe').st_ino, agent.stat().st_ino)
                    self.assertIsNone(process.poll())
                    cli = agent.with_name('netdatacli')
                    cli.write_text('#!/bin/sh\n[ "$1" != ping ] || exit 0\n'
                                   + ('kill -TERM ' + str(process.pid) + '\n' if stops else '') + 'exit 255\n')
                    cli.chmod(0o755)
                    marker = root / 'switch-allowed'
                    q = shlex.quote
                    script = '. ' + q(str(HELPER)) + '\n'
                    script += 'DP_PREFIX=' + q(str(root)) + '\nDP_WORK=' + q(str(root)) + '\n'
                    script += 'DP_AGENT=' + q(str(agent)) + '\nDP_FLAVOR=source\nDP_MODE=managed\n'
                    script += 'dem_pair_record_executables || exit $?\ndem_pair_owner_stop || exit $?\n'
                    script += 'dem_pair_wait_stopped 2 || exit $?\n: > ' + q(str(marker)) + '\n'
                    result = subprocess.run(['sh', '-c', script], text=True, capture_output=True, timeout=10)
                    self.assertEqual(result.returncode, 0 if stops else 75, result.stderr)
                    self.assertEqual(marker.exists(), stops)
                    if stops:
                        process.wait(timeout=10)
                    else:
                        self.assertIsNone(process.poll())
                finally:
                    # Only terminate the exact fixture executable started above.
                    if process.poll() is None:
                        process.terminate()
                    process.wait(timeout=10)

    def test_root_policy_dispatch_preserves_registered_commands(self):
        result, commands, _ = self.exercise_commands(root_dispatch=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(commands, ['record-read', 'system-stop', 'record-read', 'system-start'])


class IPCContextTests(unittest.TestCase):
    def test_capture_uses_child_environment_not_unexported_shell_variables(self):
        script = '. ' + shlex.quote(str(HELPER)) + '''
unset NETDATA_PIPENAME NETDATA_RUN_DIR
NETDATA_PIPENAME=/unexported/pipe
NETDATA_RUN_DIR=/unexported/run
dem_pair_capture_ipc || exit $?
printf '%s\\n' "$DP_IPC_PIPE" "$DP_IPC_RUN"
'''
        result = subprocess.run(['sh', '-c', script], text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ['unset', 'unset'])


class FailureStatusTests(unittest.TestCase):
    def test_static_updater_returns_installer_failure_and_retains_inputs(self):
        with tempfile.TemporaryDirectory(prefix='netdata-updater-fixture-') as directory:
            root = Path(directory)
            prefix = root / 'prefix'
            (prefix / 'etc/netdata').mkdir(parents=True)
            (prefix / 'bin').mkdir()
            cli = prefix / 'bin/netdatacli'
            cli.write_text('#!/bin/sh\nexit 255\n')
            cli.chmod(0o755)
            install_type = prefix / 'etc/netdata/.install-type'
            install_type.write_text("INSTALL_TYPE='fixture-static'\n")
            payload = root / 'failure.run'
            payload.write_text('#!/bin/sh\nexit 37\n')
            scratch = root / 'scratch'
            updater = (HELPER.parent / 'netdata-updater.sh').read_text()
            function = updater.split('update_static() {', 1)[1].split('\nget_new_binpkg_major()', 1)[0]
            function = 'update_static() {' + function
            # Replace only the fixed install prefix so the actual function's
            # failure branch can execute without touching a host installation.
            function = function.replace('/opt/netdata', str(prefix))
            q = shlex.quote
            script = function + '\n'
            script += 'exec 3>&2\nNETDATA_MANIFEST_INSTALL=0\nREINSTALL_OPTIONS=\nlogfile=\n'
            script += 'create_exec_tmp_directory() { ndtmpdir=' + q(str(scratch)) + '; mkdir -p "$ndtmpdir"; }\n'
            script += 'update_available() { return 0; }\nfetch_release_metadata() { archive_base_name=fixture.run; }\n'
            script += 'download() { cp ' + q(str(payload)) + ' "$2"; }\nverify_manifest() { :; }\n'
            script += 'info() { :; }; error() { printf "%s\\n" "$*" >&2; }\n'
            script += 'restore_preview_channel() { exit 91; }\nupdate_static\n'
            result = subprocess.run(['sh', '-c', script], text=True, capture_output=True)
            self.assertEqual(result.returncode, 37, result.stderr)
            self.assertIn('status 37', result.stderr)
            self.assertTrue((scratch / 'fixture.run').is_file())
            self.assertEqual(install_type.read_text(), "INSTALL_TYPE='fixture-static'\n")

    def test_static_start_failure_is_not_reported_as_running(self):
        source = (HELPER.parents[1] / 'makeself/install-or-update.sh').read_text()
        tail = source[source.rindex('if [ ${STARTIT} -eq 0 ]; then'):]
        script = 'STARTIT=1\nNETDATA_PREFIX=/fixture\n'
        script += 'progress() { :; }; create_netdata_conf() { :; }; netdata_banner() { printf "%s\\n" "$*"; }\n'
        script += 'restart_netdata() { return 38; }; run() { return 0; }\n' + tail
        result = subprocess.run(['sh', '-c', script], text=True, capture_output=True)
        self.assertEqual(result.returncode, 38, result.stderr)
        self.assertNotIn('running now', result.stdout)


@unittest.skipUnless(GNU_LINUX, 'static overlay uses Linux GNU tar')
class StaticOverlayTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-static-overlay-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.stage = self.root / 'stage'
        self.target = self.root / 'installed'
        self.marker = self.root / 'postinstall-ran'
        self.scratch = self.root / 'scratch'
        self.scratch.mkdir()
        for base in (self.stage, self.target):
            for relative in ('etc/netdata', 'var/lib/netdata', 'system', 'bin', 'usr/share/netdata'):
                (base / relative).mkdir(parents=True)
        (self.stage / 'bin/bash').symlink_to(shutil.which('bash'))
        (self.stage / 'system/functions.sh').write_text('')
        shutil.copyfile(HELPER, self.stage / 'system/netdata-dem-pair.sh')
        (self.stage / 'system/install-or-update.sh').write_text(
            '#!/bin/sh\nprintf completed > ' + shlex.quote(str(self.marker)) + '\n')
        (self.stage / 'etc/netdata/stock.conf').write_text('stock')
        (self.stage / 'etc/netdata/edit-config').write_text('#!/bin/sh\nexit 0\n')
        (self.stage / 'etc/netdata/edit-config').chmod(0o755)
        (self.stage / 'etc/netdata/.environment').write_text('NETDATA_PREFIX=/opt/netdata\n')
        (self.stage / 'etc/netdata/.install-type').write_text('INSTALL_TYPE="binpkg-static"\n')
        (self.target / 'etc/netdata/local.conf').write_text('administrator config')
        (self.stage / 'var/lib/netdata/history').write_text('must not replace data')
        (self.target / 'var/lib/netdata/history').write_text('retained history')
        for number in range(100):
            (self.stage / 'usr/share/netdata' / str(number)).write_bytes(bytes([number]) * 4096)
        self.script = self.root / 'post-installer.sh'
        source = (HELPER.parents[1] / 'makeself/post-installer.sh').read_text()
        self.script.write_text(source.replace('/opt/netdata', str(self.target)))

    def run_static(self, limit_file_size=False, options=('--dont-start-it',), **environment):
        def limits():
            import resource
            resource.setrlimit(resource.RLIMIT_FSIZE, (65536, 65536))
            resource.setrlimit(resource.RLIMIT_CORE, (0, 0))

        return subprocess.run(['bash', str(self.script), *options], cwd=self.stage,
                              env=dict(os.environ, TMPDIR=str(self.scratch), NETDATA_DEM_STATIC_OWNER_BASH='1',
                                       **environment), text=True, capture_output=True,
                              preexec_fn=limits if limit_file_size else None)

    def test_missing_installer_metadata_is_installed_without_replacing_configuration(self):
        for name in ('.environment', '.install-type'):
            (self.stage / 'etc/netdata' / name).write_text('packaged ' + name)
        result = self.run_static()
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in ('.environment', '.install-type'):
            path = self.target / 'etc/netdata' / name
            self.assertTrue(path.is_file(), name)
            self.assertEqual(path.read_text(), 'packaged ' + name)
            path.write_text('retained ' + name)
        result = self.run_static()
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in ('.environment', '.install-type'):
            self.assertEqual((self.target / 'etc/netdata' / name).read_text(), 'retained ' + name)
        self.assertEqual((self.target / 'etc/netdata/local.conf').read_text(), 'administrator config')
        self.assertFalse((self.target / 'etc/netdata/stock.conf').exists())

    def test_packaged_config_tool_and_missing_directories_update_without_touching_admin_files(self):
        source = self.stage / 'etc/netdata'
        target = self.target / 'etc/netdata'
        (source / 'edit-config').write_text('#!/bin/sh\nprintf current\n')
        (source / 'edit-config').chmod(0o755)
        (source / 'go.d/nested').mkdir(parents=True)
        target.chmod(0o700)
        (target / 'go.d').mkdir(mode=0o711)
        for previous in (None, '#!/bin/sh\nprintf old\n'):
            with self.subTest(previous=previous):
                if previous is not None:
                    (target / 'edit-config').write_text(previous)
                result = self.run_static()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue((target / 'edit-config').is_file())
                self.assertEqual((target / 'edit-config').read_bytes(), (source / 'edit-config').read_bytes())
                self.assertEqual((target / 'edit-config').stat().st_mode & 0o777, 0o755)
                self.assertTrue((target / 'go.d/nested').is_dir())
                self.assertEqual(target.stat().st_mode & 0o777, 0o700)
                self.assertEqual((target / 'go.d').stat().st_mode & 0o777, 0o711)
                self.assertEqual((target / 'local.conf').read_text(), 'administrator config')
                self.assertFalse((target / 'stock.conf').exists())

    def test_relocated_state_parents_remain_symlinks_after_overlay(self):
        self.check_relocated_state_parents()

    @unittest.skipUnless(shutil.which('busybox'), 'BusyBox tar unavailable')
    def test_busybox_overlay_preserves_relocated_state_parents(self):
        help_text = subprocess.run(['busybox', 'tar', '--help'], capture_output=True, text=True)
        if '--exclude' not in help_text.stdout + help_text.stderr:
            self.skipTest('BusyBox build omits the overlay exclusion feature')
        commands = self.root / 'commands'
        commands.mkdir()
        tar = commands / 'tar'
        tar.write_text('#!/bin/sh\nexec ' + shlex.quote(shutil.which('busybox')) + ' tar "$@"\n')
        tar.chmod(0o755)
        self.check_relocated_state_parents(PATH=str(commands) + ':' + os.environ['PATH'])

    def check_relocated_state_parents(self, **environment):
        for relative in ('etc', 'var/lib', 'var/cache', 'var/log'):
            with self.subTest(parent=relative):
                parent = self.target / relative
                parent.mkdir(parents=True, exist_ok=True)
                (self.stage / relative).mkdir(parents=True, exist_ok=True)
                relocated = self.root / ('relocated-' + relative.replace('/', '-'))
                parent.rename(relocated)
                parent.symlink_to(relocated, target_is_directory=True)
                witness = relocated / 'netdata/retained-state'
                witness.parent.mkdir(parents=True, exist_ok=True)
                witness.write_text('retained')
                result = self.run_static(**environment)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(parent.is_symlink())
                self.assertEqual((parent / 'netdata/retained-state').read_text(), 'retained')

    def test_paired_activation_deferral_is_not_saved_for_future_updates(self):
        source = (HELPER.parents[1] / 'makeself/install-or-update.sh').read_text()
        options = source[source.index('STARTIT=1'):source.index('\nif [ ! "${DISABLE_TELEMETRY')]
        save = source[source.index('echo "Save install options"'):source.index('\n# ---',
                      source.index('echo "Save install options"'))]
        # Exercise the real adapter, option parser and persisted environment,
        # excluding unrelated account/service setup on the test host.
        with (self.stage / 'system/netdata-dem-pair.sh').open('a') as helper:
            helper.write('\ndem_pair_plan() { DP_ENABLED=1; }\n'
                         'dem_pair_begin() { :; }\ndem_pair_finish() { :; }\n')
        (self.target / 'bin/bash').symlink_to(shutil.which('bash'))
        environment = self.target / 'etc/netdata/.environment'
        child = 'NETDATA_PREFIX=' + shlex.quote(str(self.target)) + '\n' + options + '\n' + save + '\n'
        child += 'printf "%s\\n" "$STARTIT" > ' + shlex.quote(str(self.marker)) + '\n'
        (self.stage / 'system/install-or-update.sh').write_text(child)

        for arguments in (('--stable-channel',), ('--stable-channel', '--dont-start-it')):
            with self.subTest(arguments=arguments):
                environment.write_text('REINSTALL_OPTIONS=""\n')
                result = self.run_static(options=arguments)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.marker.read_text(), '0\n')
                self.assertIn('--stable-channel', environment.read_text())
                self.assertNotIn('--dont-start-it', environment.read_text())

    def test_unenrolled_overlay_needs_no_second_full_temporary_archive(self):
        # Each payload file fits, but a second full archive exceeds this limit.
        result = self.run_static(limit_file_size=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.marker.exists())
        for number in range(100):
            self.assertEqual((self.target / 'usr/share/netdata' / str(number)).read_bytes(), bytes([number]) * 4096)
        self.assertEqual((self.target / 'etc/netdata/local.conf').read_text(), 'administrator config')
        self.assertFalse((self.target / 'etc/netdata/stock.conf').exists())
        self.assertEqual((self.target / 'var/lib/netdata/history').read_text(), 'retained history')
        self.assertEqual(list(self.scratch.iterdir()), [])

    def test_either_archive_failure_stops_before_postinstall(self):
        commands = self.root / 'commands'
        commands.mkdir()
        tar = commands / 'tar'
        real_tar = shlex.quote(shutil.which('tar'))
        tar.write_text('#!/bin/sh\n'
                       'if [ "$1" = -cf ] && [ "$FAIL_TAR" = create ]; then\n'
                       '  ' + real_tar + ' "$@" || exit $?\n  exit 37\nfi\n'
                       'if [ "$1" = -xpf ] && [ "$FAIL_TAR" = extract ]; then exit 42; fi\n'
                       'exec ' + real_tar + ' "$@"\n')
        tar.chmod(0o755)
        for phase, status in (('create', 37), ('extract', 42)):
            with self.subTest(phase=phase):
                result = self.run_static(PATH=str(commands) + os.pathsep + os.environ['PATH'], FAIL_TAR=phase)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertFalse(self.marker.exists())


class ArchiveEntryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-makeself-test-')
        self.root = Path(self.temp.name)
        self.tree = self.root / 'tree'
        (self.tree / 'system').mkdir(parents=True)
        startup = self.tree / 'system/post-installer.sh'
        startup.write_text('#!/bin/sh\nprintf started > "$1"\n')
        startup.chmod(0o755)
        self.archive = self.root / 'fixture.run'
        self.target = self.root / 'extracted'
        self.started = self.root / 'started'
        self.makeself = HELPER.parents[1] / 'makeself/makeself.sh'
        self.header = self.makeself.with_name('makeself-header.sh')

    def tearDown(self):
        self.temp.cleanup()

    def build(self, netdata=True):
        label = 'Netdata, X-Ray Vision for your infrastructure' if netdata else 'Generic fixture'
        result = subprocess.run(['sh', str(self.makeself), '--gzip', '--header', str(self.header),
                                 str(self.tree), str(self.archive), label, './system/post-installer.sh'],
                                text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def execute(self, *args):
        return subprocess.run(['sh', str(self.archive), '--nox11', *map(str, args)],
                              cwd=self.root, text=True, capture_output=True)

    def test_target_startup_refused_before_any_extraction(self):
        self.build()
        result = self.execute('--target', self.target, '--', self.started)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertFalse(self.target.exists())
        self.assertFalse(self.started.exists())
        self.assertIn('--noexec', result.stderr)

    def test_keep_startup_refused_before_extraction(self):
        self.build()
        result = self.execute('--keep', '--', self.started)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertFalse(self.started.exists())

    def test_noexec_target_inspection_preserved_in_either_argument_order(self):
        self.build()
        for args in [('--noexec', '--target', self.target), ('--target', self.target, '--noexec')]:
            result = self.execute(*args)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue((self.target / 'system/post-installer.sh').exists())
            self.assertFalse(self.started.exists())
            shutil.rmtree(self.target)

    def test_default_staging_runs_and_generic_target_still_runs(self):
        self.build()
        result = self.execute('--', self.started)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.started.read_text(), 'started')
        self.started.unlink()
        self.build(netdata=False)
        result = self.execute('--target', self.target, '--', self.started)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.started.read_text(), 'started')


@unittest.skipUnless(GNU_LINUX, "owner retirement uses Linux ownership/flock")
class LegacyAbsenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-legacy-empty-')
        self.root = Path(self.temp.name)
        self.prefix = self.root / 'prefix'
        self.runtime = self.root / 'runtime'
        self.runtime.mkdir(mode=0o700)
        for name in ('generations', 'claims', 'uses', 'staging', 'trash', 'initializing'):
            (self.runtime / name).mkdir(mode=0o700)
        for name, data in [('layout.json', '{"format":1,"phase":"ready"}\n'),
                           ('selection.json', '{"release":null}\n'),
                           ('coordination.lock', ''), ('installation.lock', '')]:
            (self.runtime / name).write_text(data)
            (self.runtime / name).chmod(0o600)
        self.agent = self.prefix / 'usr/sbin/netdata'
        self.agent.parent.mkdir(parents=True)
        self.agent.write_text(
            '#!/bin/sh\nfor argument do :; done\nif [ "$argument" = netdata ]; '
            'then id -un; else printf "%s\\n" "$argument"; fi\n')
        self.agent.chmod(0o755)
        self.plugin = self.prefix / 'usr/libexec/netdata/plugins.d/dem.plugin'
        self.plugin.parent.mkdir(parents=True)
        self.called = self.root / 'status-called'
        self.plugin.write_text('#!/bin/sh\ncase "$2" in\n'
                               'setup-plan) printf "%s\\n" netdata-dem-runtime-setup-v1 standalone managed '
                               + shlex.quote(str(self.runtime)) + ';;\n'
                               'status) : > ' + shlex.quote(str(self.called)) +
                               '; printf "%s\\n" "${LEGACY_DIAGNOSTIC:-matching browser runtime '
                               'is not prepared}" >&2; exit 1;;\n'
                               '*) exit 1;;\nesac\n')
        self.plugin.chmod(0o755)
        config = self.prefix / 'etc/netdata'
        config.mkdir(parents=True)
        (config / 'dem.conf').write_text('runtime: {mode: managed, cache_path: ' + str(self.runtime) + '}\n')

    def tearDown(self):
        self.temp.cleanup()

    def plan(self, extra=''):
        q = shlex.quote
        body = '. ' + q(str(HELPER)) + '\n' + extra + '\n'
        body += ('dem_pair_plan ' + ' '.join(
            q(str(x)) for x in (self.root / 'missing-target', self.agent, self.prefix)) + ' source')
        body += ' || exit $?; [ "$DP_ENABLED" = 0 ]'
        return subprocess.run(['sh', '-c', body], text=True, capture_output=True)

    def test_accepts_only_known_ready_empty_selections(self):
        for selection in ('{"release":null}\n', '{"manifest":null}\n'):
            (self.runtime / 'selection.json').write_text(selection)
            result = self.plan()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(self.called.is_file())
            self.assertEqual((self.runtime / 'selection.json').read_text(), selection)

    def test_missing_selection_initialization_and_any_proof_are_not_absence(self):
        mutations = [('missing selection', lambda: (self.runtime / 'selection.json').unlink()),
                     ('initializing layout', lambda: (self.runtime /
                      'layout.json').write_text('{"format":1,"phase":"initializing"}\n')),
                     ('recovery journal', lambda: (self.runtime / 'recovery.json').write_text('{}'))]
        mutations += [(name, lambda name=name: (self.runtime / name / 'retained-proof').write_text('preserve'))
                      for name in ('generations', 'claims', 'uses', 'staging', 'trash', 'initializing')]
        for name, mutate in mutations:
            with self.subTest(name=name):
                (self.runtime / 'layout.json').write_text('{"format":1,"phase":"ready"}\n')
                (self.runtime / 'selection.json').write_text('{"release":null}\n')
                for item in self.runtime.rglob('retained-proof'):
                    item.unlink()
                (self.runtime / 'recovery.json').unlink(missing_ok=True)
                self.called.unlink(missing_ok=True)
                mutate()
                result = self.plan()
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertFalse(self.called.exists(), 'unsafe root must never reach legacy Open/status')

    def test_status_exit_one_with_other_diagnostic_is_not_absence(self):
        result = self.plan('LEGACY_DIAGNOSTIC="unreadable ownership proof"; export LEGACY_DIAGNOSTIC')
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertTrue(self.called.exists())

    @unittest.skipUnless(os.environ.get('DEM_LEGACY_RUNTIME_HELPER'), 'actual baseline CLI not supplied')
    def test_actual_baseline_cli_ready_empty_and_missing_selection(self):
        shutil.copyfile(os.environ['DEM_LEGACY_RUNTIME_HELPER'], self.plugin)
        self.plugin.chmod(0o755)
        result = self.plan()
        self.assertEqual(result.returncode, 0, result.stderr)
        before = (self.runtime / 'layout.json').read_bytes()
        (self.runtime / 'selection.json').unlink()
        result = self.plan()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertFalse((self.runtime / 'selection.json').exists())
        self.assertEqual((self.runtime / 'layout.json').read_bytes(), before)


@unittest.skipUnless(GNU_LINUX, "installation owner fixtures require Linux/flock")
class OwnerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-dem-pair-test-')
        self.root = Path(self.temp.name)
        self.live = self.root / 'live prefix'
        self.stage = self.root / 'stage'
        self.state = self.root / 'state'
        self.events = self.root / 'events'
        for base in (self.live, self.stage):
            (base / 'usr/libexec/netdata/plugins.d').mkdir(parents=True)
            (base / 'usr/sbin').mkdir(parents=True)
            (base / 'usr/share/netdata/web').mkdir(parents=True)
            (base / 'etc/netdata').mkdir(parents=True)
            (base / 'var/lib/netdata/dem/runtime').mkdir(parents=True)
        self.core = self.live / 'usr/sbin/netdata'
        self.core.write_text('old core')
        self.core.chmod(0o751)
        (self.stage / 'usr/sbin/netdata').write_text('new core')
        (self.stage / 'usr/sbin/netdata').chmod(0o755)
        self.plugin = self.live / 'usr/libexec/netdata/plugins.d/dem.plugin'
        self.plugin.write_text('old helper')
        self.plugin.chmod(0o750)
        self.config = self.live / 'etc/netdata/dem.conf'
        self.config.write_text('administrator config')
        (self.stage / 'etc/netdata/dem.conf').write_text('stock override forbidden')
        self.proof = self.live / 'var/lib/netdata/dem/runtime/proof'
        self.proof.write_text('private ownership proof')
        (self.stage / 'var/lib/netdata/dem/runtime/proof').write_text('forbidden proof')
        self.new_file = self.stage / 'usr/share/netdata/web/new.html'
        self.new_file.write_text('new payload')
        self.target = self.stage / 'usr/libexec/netdata/plugins.d/dem.plugin'
        self.target.write_text('''#!/bin/sh
command=$2
shift 2
if [ "$command" = pair-plan ]; then
    case " $* " in *" --source-update "*) ;; *) exit 98 ;; esac
    if [ "${PAIR_FAIL_SOURCE_PLAN:-0}" != 0 ]; then exit "$PAIR_FAIL_SOURCE_PLAN"; fi
    if [ -n "${PAIR_SOURCE_PLAN_ONCE:-}" ]; then
        if [ -f "$PAIR_SOURCE_PLAN_ONCE" ]; then exit 49; fi
        : > "$PAIR_SOURCE_PLAN_ONCE"
    fi
    printf '%s\\n' "${PAIR_SOURCE_PLAN:-paired}"
    exit 0
fi
printf '%s\\n' "$command" >> ''' + shlex.quote(str(self.events)) + '''
if [ "$command" = fetch ] && [ "${PAIR_FAIL_FETCH:-0}" != 0 ]; then exit "$PAIR_FAIL_FETCH"; fi
if [ "$command" = activate ] && [ "${PAIR_FAIL_ACTIVATE:-0}" != 0 ]; then exit "$PAIR_FAIL_ACTIVATE"; fi
if [ "$command" = prepare ] && [ "${PAIR_FAIL_PREPARE:-0}" != 0 ]; then exit "$PAIR_FAIL_PREPARE"; fi
if [ "$command" = preload-validate ] && [ "${PAIR_FAIL_PRELOAD_VALIDATE:-0}" != 0 ]; then
    exit "$PAIR_FAIL_PRELOAD_VALIDATE"
fi
if [ "$command" = preload-validate ] && [ -n "${PAIR_PRELOAD_VALIDATE_ONCE:-}" ]; then
    if [ -f "$PAIR_PRELOAD_VALIDATE_ONCE" ]; then exit 49; fi
    : > "$PAIR_PRELOAD_VALIDATE_ONCE"
fi
if [ "$command" = fetch ]; then
    while [ "$#" != 0 ]; do
        if [ "$1" = --output ]; then output=$2; shift; fi
        shift
    done
    mkdir -m 700 "$output" || exit $?
    for file in release.json runtime.tar.zst release-public.gpg; do printf fixture > "$output/$file"; done
fi
exit 0
''')
        self.target.chmod(0o755)

    def tearDown(self):
        self.temp.cleanup()

    def run_owner(self, body='dem_pair_finish', running=False, extra=''):
        q = shlex.quote
        init = '\n'.join([
            '. ' + q(str(HELPER)), 'DP_ENABLED=1', 'DP_CONSENT=automatic', 'DP_MODE=managed',
            'DP_PREFIX=' + q(str(self.live)), 'DP_TOOL=' + q(str(self.target)),
            'DP_AGENT=' + q(str(self.core)), 'DP_USER=$(id -un)',
            'DP_CONFIG=' + q(str(self.live / 'etc/netdata')),
            'DP_LIB=' + q(str(self.live / 'var/lib/netdata')),
            'DP_STOCK=' + q(str(self.live / 'usr/lib/netdata/conf.d')),
            'DP_CACHE=' + q(str(self.live / 'var/lib/netdata/dem/runtime')),
            'DP_FLAVOR=source', 'NETDATA_DEM_PAIR_STATE_DIR=' + q(str(self.state)),
            'dem_pair_owner_running() { return ' + ('0' if running else '1') + '; }',
            'dem_pair_owner_stop() { printf "stop\\n" >> ' + q(str(self.events)) + '; }',
            'dem_pair_owner_start() { printf "start\\n" >> ' + q(str(self.events)) + '; }',
            extra, "trap 'dem_pair_exit \"$?\"' EXIT",
            'dem_pair_begin || exit $?', body,
        ])
        return subprocess.run(['sh', '-c', init], text=True, capture_output=True)

    def recorded(self):
        return self.events.read_text().splitlines() if self.events.exists() else []

    def test_update_coordinates_runtime_without_copying_agent_payload(self):
        result = self.run_owner(running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded(), ['fetch', 'stop', 'prepare', 'activate', 'start'])
        self.assertEqual(self.core.read_text(), 'old core')
        self.assertEqual(self.config.read_text(), 'administrator config')
        self.assertEqual(self.proof.read_text(), 'private ownership proof')
        self.assertEqual([p.name for p in self.state.iterdir()], ['lock'])

    def test_unstamped_source_keeps_runtime_without_acquisition(self):
        for consent in ('manual', 'automatic'):
            with self.subTest(consent=consent):
                self.events.write_text('')
                extra = 'export PAIR_SOURCE_PLAN=retain\nDP_CONSENT=' + consent
                result = self.run_owner(running=True, extra=extra)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.recorded(), ['stop', 'start'])
                self.assertEqual(self.proof.read_text(), 'private ownership proof')
                self.assertEqual(self.config.read_text(), 'administrator config')
                self.assertIn('Retaining the installed runtime', result.stderr)

    def test_source_retention_validation_failure_precedes_stop(self):
        result = self.run_owner(running=True, extra='export PAIR_FAIL_SOURCE_PLAN=37')
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(self.recorded(), [])
        self.assertEqual(self.core.read_text(), 'old core')

    def test_source_retention_rechecks_before_restart(self):
        marker = self.root / 'source-validation'
        extra = 'export PAIR_SOURCE_PLAN=retain PAIR_SOURCE_PLAN_ONCE=' + shlex.quote(str(marker))
        result = self.run_owner(running=True, extra=extra)
        self.assertEqual(result.returncode, 49, result.stderr)
        self.assertEqual(self.recorded(), ['stop'])
        self.assertEqual(self.proof.read_text(), 'private ownership proof')

    def test_static_updates_still_require_the_exact_pair(self):
        result = self.run_owner(running=True, extra='DP_FLAVOR=static\nexport PAIR_SOURCE_PLAN=retain')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded(), ['fetch', 'stop', 'prepare', 'activate', 'start'])

    def test_inactive_or_explicit_dont_start_stays_stopped(self):
        for running, extra in [(False, ''), (True, 'DP_DONT_START=1')]:
            with self.subTest(running=running):
                self.events.write_text('')
                result = self.run_owner(running=running, extra=extra)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotIn('start', self.recorded())

    def test_failed_acquisition_does_not_stop_or_modify_installation(self):
        result = self.run_owner(running=True, extra='export PAIR_FAIL_FETCH=37')
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(self.recorded(), ['fetch'])
        self.assertEqual(self.core.read_text(), 'old core')
        self.assertNotIn('Agent update incomplete', result.stderr)

    def test_manual_without_inputs_refuses_before_stop(self):
        result = self.run_owner(running=True, extra='DP_CONSENT=manual')
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(self.recorded(), [])
        self.assertIn('manual runtime updates require', result.stderr)

    def test_invalid_ipc_context_refuses_before_fetch_or_shutdown(self):
        for variable, value in [('NETDATA_PIPENAME', 'relative'), ('NETDATA_RUN_DIR', '/bad\npath')]:
            with self.subTest(variable=variable):
                result = self.run_owner(running=True, extra='export ' + variable + '=' + shlex.quote(value))
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(self.recorded(), [])
                self.assertEqual(self.core.read_text(), 'old core')

    def test_postinstall_cannot_change_runtime_or_configuration_permissions(self):
        for state in ('DP_CACHE', 'DP_CONFIG', 'DP_LIB'):
            with self.subTest(state=state):
                result = self.run_owner(extra=state + '=' + shlex.quote(str(self.live / 'usr/libexec/netdata/private')))
                self.assertNotEqual(result.returncode, 0, result.stderr)
                self.assertIn('overlaps an Agent postinstall tree', result.stderr)
                self.assertEqual(self.recorded(), [])

    def test_partial_agent_replacement_reports_failure_without_restoring(self):
        body = 'printf partial > ' + shlex.quote(str(self.core)) + '; exit 47'
        result = self.run_owner(body, running=True)
        self.assertEqual(result.returncode, 47, result.stderr)
        self.assertEqual(self.core.read_text(), 'partial')
        self.assertEqual(self.recorded(), ['fetch', 'stop', 'prepare'])
        self.assertIn('no Agent files were restored', result.stderr)
        self.assertEqual(self.config.read_text(), 'administrator config')
        self.assertEqual(self.proof.read_text(), 'private ownership proof')
        # The next invocation is not blocked by an installer recovery slot.
        self.events.write_text('')
        result = self.run_owner('printf repaired > ' + shlex.quote(str(self.core)) + '; dem_pair_finish')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.core.read_text(), 'repaired')
        self.assertEqual(self.recorded(), ['fetch', 'stop', 'prepare', 'activate'])

    def test_prepare_and_activation_failure_leave_stopped_and_preserve_status(self):
        for phase, status in [('PREPARE', 38), ('ACTIVATE', 39)]:
            with self.subTest(phase=phase):
                self.events.write_text('')
                result = self.run_owner(running=True, extra=f'export PAIR_FAIL_{phase}={status}')
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertNotIn('start', self.recorded())
                self.assertNotIn('rollback', self.recorded())
                self.assertIn('Runtime selection and ownership records are preserved', result.stderr)

    def test_transient_staging_is_cleaned_after_success_and_failure(self):
        for ending in ['dem_pair_finish', 'exit 47']:
            result = self.run_owner('printf "%s\\n" "$DP_WORK"; ' + ending)
            stage = Path(result.stdout.strip())
            self.assertTrue(stage.name.startswith('netdata-pair-stage.'))
            self.assertFalse(stage.exists(), result.stderr)

    def test_start_uses_original_invocation_ipc_after_postinstall_defaults(self):
        trace = self.root / 'ipc'
        extra = 'export NETDATA_PIPENAME=/private/original; unset NETDATA_RUN_DIR\n'
        extra += ('dem_pair_owner_start() { printf "%s:%s" "$NETDATA_PIPENAME" "${NETDATA_RUN_DIR-unset}" > '
                  + shlex.quote(str(trace)) + '; }')
        result = self.run_owner('export NETDATA_PIPENAME=/changed NETDATA_RUN_DIR=/changed; dem_pair_finish',
                                running=True, extra=extra)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(trace.read_text(), '/private/original:unset')

    def test_preload_validates_target_without_previous_root_or_snapshot(self):
        result = self.run_owner(extra='DP_MODE=preloaded; DP_DONT_START=1; NETDATA_DEM_PRELOAD_UPDATE=1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded(), ['preload-validate', 'preload-validate'])
        self.assertEqual(self.core.read_text(), 'old core')
        self.assertEqual([p.name for p in self.state.iterdir()], ['lock'])

    def test_preload_running_agent_refused_before_installation(self):
        result = self.run_owner(running=True, extra='DP_MODE=preloaded; DP_DONT_START=1')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.recorded(), ['preload-validate'])
        self.assertEqual(self.core.read_text(), 'old core')

    def test_existing_installation_lock_prevents_fetch(self):
        self.state.mkdir()
        import fcntl
        with (self.state / 'lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_owner()
            self.assertNotEqual(result.returncode, 0, result.stderr)
            self.assertEqual(self.recorded(), [])


if __name__ == '__main__':
    unittest.main()
