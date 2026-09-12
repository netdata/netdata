"""Black-box source snapshot contracts, using disposable local Git repositories."""

import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest


HELPER = Path(__file__).resolve().parents[1] / "scripts" / "snapshot-source.py"


class SnapshotTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="preview-fixture-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
        self.repo = self.root / "source repo"
        self.repo.mkdir()
        self.git("init", "-q")
        self.git("config", "user.name", "Fixture Author")
        self.git("config", "user.email", "fixture@example.com")
        self.git("config", "core.autocrlf", "false")
        self.write("page.md", b"base page\n")
        self.write("deleted.md", b"base deletion candidate\n")
        self.git("add", ".")
        self.git("commit", "-qm", "base")
        self.base = self.git("rev-parse", "HEAD").strip()
        self.output = self.root / "snapshot"

    def git(self, *args):
        return subprocess.check_output(
            ["git", "-C", str(self.repo), *args], stderr=subprocess.PIPE, env=self.env
        ).decode()

    def write(self, name, data):
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return path

    def commit(self):
        self.git("add", ".")
        self.git("commit", "-qm", "fixture change")
        return self.git("rev-parse", "HEAD").strip()

    @staticmethod
    def tree_state(tree):
        # Include .git bytes and modes: source index, refs and config must not change.
        result = {}
        for directory, dirs, files in os.walk(tree, followlinks=False):
            for name in dirs + files:
                path = Path(directory) / name
                mode = path.lstat().st_mode
                data = (os.fsencode(os.readlink(path)) if path.is_symlink()
                        else path.read_bytes() if stat.S_ISREG(mode) else None)
                result[str(path.relative_to(tree))] = (mode, data)
        return result

    def run_helper(self, *args, success=True, env=None):
        before = self.tree_state(self.repo)
        result = subprocess.run(
            [sys.executable, str(HELPER), *map(str, args)],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=self.env if env is None else env,
        )
        self.assertEqual(self.tree_state(self.repo), before, "helper mutated source")
        if success:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def snapshot(self, *selection, success=True):
        return self.run_helper("--repo", self.repo, "--output", self.output,
                               *selection, success=success)

    @property
    def manifest_path(self):
        return self.output.with_name(self.output.name + ".manifest.json")

    def manifest(self, mode, commit):
        raw = self.manifest_path.read_text()
        self.assertNotIn(str(self.root), raw)
        self.assertNotIn("https://", raw)
        manifest = json.loads(raw)
        self.assertEqual(manifest["mode"], mode)
        self.assertEqual(manifest["commit"], commit)
        actual = {}
        for name, (mode_bits, data) in self.tree_state(self.output).items():
            if data is None:
                continue
            mode_bits = ("120000" if stat.S_ISLNK(mode_bits) else
                         "100755" if mode_bits & stat.S_IXUSR else "100644")
            actual[name] = {"sha256": hashlib.sha256(data).hexdigest(), "mode": mode_bits}
        self.assertEqual(manifest["files"], actual)
        return manifest

    def test_caller_git_environment_does_not_select_snapshot_source(self):
        foreign = self.root / "foreign repo"
        self.git("clone", "-q", "--no-hardlinks", str(self.repo), str(foreign))
        empty_index = self.root / "empty-index"
        subprocess.run(["git", "-C", str(foreign), "read-tree", "--empty"],
                       check=True, env=dict(self.env, GIT_INDEX_FILE=str(empty_index)))
        empty_objects = self.root / "empty-objects"
        empty_objects.mkdir()
        overrides = [
            {"GIT_INDEX_FILE": str(empty_index)},
            {"GIT_DIR": str(foreign / ".git"), "GIT_WORK_TREE": str(foreign),
             "GIT_COMMON_DIR": str(foreign / ".git")},
            {"GIT_OBJECT_DIRECTORY": str(empty_objects),
             "GIT_ALTERNATE_OBJECT_DIRECTORIES": str(empty_objects)},
            {"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "invalid-key", "GIT_CONFIG_VALUE_0": "unused"},
            {"GIT_CONFIG_PARAMETERS": "'invalid-key=unused'"},
        ]
        self.write("page.md", b"selected commit\n")
        selected_commit = self.commit()
        self.write("page.md", b"selected working content\n")
        for index, override in enumerate(overrides):
            for working in (False, True):
                with self.subTest(override=override, working=working):
                    self.output = self.root / f"environment-{index}-{working}"
                    before = self.tree_state(self.root)
                    self.run_helper("--repo", self.repo, "--output", self.output,
                                    *(("--working-tree",) if working else ("--ref", "HEAD")),
                                    env=dict(self.env, **override))
                    after = self.tree_state(self.root)
                    for path, value in before.items():
                        self.assertEqual(after.get(path), value, path)
                    manifest = self.manifest("working-tree" if working else "ref", selected_commit)
                    self.assertEqual(set(manifest["files"]), {"page.md", "deleted.md"})
                    expected = b"selected working content\n" if working else b"selected commit\n"
                    self.assertEqual((self.output / "page.md").read_bytes(), expected)

    def test_linked_worktree_uses_its_own_head_index_and_files(self):
        primary = self.repo
        linked = self.root / "linked worktree"
        self.git("worktree", "add", "-q", "--detach", str(linked), self.base)
        self.repo = linked
        self.write("page.md", b"linked commit\n")
        linked_commit = self.commit()
        self.write("page.md", b"linked working content\n")
        self.write("staged.md", b"linked staged addition\n")
        self.git("add", "staged.md")
        override = dict(self.env, GIT_DIR=str(primary / ".git"), GIT_WORK_TREE=str(primary),
                        GIT_COMMON_DIR=str(primary / ".git"), GIT_INDEX_FILE=str(primary / ".git/index"),
                        GIT_CONFIG_COUNT="1", GIT_CONFIG_KEY_0="core.bare", GIT_CONFIG_VALUE_0="true")
        for working in (False, True):
            with self.subTest(working=working):
                self.output = self.root / f"linked-{working}"
                before = self.tree_state(self.root)
                self.run_helper("--repo", linked, "--output", self.output,
                                *(("--working-tree",) if working else ("--ref", "HEAD")), env=override)
                after = self.tree_state(self.root)
                for path, value in before.items():
                    self.assertEqual(after.get(path), value, path)
                manifest = self.manifest("working-tree" if working else "ref", linked_commit)
                self.assertEqual(set(manifest["files"]),
                                 {"page.md", "deleted.md", "staged.md"} if working else {"page.md", "deleted.md"})
                expected = b"linked working content\n" if working else b"linked commit\n"
                self.assertEqual((self.output / "page.md").read_bytes(), expected)

    def test_requested_commit_ignores_checkout_dirty_and_untracked(self):
        self.write("page.md", b"later commit\n")
        self.commit()
        self.write("page.md", b"dirty checkout\n")
        self.write("private.txt", b"unselected\n")
        self.snapshot("--ref", self.base)
        self.assertEqual((self.output / "page.md").read_bytes(), b"base page\n")
        self.assertFalse((self.output / "private.txt").exists())
        manifest = self.manifest("ref", self.base)
        self.assertEqual(set(manifest["files"]), {"page.md", "deleted.md"})
        self.assertEqual(manifest["selected_untracked"], [])
        self.assertEqual(manifest["missing_tracked"], [])

    def test_working_tree_selection_and_deletions(self):
        self.write("page.md", b"tracked edit\n")
        (self.repo / "deleted.md").unlink()
        self.write("staged.md", b"staged addition\n")
        self.git("add", "staged.md")
        self.write("staged.md", b"current staged addition\n")
        self.write("notes/z.md", b"explicit z\n")
        self.write("notes/a.md", b"explicit a\n")
        self.write("private.txt", b"not selected\n")
        self.snapshot("--working-tree", "--include-untracked", "notes/z.md",
                      "--include-untracked", "notes/a.md")
        manifest = self.manifest("working-tree", self.base)
        self.assertEqual(set(manifest["files"]), {"page.md", "staged.md", "notes/a.md", "notes/z.md"})
        self.assertEqual(manifest["selected_untracked"], ["notes/a.md", "notes/z.md"])
        self.assertEqual(manifest["missing_tracked"], ["deleted.md"])
        self.assertEqual((self.output / "page.md").read_bytes(), b"tracked edit\n")
        self.assertEqual((self.output / "staged.md").read_bytes(), b"current staged addition\n")

    def test_staged_deletion_is_absent_from_snapshot_and_missing_indexed_list(self):
        self.git("rm", "deleted.md")
        self.snapshot("--working-tree")
        manifest = self.manifest("working-tree", self.base)
        self.assertFalse((self.output / "deleted.md").exists())
        self.assertNotIn("deleted.md", manifest["files"])
        self.assertEqual(manifest["missing_tracked"], [])

    def test_exact_blobs_links_modes_and_unusual_names(self):
        name = "folder with spaces/file ; $literal `name`.md"
        self.write(name, b"literal filename\n")
        executable = self.write("run.sh", b"#!/bin/sh\nexit 0\n")
        executable.chmod(0o755)
        (self.repo / "contained").symlink_to("folder with spaces")
        self.write(".gitattributes", b"page.md export-ignore\nsubst.md export-subst\n")
        self.write("subst.md", b"$Format:%H$\n")
        commit = self.commit()
        self.snapshot("--ref", commit)
        manifest = self.manifest("ref", commit)
        self.assertEqual((self.output / name).read_bytes(), b"literal filename\n")
        self.assertEqual((self.output / "subst.md").read_bytes(), b"$Format:%H$\n")
        self.assertTrue((self.output / "page.md").is_file())
        self.assertEqual(os.readlink(self.output / "contained"), "folder with spaces")
        self.assertEqual(manifest["files"]["run.sh"]["mode"], "100755")
        self.assertEqual(manifest["files"]["contained"]["mode"], "120000")

    def test_untracked_ignored_and_traversal_rejected(self):
        self.write(".gitignore", b"secret.txt\n")
        self.write("secret.txt", b"ignored fixture\n")
        (self.root / "outside.txt").write_bytes(b"outside fixture\n")
        for index, selection in enumerate(("secret.txt", "../outside.txt", str(self.root / "outside.txt"))):
            with self.subTest(selection=selection):
                self.output = self.root / f"rejected-{index}"
                self.snapshot("--working-tree", "--include-untracked", selection, success=False)
                self.assertFalse(self.manifest_path.exists())

    def test_unmerged_index_rejected(self):
        blob = self.git("rev-parse", "HEAD:page.md").strip()
        self.git("update-index", "--force-remove", "page.md")
        entries = "".join(f"100644 {blob} {stage}\tpage.md\n" for stage in (1, 2, 3))
        subprocess.run(["git", "-C", str(self.repo), "update-index", "--index-info"],
                       input=entries, text=True, check=True, env=self.env)
        self.snapshot("--working-tree", success=False)
        self.assertFalse(self.manifest_path.exists())

    def test_escaping_links_rejected_in_both_modes(self):
        (self.repo / "escape").symlink_to("../outside")
        commit = self.commit()
        for index, selection in enumerate((("--ref", commit), ("--working-tree",))):
            with self.subTest(selection=selection):
                self.output = self.root / f"escape-{index}"
                self.snapshot(*selection, success=False)
                self.assertFalse(self.manifest_path.exists())

    def test_existing_output_or_manifest_never_overwritten(self):
        for index, kind in enumerate(("directory", "file", "symlink", "manifest")):
            with self.subTest(kind=kind):
                self.output = self.root / f"existing-{index}"
                existing = self.manifest_path if kind == "manifest" else self.output
                if kind == "directory":
                    existing.mkdir()
                    (existing / "sentinel").write_bytes(b"preserve\n")
                elif kind == "symlink":
                    existing.symlink_to("missing-target")
                else:
                    existing.write_bytes(b"preserve\n")
                before = self.tree_state(self.root)
                self.snapshot("--ref", self.base, success=False)
                after = self.tree_state(self.root)
                for path, value in before.items():
                    self.assertEqual(after.get(path), value, path)

    def test_gitlink_pin_recorded_without_expansion(self):
        self.git("update-index", "--add", "--cacheinfo", f"160000,{self.base},vendor/module")
        self.git("commit", "-qm", "gitlink fixture")
        commit = self.git("rev-parse", "HEAD").strip()
        self.write("vendor/module/private.txt", b"must not expand\n")
        for index, selection in enumerate((("--ref", commit), ("--working-tree",))):
            with self.subTest(selection=selection):
                self.output = self.root / f"gitlink-{index}"
                self.snapshot(*selection)
                manifest = self.manifest("ref" if index == 0 else "working-tree", commit)
                self.assertEqual(manifest["submodules"], [{"path": "vendor/module", "commit": self.base}])
                self.assertFalse((self.output / "vendor/module/private.txt").exists())

    def test_check_links_contains_links_and_rejects_escape_and_root_link(self):
        tree = self.root / "link-tree"
        tree.mkdir()
        (tree / "target").write_bytes(b"local\n")
        (tree / "contained").symlink_to("target")
        self.run_helper("--check-links", tree)
        (tree / "escape").symlink_to("../outside")
        self.run_helper("--check-links", tree, success=False)
        root_link = self.root / "root-link"
        root_link.symlink_to(tree, target_is_directory=True)
        self.run_helper("--check-links", root_link, success=False)


if __name__ == "__main__":
    unittest.main()
