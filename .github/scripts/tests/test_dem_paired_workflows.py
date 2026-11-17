# SPDX-License-Identifier: GPL-3.0-or-later
"""Release lanes cannot publish a different checkout or omit their companion."""
from pathlib import Path
import ast
import fnmatch
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


def commands(job):
    return '\n'.join(step.get('run', '') for step in steps(job))


def job_can_run(jobs, name, results, event='workflow_dispatch', release_type='nightly',
                companion_run='123', repository='netdata/netdata', cancelled=False):
    """Evaluate this lane's job guards, including GitHub's ancestor success default.

    A successful intermediary does not clear a skipped ancestor. Explicit status
    functions override that default. The service-side needs wildcard can also
    include ancestors (actions/runner#2346); named needs select direct jobs.
    This models scheduling only; it does not execute or simulate job steps.
    """
    expression = jobs[name].get('if', 'true').removeprefix('${{').removesuffix('}}').strip()
    ancestors = [results[ancestor] for ancestor in dependencies(jobs, name)]
    if not re.search(r'\b(?:success|failure|cancelled|always)\s*\(', expression):
        if cancelled or any(result != 'success' for result in ancestors):
            return False
    status = {'always': True, 'success': all(result == 'success' for result in ancestors),
              'failure': 'failure' in ancestors, 'cancelled': cancelled or 'cancelled' in ancestors}
    expression = re.sub(r'\b(always|success|failure|cancelled)\(\)',
                        lambda match: repr(status[match[1]]), expression)
    expression = re.sub(r"contains\(needs\.\*\.result, '(\w+)'\)",
                        lambda match: repr(match[1] in ancestors), expression)
    expression = re.sub(r'needs\.([\w-]+)\.result', lambda match: repr(results[match[1]]), expression)
    for key, value in {'github.event_name': event, 'github.repository': repository,
                       'github.event.inputs.type': release_type, 'inputs.companion_run_id': companion_run}.items():
        expression = expression.replace(key, repr(value))
    expression = expression.replace('&&', ' and ').replace('||', ' or ')
    expression = re.sub(r'!(?!=)', ' not ', expression)
    expression = re.sub(r'\btrue\b', 'True', expression)

    def evaluate(node):
        if isinstance(node, ast.Constant):
            return node.value
        if isinstance(node, ast.BoolOp):
            values = [evaluate(value) for value in node.values]
            return all(values) if isinstance(node.op, ast.And) else any(values)
        if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.Not):
            return not evaluate(node.operand)
        if isinstance(node, ast.Compare) and len(node.ops) == 1:
            left, right = evaluate(node.left), evaluate(node.comparators[0])
            if isinstance(node.ops[0], ast.Eq):
                return left == right
            if isinstance(node.ops[0], ast.NotEq):
                return left != right
        raise AssertionError(f'unsupported job condition: {expression}')

    return bool(evaluate(ast.parse('(' + expression.strip() + ')', mode='eval').body))


class CoupledWorkflowTests(unittest.TestCase):
    def test_scheduler_model_preserves_skipped_ancestors_in_default_and_wildcard_guards(self):
        jobs = {'optional': {}, 'gate': {'needs': 'optional', 'if': 'always()'},
                'implicit': {'needs': 'gate'},
                'wildcard': {'needs': 'gate',
                             'if': "!cancelled() && !contains(needs.*.result, 'skipped')"},
                'explicit': {'needs': 'gate', 'if': "!cancelled() && needs.gate.result == 'success'"}}
        results = {'optional': 'skipped', 'gate': 'success'}
        self.assertTrue(job_can_run(jobs, 'gate', results))
        self.assertFalse(job_can_run(jobs, 'implicit', results))
        self.assertFalse(job_can_run(jobs, 'wildcard', results))
        self.assertTrue(job_can_run(jobs, 'explicit', results))

    def test_skipped_optional_publisher_preserves_build_and_release_lanes(self):
        ordinary = {
            'build.yml': {'build-dist', 'build-static', 'windows-build', 'windows-test', 'prepare-upload',
                          'artifact-verification-dist', 'artifact-verification-static',
                          'artifact-verification-updater'},
            'docker.yml': {'build-images'},
            'packaging.yml': {'build'},
        }
        for filename, builds in ordinary.items():
            jobs = load(filename)['jobs']
            descendants = {name for name in jobs if 'release-companion' in dependencies(jobs, name)}
            for event, release_type, companion_run, repository in [
                ('pull_request', '', '', 'netdata/netdata'), ('push', '', '', 'netdata/netdata'),
                ('workflow_dispatch', 'nightly', '123', 'netdata/netdata'),
                ('workflow_dispatch', 'release', '123', 'netdata/netdata'),
                ('workflow_dispatch', 'release-candidate', '123', 'netdata/netdata'),
                ('workflow_dispatch', 'nightly', '', 'netdata/netdata'),
                ('workflow_dispatch', 'release', '123', 'contributor/fork'),
            ]:
                with self.subTest(workflow=filename, event=event, release_type=release_type,
                                  reused_companion=bool(companion_run), repository=repository):
                    results = dict.fromkeys(jobs, 'success')
                    for name in jobs:
                        if name in descendants | {'runtime-companion', 'release-companion',
                                                  'gen-tags', 'normalize-tag'}:
                            results[name] = ('success' if job_can_run(
                                jobs, name, results, event, release_type, companion_run, repository) else 'skipped')
                    expected = set(builds) & descendants
                    if event == 'workflow_dispatch':
                        if filename == 'docker.yml':
                            expected = descendants
                        elif filename == 'build.yml' and repository == 'netdata/netdata':
                            expected |= {
                                'nightly': {'create-nightly', 'publish-nightly-artifacts'},
                                'release': {'upload-release', 'publish-stable-artifacts'},
                            }.get(release_type, set())
                    self.assertEqual({name for name in descendants if results[name] == 'success'}, expected)
                    self.assertEqual(results['release-companion'], 'success')
                    self.assertEqual(results['runtime-companion'],
                                     'success' if event == 'workflow_dispatch' and not companion_run else 'skipped')

    def test_companion_descendants_require_successful_prerequisites_and_no_cancellation(self):
        for filename in ('build.yml', 'docker.yml', 'packaging.yml'):
            jobs = load(filename)['jobs']
            for name, job in jobs.items():
                if 'release-companion' not in dependencies(jobs, name):
                    continue
                release_type = 'release' if name in {'upload-release', 'publish-stable-artifacts'} else 'nightly'
                results = dict.fromkeys(jobs, 'success')
                results['runtime-companion'] = 'skipped'
                with self.subTest(workflow=filename, job=name, cancelled=True):
                    self.assertFalse(job_can_run(jobs, name, results, release_type=release_type, cancelled=True))
                needs = job.get('needs', [])
                needs = [needs] if isinstance(needs, str) else needs
                for prerequisite in needs:
                    for outcome in ('failure', 'cancelled', 'skipped'):
                        with self.subTest(workflow=filename, job=name, prerequisite=prerequisite, outcome=outcome):
                            refused = dict(results, **{prerequisite: outcome})
                            self.assertFalse(job_can_run(jobs, name, refused, release_type=release_type))

    @unittest.skipUnless(shutil.which('git'), 'requires Git')
    def test_release_base_selects_canonical_stable_and_nightly_channels(self):
        for channel, release_type, version in [('stable', 'patch', 'v2.12.1'), ('nightly', 'nightly', 'nightly')]:
            with self.subTest(channel=channel), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                (root / 'packaging').mkdir()
                (root / 'packaging/version').write_text('v2.12.0\n')

                def git(*args):
                    subprocess.run(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                                    *args], cwd=root, check=True, capture_output=True)
                git('init', '-q', '-b', 'master')
                git('add', 'packaging/version')
                git('commit', '-qm', 'Fixture')
                git('tag', '-a', 'v2.12.0', '-m', 'Fixture')
                git('branch', 'v2.12')
                output = root / 'outputs'
                result = subprocess.run(['sh', str(ROOT / '.github/scripts/prepare-release-base.sh'),
                                         'netdata/netdata', 'workflow_dispatch', release_type, version, ''],
                                        cwd=root, env=dict(os.environ, GITHUB_OUTPUT=str(output)),
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                fields = dict(line.split('=', 1) for line in output.read_text().splitlines())
                self.assertEqual(fields['channel'], channel)
                self.assertEqual(fields['run'], 'true')
                self.assertEqual((root / 'packaging/version').read_text().strip(),
                                 'v2.12.0-1-nightly' if channel == 'nightly' else 'v2.12.1')

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
                                      'jobs': [{'name': 'runtime-companion / Publish the tested Agent companion',
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

    def test_release_freezes_post_commit_identity_before_all_lane_dispatches(self):
        workflow = load('release.yml')
        jobs = workflow['jobs']
        commit = next(step for step in steps(jobs['update-changelogs']) if step.get('id') == 'commit')['run']
        self.assertLess(commit.index('git commit'), commit.index('source_sha=$(git rev-parse HEAD)'))
        companion = jobs['runtime-companion']
        self.assertEqual(companion['with']['source_sha'], '${{ needs.update-changelogs.outputs.source_sha }}')
        self.assertIs(companion['with']['publish'], True)
        for name in ('trigger-artifacts', 'trigger-packages', 'trigger-docker'):
            with self.subTest(lane=name):
                self.assertIn('runtime-companion', dependencies(jobs, name))
                for outcome in ('failure', 'cancelled', 'skipped'):
                    results = dict.fromkeys(jobs, 'success')
                    results['runtime-companion'] = outcome
                    self.assertFalse(job_can_run(jobs, name, results, event='schedule'))
                dispatch = next(step for step in steps(jobs[name]) if 'workflow-dispatch@' in step.get('uses', ''))
                self.assertIn('"source_sha": "${{ needs.update-changelogs.outputs.source_sha }}"',
                              dispatch['with']['inputs'])
                self.assertIn('"companion_run_id": "${{ needs.runtime-companion.outputs.run_id }}"',
                              dispatch['with']['inputs'])
                self.assertEqual(dispatch['with']['ref'], '${{ needs.update-changelogs.outputs.ref }}')
        self.assertIs(workflow['concurrency']['cancel-in-progress'], False)

    def test_companion_has_no_independent_release_or_native_publication_owner(self):
        workflow = load('dem-runtime-release.yml')
        self.assertEqual(set(workflow['on']), {'workflow_call'})
        self.assertEqual(set(workflow['on']['workflow_call']['inputs']), {'source_sha', 'channel', 'publish'})
        jobs = workflow['jobs']
        self.assertIn('build', dependencies(jobs, 'publish'))
        self.assertIs(workflow['concurrency']['cancel-in-progress'], False)
        text = (WORKFLOWS / 'dem-runtime-release.yml').read_text()
        for retired in ('--sequence', 'upload-native', 'package_release', 'rpmsign', 'debsigs'):
            self.assertNotIn(retired, text)
        for job in jobs.values():
            for step in steps(job):
                if 'actions/checkout@' in step.get('uses', ''):
                    self.assertEqual(step['with']['ref'], '${{ inputs.source_sha }}')

    def test_companion_uses_normal_agent_release_access(self):
        publisher = load('dem-runtime-release.yml')['jobs']['publish']
        self.assertNotIn('environment', publisher)
        for filename in ('release.yml', 'build.yml', 'packaging.yml', 'docker.yml'):
            with self.subTest(workflow=filename):
                caller = load(filename)['jobs']['runtime-companion']
                self.assertEqual(caller['secrets'], 'inherit')

    def test_real_smoke_is_mandatory_for_both_architecture_artifacts(self):
        build = load('dem-runtime-release.yml')['jobs']['build']
        self.assertEqual(build['strategy']['matrix'], '${{ fromJson(needs.matrix.outputs.matrix) }}')
        build_steps = steps(build)
        smoke = next(step for step in build_steps if 'run real sandboxed browser workloads' in step.get('name', ''))
        self.assertNotIn('if', smoke)
        self.assertNotIn('continue-on-error', smoke)
        for proof in ('execution_contract', 'sandboxed_local_journey',
                      'sandboxed_local_lighthouse', 'process_tree_drained'):
            self.assertIn('.qualification.' + proof, smoke['run'])
        self.assertIn('runtime qualify', smoke['run'])
        self.assertIn('policy-prepare', smoke['run'])
        self.assertNotIn('--no-sandbox', smoke['run'])
        compile_step = next(step for step in build_steps if 'Compile the exact Agent CLI' in step.get('name', ''))
        self.assertIn('packaging/release-info', compile_step['run'])
        self.assertIn('ReleaseVersion=', compile_step['run'])
        self.assertIn('ReleaseRef=$SOURCE_SHA', compile_step['run'])
        upload = next(step for step in build_steps
                      if step.get('with', {}).get('name') == 'dem-runtime-${{ matrix.arch }}')
        self.assertGreater(build_steps.index(upload), build_steps.index(smoke))
        self.assertNotIn('if', upload)
        self.assertEqual(upload['with']['if-no-files-found'], 'error')
        self.assertIs(upload['with']['overwrite'], True)

    def test_public_exposure_and_native_consumption_require_published_companions(self):
        producers = {
            'build.yml': ('prepare-upload',
                          'create-nightly', 'publish-nightly-artifacts', 'upload-release', 'publish-stable-artifacts'),
            'packaging.yml': ('build',),
            'docker.yml': ('build-images-docker-hub', 'build-images-quay', 'build-images-ghcr',
                           'publish-docker-hub', 'publish-quay', 'publish-ghcr'),
        }
        gate = load('dem-runtime-companion-check.yml')['jobs']['check']
        self.assertIn('ci.py verify-publication-receipt', commands(gate))
        proof = next(step for step in steps(gate) if 'Require successful companion publication' in step.get('name', ''))
        self.assertIn('owners.includes(run.path)', proof['with']['script'])
        self.assertIn("job.conclusion === 'success'", proof['with']['script'])
        artifact = next(step for step in steps(gate) if 'actions/download-artifact@' in step.get('uses', ''))
        self.assertEqual(artifact['with']['run-id'], '${{ inputs.companion_run_id }}')
        self.assertEqual(artifact['with']['name'], 'dem-runtime-publication-receipt')
        self.assertNotIn('pattern', artifact['with'])
        for filename, names in producers.items():
            workflow = load(filename)
            inputs = workflow['on']['workflow_dispatch']['inputs']
            self.assertIs(inputs['source_sha']['required'], False)
            self.assertIs(inputs['companion_run_id']['required'], False)
            jobs = workflow['jobs']
            self.assertEqual(jobs['release-companion']['uses'], './.github/workflows/dem-runtime-companion-check.yml')
            self.assertEqual(jobs['release-companion']['with']['source_sha'], '${{ inputs.source_sha || github.sha }}')
            self.assertEqual(jobs['release-companion']['with']['companion_run_id'],
                             '${{ inputs.companion_run_id || needs.runtime-companion.outputs.run_id }}')
            fallback = jobs['runtime-companion']
            self.assertEqual(fallback['uses'], './.github/workflows/dem-runtime-release.yml')
            self.assertIn('!inputs.companion_run_id', fallback['if'])
            self.assertEqual(fallback['with']['source_sha'], '${{ inputs.source_sha || github.sha }}')
            self.assertIs(fallback['with']['publish'], True)
            self.assertIn('runtime-companion', dependencies(jobs, 'release-companion'))
            self.assertIn("needs.runtime-companion.result == 'skipped'", jobs['release-companion']['if'])
            self.assertIn("github.event_name != 'workflow_dispatch'", workflow['concurrency']['cancel-in-progress'])
            for name in names:
                with self.subTest(workflow=filename, producer=name):
                    self.assertIn('release-companion', dependencies(jobs, name))
            for job in jobs.values():
                for step in steps(job):
                    if 'actions/checkout@' in step.get('uses', '') and 'repository' not in step.get('with', {}):
                        self.assertEqual(step['with']['ref'], '${{ inputs.source_sha || github.sha }}')

    def test_compilation_and_internal_build_transport_can_precede_publication(self):
        for filename, names in {'build.yml': ('build-dist', 'build-static', 'windows-build', 'windows-test'),
                                'docker.yml': ('build-images',)}.items():
            jobs = load(filename)['jobs']
            for name in names:
                with self.subTest(workflow=filename, job=name):
                    self.assertNotIn('release-companion', dependencies(jobs, name))
                    results = dict.fromkeys(jobs, 'success')
                    results['runtime-companion'] = 'failure'
                    results['release-companion'] = 'failure'
                    self.assertTrue(job_can_run(jobs, name, results))
                    self.assertFalse(job_can_run(jobs, name, results, cancelled=True))

    def test_frozen_release_reuses_the_wire_test_owner(self):
        jobs = load('dem-runtime-release.yml')['jobs']
        wire = jobs['runtime-wire']
        self.assertEqual(wire['uses'], './.github/workflows/dem-runtime-node-tests.yml')
        self.assertEqual(wire['with']['source_sha'], '${{ inputs.source_sha }}')
        self.assertIn('runtime-wire', dependencies(jobs, 'build'))
        node = load('dem-runtime-node-tests.yml')['jobs']['node']
        checkout = next(step for step in steps(node) if 'actions/checkout@' in step.get('uses', ''))
        self.assertEqual(checkout['with']['ref'], '${{ inputs.source_sha || github.sha }}')
        self.assertIn('npm test', commands(node))

    def test_heavy_companion_triggers_exclude_docs_metadata_and_unit_tests(self):
        check = next(step for step in steps(load('build.yml')['jobs']['file-check'])
                     if step.get('id') == 'check-runtime-files')['with']

        def matches(path):
            return (any(fnmatch.fnmatchcase(path, pattern) for pattern in check['files'].split()) and
                    not any(fnmatch.fnmatchcase(path, pattern) for pattern in check['files_ignore'].split()))
        for path in ('packaging/dem-runtime/README.md', 'packaging/dem-runtime/test_release.py',
                     'packaging/dem-runtime/native/tests/test_native.py',
                     'src/go/cmd/demplugin/runtime_test.go', 'src/go/plugin/dem/synthetic/metadata.yaml',
                     'src/dem-runtime/tests/wire.test.mjs'):
            with self.subTest(path=path):
                self.assertFalse(matches(path))
        for path in ('packaging/dem-runtime/locks/amd64.json', 'packaging/dem-runtime/build.py',
                     'src/dem-runtime/package-lock.json', 'src/dem-runtime/runner/run.mjs',
                     'src/go/cmd/demplugin/runtime.go', 'src/collectors/utils/nd-run.c'):
            with self.subTest(path=path):
                self.assertTrue(matches(path))
        for filename in ('build.yml', 'docker.yml', 'packaging.yml'):
            for step in steps(load(filename)['jobs']['file-check']):
                if step.get('id') in ('check-build-files', 'check-docker-files'):
                    self.assertNotIn('packaging/dem-runtime/**', step['with']['files'])

    def test_manual_channels_follow_existing_agent_destinations(self):
        build = load('build.yml')['jobs']['runtime-companion']['with']['channel']
        native = load('packaging.yml')['jobs']['runtime-companion']['with']['channel']
        docker = load('docker.yml')['jobs']['runtime-companion']['with']['channel']
        self.assertIn("inputs.type == 'release-candidate'", build)
        self.assertNotIn('release-candidate', native)
        self.assertIn("inputs.type == 'nightly' && 'nightly' || 'devel'", native)
        self.assertEqual(docker, "${{ inputs.version == 'nightly' && 'nightly' || 'stable' }}")
        gate = load('dem-runtime-companion-check.yml')
        self.assertEqual(gate['on']['workflow_call']['outputs']['run_id']['value'], '${{ jobs.check.outputs.run_id }}')
        self.assertEqual(gate['jobs']['check']['outputs']['run_id'], '${{ inputs.companion_run_id }}')
        for filename in ('build.yml', 'packaging.yml', 'docker.yml'):
            for job in load(filename)['jobs'].values():
                for step in steps(job):
                    if 'packaging/release_info.py' in step.get('run', ''):
                        self.assertEqual(step['env']['SOURCE_SHA'], '${{ inputs.source_sha || github.sha }}')

    def test_small_receipt_is_emitted_only_after_successful_publication(self):
        publisher = steps(load('dem-runtime-release.yml')['jobs']['publish'])
        stamp = next(step for step in publisher if step.get('name') == 'Stamp the exact Agent identity')
        self.assertEqual(stamp['env']['SOURCE_SHA'], '${{ inputs.source_sha }}')
        self.assertIn('packaging/release_info.py', stamp['run'])
        published = next(step for step in publisher if 'Publish immutable payloads' in step.get('name', ''))
        receipt = next(step for step in publisher
                       if step.get('with', {}).get('name') == 'dem-runtime-publication-receipt')
        self.assertGreater(publisher.index(receipt), publisher.index(published))
        self.assertNotIn('if', receipt)
        self.assertNotIn('continue-on-error', receipt)
        self.assertIs(receipt['with']['overwrite'], True)
        prepared = next(step['run'] for step in publisher
                        if step.get('name') == 'Prepare the exact publication receipt')
        prepared_step = next(step for step in publisher if step.get('name') == 'Prepare the exact publication receipt')
        self.assertLess(publisher.index(stamp), publisher.index(prepared_step))
        self.assertIn('packaging/release-info', prepared)
        self.assertIn('artifacts.json', prepared)
        self.assertNotIn('.tar.zst', prepared)

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

    def test_native_admission_and_docker_context_carry_exact_metadata(self):
        native = load('packaging.yml')['jobs']['build']
        native_steps = steps(native)
        prepare = next(step for step in native_steps if 'ci.py prepare-native' in step.get('run', ''))
        build = next(step for step in native_steps if step.get('name') == 'Build Packages')
        self.assertLess(native_steps.index(prepare), native_steps.index(build))
        self.assertIn('--release-info packaging/release-info', prepare['run'])
        self.assertIn('DEM_RUNTIME_PAYLOAD_DIR=/netdata/.dem-runtime-payload', prepare['run'])
        self.assertIn('-e DEM_RUNTIME_PAYLOAD_DIR=', build['run'])
        download = next(step for step in native_steps if step.get('name') == 'Download the exact paired native payload')
        self.assertEqual(download['with']['run-id'], '${{ needs.release-companion.outputs.run_id }}')
        receipt = next(step for step in native_steps
                       if step.get('name') == 'Download the successful companion publication receipt')
        self.assertEqual(receipt['with']['run-id'], download['with']['run-id'])
        self.assertEqual(receipt['with']['name'], 'dem-runtime-publication-receipt')
        self.assertIn('--receipt "$RUNNER_TEMP/native-runtime-receipt"', prepare['run'])
        # The ignore file is a symlink: Docker must reconstruct explicit identity.
        self.assertEqual((ROOT / '.dockerignore').resolve(), ROOT / '.gitignore')
        self.assertIn('/packaging/release-info', (ROOT / '.dockerignore').read_text().splitlines())
        for job in load('docker.yml')['jobs'].values():
            for step in steps(job):
                if 'docker/build-push-action@' in step.get('uses', ''):
                    self.assertEqual(step['with']['context'], '.')
                    self.assertIn('RELEASE_CHANNEL=', step['with']['build-args'])
                    self.assertIn('RELEASE_SOURCE=${{ env.NETDATA_RELEASE_SOURCE }}', step['with']['build-args'])
                    self.assertIn('BUILD_VERSION=${{ env.NETDATA_BUILD_VERSION ||', step['with']['build-args'])
                    stamp = next(item for item in steps(job) if 'packaging/release_info.py' in item.get('run', ''))
                    self.assertLess(steps(job).index(stamp), steps(job).index(step))
                    self.assertIn('NETDATA_RELEASE_SOURCE=$SOURCE_SHA', stamp['run'])

    def test_docker_builder_reconstructs_only_explicit_official_identity(self):
        dockerfile = (ROOT / 'packaging/docker/Dockerfile').read_text()
        self.assertIn('ARG BUILD_VERSION', dockerfile.split(' AS base')[0])
        self.assertIn('ARG RELEASE_SOURCE', dockerfile.split(' AS base')[0])
        fragment = re.search(r'(?m)^RUN (if \[ -n "\$RELEASE_SOURCE".*?^    fi)$', dockerfile, re.S)
        self.assertIsNotNone(fragment)
        script = fragment.group(1)
        self.assertLess(dockerfile.index(fragment.group(0)), dockerfile.index('# Install from source'))
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
