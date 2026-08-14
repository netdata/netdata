#!/usr/bin/env python3

import fnmatch
import re
import unittest
from pathlib import Path, PurePosixPath


ROOT = Path(__file__).resolve().parents[2]
REVIEW_WORKFLOW = ROOT / ".github/workflows/review.yml"
GO_WORKFLOWS = (
    ROOT / ".github/workflows/build.yml",
    ROOT / ".github/workflows/docker.yml",
)

SHELL_PATH_RE = re.compile(r"\.sh(?:\.in)?$")
SHELL_FIND_PATTERNS = ("*.sh", "*.sh.in")
GO_PATH_PATTERNS = (
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


def go_group_matches(path: str) -> bool:
    candidate = PurePosixPath(path)
    return any(candidate.match(pattern) for pattern in GO_PATH_PATTERNS)


class WorkflowFileSelectionTest(unittest.TestCase):
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

    def test_go_group_covers_root_and_nested_go_inputs(self) -> None:
        accepted = (
            "main.go",
            "go.mod",
            "go.sum",
            "src/go/main.go",
            "src/go/go.mod",
            "src/go/go.sum",
        )
        rejected = (
            "main.go.md",
            "docs/go.mod.md",
            "src/generated/golang.txt",
        )

        for path in accepted:
            with self.subTest(path=path):
                self.assertTrue(go_group_matches(path))

        for path in rejected:
            with self.subTest(path=path):
                self.assertFalse(go_group_matches(path))

    def test_workflows_use_the_tested_shell_contract(self) -> None:
        workflow = REVIEW_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(r"grep -Eq '\.sh(\.in)?$'", workflow)
        self.assertIn("pattern: |\n            *.sh\n            *.sh.in", workflow)
        self.assertNotIn('pattern: "*.sh*"', workflow)

    def test_go_decisions_do_not_consume_filename_inventories(self) -> None:
        for workflow_path in GO_WORKFLOWS:
            workflow = workflow_path.read_text(encoding="utf-8")

            with self.subTest(workflow=workflow_path.name):
                self.assertIn("id: check-go-files", workflow)
                self.assertIn("steps.check-go-files.outputs.any_modified", workflow)
                self.assertNotIn("other_changed_files", workflow)
                for pattern in GO_PATH_PATTERNS:
                    self.assertIn(f"            {pattern}\n", workflow)


if __name__ == "__main__":
    unittest.main()
