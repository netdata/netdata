# SPDX-License-Identifier: GPL-3.0-or-later
"""Execute shipped release scripts and check publication dependency boundaries.

Graph checks inspect declared dependencies; they do not model GitHub scheduling.
"""
from pathlib import Path
import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest

from ruamel.yaml import YAML

ROOT = Path(__file__).resolve().parents[3]
WORKFLOWS = ROOT / '.github/workflows'
YAML_READER = YAML(typ='safe')


def load(name):
    with (WORKFLOWS / name).open() as stream:
        return YAML_READER.load(stream)


def dependencies(jobs, name):
    result = set()
    pending = [name]
    while pending:
        node = pending.pop()
        needs = jobs[node].get('needs', [])
        needs = [needs] if isinstance(needs, str) else needs
        for dependency in needs:
            if dependency not in result:
                result.add(dependency)
                pending.append(dependency)
    return result


def steps(job):
    return job.get('steps', [])


class CoupledWorkflowTests(unittest.TestCase):
    def test_publication_dependencies_precede_agent_exposure(self):
        boundaries = (
            ('release.yml', 'runtime-companion',
             ('trigger-artifacts', 'trigger-packages', 'trigger-docker')),
            ('dem-runtime-release.yml', 'build', ('publish',)),
            ('build.yml', 'release-companion',
             ('prepare-upload', 'create-nightly', 'publish-nightly-artifacts',
              'upload-release', 'publish-stable-artifacts')),
            ('packaging.yml', 'release-companion', ('build',)),
            ('docker.yml', 'release-companion',
             ('build-images-docker-hub', 'build-images-quay', 'build-images-ghcr',
              'publish-docker-hub', 'publish-quay', 'publish-ghcr')),
        )
        for filename, prerequisite, consumers in boundaries:
            jobs = load(filename)['jobs']
            for consumer in consumers:
                with self.subTest(workflow=filename, consumer=consumer):
                    self.assertIn(prerequisite, dependencies(jobs, consumer))

    @unittest.skipUnless(shutil.which('git'), 'requires Git')
    def test_release_base_selects_canonical_release_channels(self):
        cases = [('stable', 'patch', 'v2.12.1', 'v2.12.1'),
                 ('release-candidate', 'release-candidate', 'v2.13.0-rc01', 'v2.13.0-rc01'),
                 ('nightly', 'nightly', 'nightly', 'v2.12.0-0-post-nightly')]
        for channel, release_type, version, expected_version in cases:
            with self.subTest(channel=channel), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                (root / 'packaging').mkdir()
                (root / 'packaging/version').write_text('v2.12.0\n')
                # Release preparation requires dispatched workflows on the branch to accept the release inputs.
                (root / '.github/workflows').mkdir(parents=True)
                for name in ('build', 'docker', 'packaging'):
                    (root / f'.github/workflows/{name}.yml').write_text(
                        'on:\n  workflow_dispatch:\n    inputs:\n      source_sha:\n      companion_run_id:\n')

                def git(*args):
                    subprocess.run(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                                    *args], cwd=root, check=True, capture_output=True)
                git('init', '-q', '-b', 'master')
                git('add', 'packaging/version', '.github')
                git('commit', '-qm', 'Fixture')
                git('tag', '-a', 'v2.12.0', '-m', 'Fixture')
                git('branch', 'release-v2.12')
                output = root / 'outputs'
                result = subprocess.run(['sh', str(ROOT / '.github/scripts/prepare-release-base.sh'),
                                         'netdata/netdata', 'workflow_dispatch', release_type, version, ''],
                                        cwd=root, env=dict(os.environ, GITHUB_OUTPUT=str(output)),
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                fields = dict(line.split('=', 1) for line in output.read_text().splitlines())
                self.assertEqual(fields['channel'], channel)
                self.assertEqual(fields['run'], 'true')
                self.assertEqual((root / 'packaging/version').read_text().strip(), expected_version)

    def test_lane_transport_refuses_mismatched_version_or_missing_frozen_inputs(self):
        gate = load('dem-runtime-companion-check.yml')['jobs']['check']
        script = next(step['run'] for step in steps(gate) if step.get('name') == 'Require immutable release transport')
        for version, request, channel, source, run, success in [
            ('v2.12.0', '2.12.0', 'stable', 'a' * 40, '123', True),
            ('v2.12.0-9-nightly', 'nightly', 'nightly', 'a' * 40, '123', True),
            ('v2.12.0', '2.13.0', 'stable', 'a' * 40, '123', False),
            ('v2.13.0-rc1', '2.13.0-rc1', 'stable', 'a' * 40, '123', True),
            ('v2.12.0', '', 'devel', 'a' * 40, '123', True),
            ('v2.12.0', 'ignored-native-label', 'devel', 'a' * 40, '123', True),
            ('v2.12.0', '', 'preview', 'a' * 40, '123', False),
            ('v2.12.0', 'nightly', 'nightly', 'a' * 40, '123', False),
            ('v2.12.0', '2.12.0', 'stable', '', '123', False),
            ('v2.12.0', '2.12.0', 'stable', 'a' * 40, '', False),
        ]:
            with (self.subTest(version=version, request=request, source=source, run=run),
                  tempfile.TemporaryDirectory() as temporary):
                root = Path(temporary)
                (root / 'packaging').mkdir()
                (root / 'packaging/version').write_text(version + '\n')
                result = subprocess.run(['bash', '-e', '-c', script], cwd=root, capture_output=True, text=True,
                                        env=dict(os.environ, SOURCE_SHA=source, COMPANION_RUN=run,
                                                 REQUESTED_VERSION=request, CHANNEL=channel))
                self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)

    @unittest.skipUnless(shutil.which('node'), 'requires Node for the GitHub API fixture')
    def test_lane_requires_successful_release_owned_publication(self):
        gate = load('dem-runtime-companion-check.yml')['jobs']['check']
        script = next(step['with']['script'] for step in steps(gate)
                      if 'Require successful companion publication' in step.get('name', ''))
        publisher_name = load('dem-runtime-release.yml')['jobs']['publish']['name']
        cases = [(path, 'workflow_dispatch', 'fixture/fixture', conclusion, success) for path, conclusion, success in [
            ('.github/workflows/release.yml', 'success', True),
            ('.github/workflows/build.yml', 'success', True),
            ('.github/workflows/packaging.yml', 'success', True),
            ('.github/workflows/docker.yml', 'success', True),
            ('.github/workflows/dem-runtime-release.yml', 'success', False),
            ('.github/workflows/unrelated.yml', 'success', False),
            ('.github/workflows/release.yml', 'failure', False),
            ('.github/workflows/release.yml', 'skipped', False),
        ]]
        cases += [
            ('.github/workflows/release.yml', 'schedule', 'fixture/fixture', 'success', True),
            ('.github/workflows/build.yml', 'schedule', 'fixture/fixture', 'success', False),
            ('.github/workflows/build.yml', 'pull_request', 'fixture/fixture', 'success', False),
            ('.github/workflows/build.yml', 'pull_request_target', 'fixture/fixture', 'success', False),
            ('.github/workflows/packaging.yml', 'push', 'fixture/fixture', 'success', False),
            ('.github/workflows/docker.yml', 'workflow_run', 'fixture/fixture', 'success', False),
            ('.github/workflows/build.yml', 'workflow_dispatch', 'contributor/fork', 'success', False),
        ]
        for path, event, repository, conclusion, success in cases:
            with self.subTest(path=path, event=event, repository=repository, conclusion=conclusion):
                fixture = json.dumps({'run': {'path': path, 'event': event,
                                              'head_repository': {'full_name': repository}},
                                      'jobs': [{'name': 'runtime-companion / ' + publisher_name,
                                                'conclusion': conclusion}]})
                program = ('const fixture=' + fixture + '; const context={repo:{owner:"fixture",repo:"fixture"}}; '
                           'const github={rest:{actions:{getWorkflowRun:async()=>({data:fixture.run}),'
                           'listJobsForWorkflowRun:()=>{}}},paginate:async(method,args)=>{'
                           'if(args.filter!=="all")throw new Error('
                           '"publication proof must survive a failed-fanout retry");return fixture.jobs}}; '
                           '(async()=>{' + script
                           + '})().catch(error=>{console.error(error.message);process.exit(1)});')
                result = subprocess.run(['node', '-e', program], capture_output=True, text=True,
                                        env=dict(os.environ, COMPANION_RUN='123'))
                self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)

    def test_smoke_policy_cleanup_requires_no_execution_or_successful_drain(self):
        cleanup = next(step['run'] for step in steps(load('dem-runtime-release.yml')['jobs']['build'])
                       if step.get('name') == 'Retire smoke installation and its host allowance')
        for cache, executed, uninstall_status, success, removed in [
            (False, False, 0, True, True),
            (False, True, 0, False, False),
            (True, True, 0, True, True),
            (True, True, 23, False, False),
        ]:
            with (self.subTest(cache=cache, executed=executed, status=uninstall_status),
                  tempfile.TemporaryDirectory() as temporary):
                root = Path(temporary)
                binary = root / 'runtime-smoke/bin'
                binary.mkdir(parents=True)
                (root / 'runtime-policy-registered').touch()
                if executed:
                    (root / 'runtime-smoke-execution-attempted').touch()
                if cache:
                    (root / 'runtime-smoke/cache').mkdir()
                cli = binary / 'dem.plugin'
                cli.write_text('#!/bin/sh\nexit ' + str(uninstall_status) + '\n')
                cli.chmod(0o755)
                sudo = binary / 'sudo'
                sudo.write_text('#!/bin/sh\nprintf "%s\\n" "$*" > "$RUNNER_TEMP/policy-removed"\n')
                sudo.chmod(0o755)
                result = subprocess.run(['bash', '-c', cleanup], capture_output=True, text=True,
                                        env=dict(os.environ, RUNNER_TEMP=str(root), DEM_ARCH='amd64',
                                                 GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1',
                                                 PATH=str(binary) + os.pathsep + os.environ['PATH']))
                self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
                self.assertEqual((root / 'policy-removed').exists(), removed)

    def test_docker_builder_reconstructs_only_explicit_official_identity(self):
        dockerfile = (ROOT / 'packaging/docker/Dockerfile').read_text()
        run_commands = re.findall(r'(?m)^RUN ((?:[^\n]*\\\n)*[^\n]*)', dockerfile)
        script = next(command for command in run_commands if 'packaging/release-info' in command)
        for version, channel, source in [('v2.12.0', 'stable', 'a' * 40),
                                         ('v2.12.0-9-nightly', 'nightly', 'b' * 40),
                                         ('test', 'nightly', ''), ('', 'stable', 'c' * 40)]:
            with (self.subTest(version=version, channel=channel, source=source),
                  tempfile.TemporaryDirectory() as temporary):
                root = Path(temporary)
                (root / 'packaging').mkdir()
                result = subprocess.run(['sh', '-e', '-c', script], cwd=root, capture_output=True, text=True,
                                        env=dict(os.environ, BUILD_VERSION=version, RELEASE_CHANNEL=channel,
                                                 RELEASE_SOURCE=source))
                if source and not version:
                    self.assertNotEqual(result.returncode, 0)
                    continue
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                metadata = root / 'packaging/release-info'
                if source:
                    self.assertEqual(metadata.read_text(), f'version={version}\nchannel={channel}\nsource={source}\n')
                else:
                    self.assertFalse(metadata.exists(),
                                     'ordinary developer Docker builds have no public release identity')

    def test_embedded_shell_scripts_parse_without_running_release_actions(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'step.sh'
            for filename in ('release.yml', 'dem-runtime-release.yml', 'dem-runtime-companion-check.yml',
                             'build.yml', 'packaging.yml', 'docker.yml'):
                for job_name, job in load(filename)['jobs'].items():
                    windows = job.get('runs-on') == 'windows-latest'
                    for step in steps(job):
                        if 'run' not in step or (windows and step.get('shell') != 'bash'):
                            continue
                        # GitHub resolves expressions before the shell sees the script.
                        body = re.sub(r'\$\{\{.*?\}\}', 'fixture', step['run'], flags=re.S)
                        path.write_text(body)
                        checked = subprocess.run(['bash', '-n', str(path)], capture_output=True, text=True)
                        self.assertEqual(checked.returncode, 0,
                                         f'{filename}/{job_name}/{step.get("name", step.get("id"))}: {checked.stderr}')


if __name__ == '__main__':
    unittest.main()
