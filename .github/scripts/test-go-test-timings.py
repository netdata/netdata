#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / ".github/scripts/summarize-go-test-json.py"
WORKFLOW = ROOT / ".github/workflows/go-tests.yml"
SPEC = importlib.util.spec_from_file_location("go_test_timings", SCRIPT)
REPORTER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REPORTER)


def job_steps(job):
    lines = WORKFLOW.read_text(encoding="utf-8").splitlines()
    start = lines.index(f"  {job}:")
    end = next((i for i in range(start + 1, len(lines))
                if lines[i].startswith("  ") and not lines[i].startswith("   ") and lines[i].strip()), len(lines))
    steps = {}
    for line in lines[start:end]:
        if line.startswith("      - name: "):
            name = line.split("- name: ", 1)[1]
            steps[name] = []
        elif steps:
            steps[name].append(line)
    return steps


def run_script(step):
    start = step.index("        run: |") + 1
    end = next((i for i in range(start, len(step))
                if step[i].strip() and not step[i].startswith("          ")), len(step))
    return textwrap.dedent("\n".join(step[start:end]))


class TimingReportTest(unittest.TestCase):
    def test_mixed_partial_and_failed_run(self):
        events = [
            "compiler diagnostic\n",
            json.dumps({"Action": "build-output", "ImportPath": "example/broken", "Output": "failure"}),
            json.dumps({"Action": "run", "Package": "example/pending", "Test": "TestPending"}),
            json.dumps({"Action": "pass", "Package": "example/fast", "Elapsed": 1.25}),
            json.dumps({"Action": "fail", "Package": "example/slow", "Elapsed": 9}),
            json.dumps({"Action": "skip", "Package": "example/skipped"}),
            json.dumps({"Action": "fail", "Package": "example/slow", "Test": "TestParent", "Elapsed": 8}),
            json.dumps({"Action": "fail", "Package": "example/slow", "Test": "TestParent/child", "Elapsed": 7}),
            json.dumps({"Action": "pass", "Package": "example/fast", "Test": "TestFast", "Elapsed": 0.5}),
        ]
        report = REPORTER.summarize(events, "src/go")
        self.assertIn("Package results: 1 passed, 1 failed, 1 skipped.", report)
        self.assertIn("| 9.000 | fail | example/slow |", report)
        self.assertIn("| 8.000 | fail | example/slow/TestParent |", report)
        self.assertLess(report.index("example/slow |"), report.index("example/fast |"))
        self.assertNotIn("TestParent/child", report)
        self.assertNotIn("TestPending", report)
        self.assertIn("Non-JSON diagnostic lines in the raw log: 1.", report)
        self.assertIn("Package times overlap", report)

    def test_empty_and_incomplete_logs(self):
        for events in ([], ["\n"], [json.dumps({"Action": "start", "Package": "example/pending"})]):
            with self.subTest(events=events):
                report = REPORTER.summarize(events, "suite")
                self.assertEqual(report.count("No completed timings were recorded."), 2)
                self.assertIn("0 passed, 0 failed, 0 skipped", report)

    def test_limits_rankings_and_escapes_markdown(self):
        events = [json.dumps({"Action": "pass", "Package": f"example/p{i}", "Elapsed": i})
                  for i in range(25)]
        events.append(json.dumps({"Action": "pass", "Package": "example/<p|`>", "Elapsed": 30}))
        report = REPORTER.summarize(events, "suite|<test>")
        rows = [line for line in report.splitlines() if line.startswith("| ") and " | pass | " in line]
        self.assertEqual(len(rows), 20)
        self.assertIn("26 passed", report)
        self.assertIn("suite&#124;&lt;test&gt;", report)
        self.assertIn("example/&lt;p&#124;&#96;&gt;", rows[0])
        self.assertNotIn("example/p0 |", report)

    def test_ignores_non_events_and_invalid_timings(self):
        events = [json.dumps(value) for value in [[], None, "text", {},
                  {"Action": "pass", "Package": "example/nan", "Elapsed": float("nan")},
                  {"Action": "pass", "Package": "example/negative", "Elapsed": -1},
                  {"Action": "pass", "Package": "example/bool", "Elapsed": True}]]
        report = REPORTER.summarize(events, "suite")
        self.assertEqual(report.count("No completed timings were recorded."), 2)

    def test_cli_reads_without_changing_log(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "go-test.jsonl"
            raw = b'diagnostic\xff\n{"Action":"pass","Package":"example/p","Elapsed":2}\n'
            path.write_bytes(raw)
            result = subprocess.run([sys.executable, str(SCRIPT), str(path), "--label", "CLI suite"],
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("| 2.000 | pass | example/p |", result.stdout)
            self.assertEqual(path.read_bytes(), raw)


class WorkflowTimingTest(unittest.TestCase):
    def test_actual_pipelines_preserve_raw_output_and_failure(self):
        raw = (b'compiler diagnostic\n'
               b'{"Action":"output","Output":"tool output\\n"}\n'
               b'{"Action":"pass","Package":"example/p","Test":"TestWork","Elapsed":0.5}\n'
               b'{"Action":"pass","Package":"example/p","Elapsed":1.5}\n')
        suites = {
            "tests": ("Go test", ["-json", "./...", "-race", "-count=1"]),
            "ebpf-libbpf-tests": ("Go test (libbpf tag)",
                                  ["-tags", "netdata_ebpf_libbpf netdata_ebpf_test", "-json", "./...",
                                   "-race", "-count=1"]),
        }
        for job, (step_name, expected_args) in suites.items():
            for status in (0, 7):
                with self.subTest(job=job, status=status), tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    (root / "fixture").write_bytes(raw)
                    fake_go = root / "go"
                    fake_go.write_text(f"#!{sys.executable}\n" + textwrap.dedent("""\
                        import json, os, sys
                        from pathlib import Path
                        Path(os.environ['GO_TEST_ARGS']).write_text(json.dumps(sys.argv[1:]))
                        sys.stdout.buffer.write(Path(os.environ['GO_TEST_FIXTURE']).read_bytes())
                        sys.exit(int(os.environ['GO_TEST_EXIT_CODE']))
                    """), encoding="utf-8")
                    fake_go.chmod(0o755)
                    formatter = root / "gotestfmt"
                    formatter.write_text(f"#!{sys.executable}\n" + textwrap.dedent("""\
                        import json, sys
                        for line in sys.stdin:
                            assert json.loads(line)['Package']
                    """), encoding="utf-8")
                    formatter.chmod(0o755)
                    env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                               RUNNER_TEMP=str(root), GITHUB_WORKSPACE=str(ROOT),
                               GITHUB_STEP_SUMMARY=str(root / "summary.md"), SUITE=job,
                               GO_TEST_FIXTURE=str(root / "fixture"), GO_TEST_ARGS=str(root / "args.json"),
                               GO_TEST_EXIT_CODE=str(status))
                    steps = job_steps(job)
                    result = subprocess.run(["bash", "-c", run_script(steps[step_name])],
                                            env=env, text=True, capture_output=True)
                    self.assertEqual(result.returncode, status, result.stderr)
                    self.assertEqual((root / "go-test.jsonl").read_bytes(), raw)
                    self.assertEqual(json.loads((root / "args.json").read_text()), ["test", *expected_args])
                    report = subprocess.run(["bash", "-e", "-c", run_script(steps["Summarize Go test timings"])],
                                            cwd=ROOT, env=env, text=True, capture_output=True)
                    self.assertEqual(report.returncode, 0, report.stderr)
                    self.assertEqual((root / "summary.md").read_bytes(),
                                     (root / "go-test-timings.md").read_bytes())
                    self.assertIn("example/p/TestWork", (root / "summary.md").read_text())

    def test_cache_ordering_and_namespace_separation(self):
        purposes = {"tests": "${{ matrix.module }}-native-race-",
                    "ebpf-libbpf-tests": "src/collectors/ebpf.plugin/ebpfgo.plugin-libbpf-race-",
                    "build-tests": "${{ matrix.module }}-build-${{ matrix.platform }}-"}
        for job, purpose in purposes.items():
            with self.subTest(job=job):
                steps = job_steps(job)
                names = list(steps)
                self.assertLess(names.index("Checkout"), names.index("Install Go"))
                self.assertLess(names.index("Install Go"), names.index("Cache Go modules"))
                self.assertLess(names.index("Install Go"), names.index("Cache Go build outputs"))
                self.assertIn("          cache: false", steps["Install Go"])
                for name, path in (("Cache Go modules", "~/go/pkg/mod"),
                                   ("Cache Go build outputs", "~/.cache/go-build")):
                    cache = steps[name]
                    key = next(line.strip()[5:] for line in cache if line.strip().startswith("key: "))
                    fallback = cache[cache.index("          restore-keys: |") + 1].strip()
                    self.assertTrue(key.startswith(fallback))
                    if name == "Cache Go build outputs":
                        self.assertTrue(fallback.endswith(purpose))
                    else:
                        module = ("src/collectors/ebpf.plugin/ebpfgo.plugin"
                                  if job == "ebpf-libbpf-tests" else "${{ matrix.module }}")
                        self.assertTrue(fallback.endswith(module + "-"))
                    self.assertIn("${{ runner.arch }}", fallback)
                    self.assertIn("${{ steps.go.outputs.go-version }}", fallback)
                    self.assertIn("hashFiles(", key)
                    self.assertIn("/go.mod", key)
                    self.assertIn("/go.sum", key)
                    self.assertIn("            " + path, cache)

    def test_reporting_covers_success_and_failure_with_unique_artifacts(self):
        artifact_names = []
        for job in ("tests", "ebpf-libbpf-tests"):
            steps = job_steps(job)
            for name in ("Summarize Go test timings", "Upload Go test timings"):
                condition = next(line.strip() for line in steps[name] if line.strip().startswith("if:"))
                self.assertIn("!cancelled()", condition)
                self.assertIn("steps.go-test.outcome == 'success'", condition)
                self.assertIn("steps.go-test.outcome == 'failure'", condition)
            upload = steps["Upload Go test timings"]
            artifact_names.append(next(line.strip()[6:] for line in upload if line.strip().startswith("name: ")))
            self.assertIn("          retention-days: 7", upload)
            self.assertIn("            ${{ runner.temp }}/go-test.jsonl", upload)
            self.assertIn("            ${{ runner.temp }}/go-test-timings.md", upload)
        self.assertNotEqual(*artifact_names)
        self.assertIn("${{ strategy.job-index }}", artifact_names[0])
        self.assertIn("        run: python3 .github/scripts/test-go-test-timings.py",
                      job_steps("matrix")["Test Go test timing reports"])


if __name__ == "__main__":
    unittest.main()
