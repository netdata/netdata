#!/usr/bin/env python3

import fnmatch
import re
import unittest
from pathlib import Path, PurePosixPath


ROOT = Path(__file__).resolve().parents[2]
REVIEW_WORKFLOW = ROOT / ".github/workflows/review.yml"
GO_TESTS_WORKFLOW = ROOT / ".github/workflows/go-tests.yml"
BUILD_WORKFLOW_STEPS = (
    (ROOT / ".github/workflows/build.yml", "Check build files"),
    (ROOT / ".github/workflows/docker.yml", "Check build system files"),
)

SHELL_PATH_RE = re.compile(r"\.sh(?:\.in)?$")
SHELL_FIND_PATTERNS = ("*.sh", "*.sh.in")
BROAD_GO_PATH_PATTERNS = (
    "*.go",
    "go.mod",
    "go.sum",
    "**/*.go",
    "**/go.mod",
    "**/go.sum",
)


def shell_trigger_matches(path: str) -> bool:
    return SHELL_PATH_RE.search(path) is not None


def shell_scanner_matches(path: str) -> bool:
    name = PurePosixPath(path).name
    return any(fnmatch.fnmatchcase(name, pattern) for pattern in SHELL_FIND_PATTERNS)


def workflow_step(workflow: str, name: str) -> list[str]:
    lines = workflow.splitlines()
    marker = f"- name: {name}"
    starts = [index for index, line in enumerate(lines) if line.strip() == marker]

    if len(starts) != 1:
        raise ValueError(f"expected one {name!r} step, found {len(starts)}")

    start = starts[0]
    end = next(
        (
            index
            for index in range(start + 1, len(lines))
            if lines[index].strip().startswith("- name: ")
        ),
        len(lines),
    )
    return lines[start:end]


def literal_block(step: list[str], key: str) -> set[str]:
    marker = f"{key}: |"
    starts = [index for index, line in enumerate(step) if line.strip() == marker]

    if len(starts) != 1:
        raise ValueError(f"expected one {key!r} block, found {len(starts)}")

    start = starts[0]
    indentation = len(step[start]) - len(step[start].lstrip())
    values = []
    for line in step[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) <= indentation:
            break
        if line.strip():
            values.append(line.strip())

    return set(values)


class WorkflowFileSelectionTest(unittest.TestCase):
    def test_literal_block_ignores_indentation_and_blank_lines(self) -> None:
        workflow = """
  - name: Example
    with:
      files: |
        *.go

        go.mod
  - name: Next step
    run: true
"""

        step = workflow_step(workflow, "Example")
        self.assertEqual(literal_block(step, "files"), {"*.go", "go.mod"})

    def test_shell_trigger_and_scanner_accept_only_supported_suffixes(self) -> None:
        accepted = (
            "install.sh",
            ".github/scripts/check.sh",
            "packaging/templates/installer.sh.in",
        )
        rejected = (
            "src/collectors/freebsd.plugin/integrations/kern.ipc.shm.md",
            "packaging/checksums.sha256",
            "docs/example.sh.md",
            "scripts/shellscript",
        )

        for path in accepted:
            with self.subTest(path=path):
                self.assertTrue(shell_trigger_matches(path))
                self.assertTrue(shell_scanner_matches(path))

        for path in rejected:
            with self.subTest(path=path):
                self.assertFalse(shell_trigger_matches(path))
                self.assertFalse(shell_scanner_matches(path))

    def test_large_documentation_change_does_not_select_shellcheck(self) -> None:
        paths = (
            f"src/collectors/example/integrations/metric-{index}.shm.md"
            for index in range(20_000)
        )
        self.assertFalse(any(shell_trigger_matches(path) for path in paths))

    def test_workflows_use_the_tested_shell_contract(self) -> None:
        workflow = REVIEW_WORKFLOW.read_text(encoding="utf-8")
        shellcheck_step = workflow_step(workflow, "Run shellcheck")

        self.assertIn(r"grep -Eq '\.sh(\.in)?$'", workflow)
        self.assertEqual(literal_block(shellcheck_step, "pattern"), {"*.sh", "*.sh.in"})
        self.assertNotIn('pattern: "*.sh*"', workflow)

    def test_build_decisions_avoid_broad_go_triggers_and_filename_inventories(self) -> None:
        for workflow_path, build_step_name in BUILD_WORKFLOW_STEPS:
            workflow = workflow_path.read_text(encoding="utf-8")
            build_files_step = workflow_step(workflow, build_step_name)
            check_run_step = workflow_step(workflow, "Check Run")
            check_go_step = workflow_step(workflow, "Check Go")

            with self.subTest(workflow=workflow_path.name):
                self.assertTrue(
                    any("steps.check-build-files.outputs.any_modified" in line for line in check_run_step)
                )
                self.assertTrue(
                    any("steps.check-build-files.outputs.any_modified" in line for line in check_go_step)
                )
                self.assertNotIn("other_changed_files", workflow)
                self.assertTrue(
                    set(BROAD_GO_PATH_PATTERNS).isdisjoint(literal_block(build_files_step, "files"))
                )

    def test_go_workflow_owns_agent_go_validation(self) -> None:
        workflow = GO_TESTS_WORKFLOW.read_text(encoding="utf-8")
        check_files_step = workflow_step(workflow, "Check files")

        self.assertTrue(
            {
                "src/go/**",
                "src/collectors/cgroups.plugin/cgroup-name/**",
                "src/collectors/ebpf.plugin/ebpfgo.plugin/**",
            }
            <= literal_block(check_files_step, "files")
        )
        self.assertIn("CGO_ENABLED=0 go build", workflow)
        self.assertIn("go test -json ./... -race", workflow)
        self.assertIn("name: Go build tests", workflow)


if __name__ == "__main__":
    unittest.main()
