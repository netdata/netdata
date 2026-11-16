#!/usr/bin/env python3
"""Export local preview inputs without changing the source checkout."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shlex
import stat
import subprocess
import sys


# Repository-local variables listed by `git rev-parse --local-env-vars`, plus
# inline config entries. Keep global/system config controls (including test isolation).
GIT_LOCAL_ENV = {
    "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
    "GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE", "GIT_GRAFT_FILE",
    "GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX",
    "GIT_SHALLOW_FILE", "GIT_COMMON_DIR",
}
GIT_ENV = {key: value for key, value in os.environ.items()
           if key not in GIT_LOCAL_ENV and not key.startswith(("GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"))}
GIT_ENV["GIT_OPTIONAL_LOCKS"] = "0"


class SnapshotError(Exception):
    pass


def git_command(repo, *args):
    command = ["git", "-C", str(repo), *args]
    print("Running: " + shlex.join(command), file=sys.stderr)
    return command


def git_read(repo, *args):
    command = git_command(repo, *args)
    result = subprocess.run(command, stdout=subprocess.PIPE, env=GIT_ENV)
    if result.returncode:
        print(f"Git operation failed in {repo}: status {result.returncode}", file=sys.stderr)
        raise subprocess.CalledProcessError(result.returncode, command)
    return result.stdout


def relative_path(value):
    path = PurePosixPath(value)
    if path.is_absolute() or not path.parts or ".." in path.parts or path.parts[0] == ".git":
        raise SnapshotError("Expected a repository-relative file path")
    return path.as_posix()


def entries_from(raw, working):
    entries = {}
    for record in raw.split(b"\0"):
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        first, second, third = metadata.decode("ascii").split()
        if working:
            mode, oid, stage = first, second, third
            if stage != "0":
                raise SnapshotError("Resolve index conflicts before taking a working-tree snapshot")
        else:
            mode, oid = first, third
        path = relative_path(os.fsdecode(raw_path))
        if mode not in {"100644", "100755", "120000", "160000"}:
            raise SnapshotError(f"Unsupported Git mode {mode} at {path}")
        entries[path] = (mode, oid)
    return entries


def check_links(root):
    root = Path(root)
    if root.is_symlink() or not root.is_dir():
        raise SnapshotError("Link-check root must be a real directory")
    root = root.resolve()
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(directory) / name
            if not path.is_symlink():
                continue
            relative = path.relative_to(root)
            try:
                target = path.resolve()
            except (OSError, RuntimeError) as error:
                raise SnapshotError(f"Cannot resolve symlink at {relative}") from error
            if not target.is_relative_to(root):
                raise SnapshotError(f"Symlink escapes snapshot at {relative}")


def copy_stream(source, destination, mode, size=None):
    digest = hashlib.sha256()
    remaining = size
    with destination.open("xb") as target:
        while remaining is None or remaining:
            chunk = source.read(1024 * 1024 if remaining is None else min(1024 * 1024, remaining))
            if not chunk:
                if remaining:
                    raise SnapshotError("Unexpected end of Git blob")
                break
            target.write(chunk)
            digest.update(chunk)
            if remaining is not None:
                remaining -= len(chunk)
    destination.chmod(0o755 if mode == "100755" else 0o644)
    return {"sha256": digest.hexdigest(), "mode": mode}


def write_link(destination, target):
    destination.symlink_to(os.fsdecode(target))
    return {"sha256": hashlib.sha256(target).hexdigest(), "mode": "120000"}


def committed_files(repo, entries, output):
    records = {}
    process = subprocess.Popen(git_command(repo, "cat-file", "--batch"), stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, env=GIT_ENV)
    try:
        for path, (mode, oid) in entries.items():
            if mode == "160000":
                continue
            process.stdin.write(oid.encode("ascii") + b"\n")
            process.stdin.flush()
            header = process.stdout.readline().split()
            if len(header) != 3 or header[1] != b"blob":
                raise SnapshotError(f"Expected a Git blob at {path}")
            size = int(header[2])
            destination = output / path
            destination.parent.mkdir(parents=True, exist_ok=True)
            if mode == "120000":
                target = process.stdout.read(size)
                if len(target) != size:
                    raise SnapshotError(f"Incomplete symlink blob at {path}")
                records[path] = write_link(destination, target)
            else:
                records[path] = copy_stream(process.stdout, destination, mode, size)
            if process.stdout.read(1) != b"\n":
                raise SnapshotError(f"Invalid Git batch framing at {path}")
        process.stdin.close()
        status = process.wait()
        if status:
            print(f"Git cat-file failed in {repo}: status {status}", file=sys.stderr)
            raise subprocess.CalledProcessError(status, process.args)
    finally:
        if process.poll() is None:
            process.kill()  # Only this helper-owned child; it may be blocked writing a rejected blob.
            process.wait()
        if not process.stdin.closed:
            process.stdin.close()
        process.stdout.close()
    return records


def file_stamp(path):
    info = path.lstat()
    return (info.st_dev, info.st_ino, info.st_mode, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


def working_files(repo, entries, selected, output):
    records, missing, stamps = {}, [], {}
    for path in sorted(set(entries) | set(selected)):
        if path in entries and entries[path][0] == "160000":
            continue
        source = repo / path
        for parent in source.parents:
            if parent == repo:
                break
            if parent.is_symlink():
                raise SnapshotError(f"Source parent is a symlink at {path}")
        try:
            before = file_stamp(source)
        except (FileNotFoundError, NotADirectoryError):
            if path in selected:
                raise SnapshotError(f"Selected untracked file disappeared: {path}")
            missing.append(path)
            continue
        destination = output / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        if stat.S_ISLNK(before[2]):
            records[path] = write_link(destination, os.fsencode(os.readlink(source)))
        elif stat.S_ISREG(before[2]):
            mode = "100755" if before[2] & 0o111 else "100644"
            descriptor = os.open(source, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
            with os.fdopen(descriptor, "rb") as stream:
                if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                    raise SnapshotError(f"Source is not a regular file at {path}")
                records[path] = copy_stream(stream, destination, mode)
        else:
            raise SnapshotError(f"Source is not a file or symlink at {path}")
        stamps[path] = before
    for path, before in stamps.items():
        if file_stamp(repo / path) != before:
            raise SnapshotError(f"Source changed during snapshot at {path}; retry in a fresh output")
    return records, missing


def snapshot(args):
    repo = Path(args.repo).resolve(strict=True)
    repo = Path(os.fsdecode(git_read(repo, "rev-parse", "--show-toplevel")).strip()).resolve()
    revision = args.ref if args.ref is not None else "HEAD"
    commit = git_read(repo, "rev-parse", "--verify", "--end-of-options", revision + "^{commit}").decode().strip()
    if args.working_tree:
        index = git_read(repo, "ls-files", "--stage", "-z")
        entries = entries_from(index, True)
    else:
        entries = entries_from(git_read(repo, "ls-tree", "-rz", "--full-tree", commit), False)
    selected = sorted({relative_path(path) for path in args.include_untracked})
    if selected:
        eligible = {os.fsdecode(p) for p in git_read(repo, "ls-files", "--others", "--exclude-standard", "-z").split(b"\0") if p}
        for path in selected:
            if path not in eligible:
                raise SnapshotError(f"Not an eligible nonignored untracked file: {path}")
    output = Path(args.output).absolute()
    manifest = output.with_name(output.name + ".manifest.json")
    if os.path.lexists(output) or os.path.lexists(manifest):
        raise SnapshotError("Output directory and manifest must both be new; existing contents were preserved")
    output.parent.resolve(strict=True)
    print(f"Creating snapshot directory: {output}", file=sys.stderr)
    output.mkdir(mode=0o700)
    if args.working_tree:
        files, missing = working_files(repo, entries, selected, output)
        if git_read(repo, "ls-files", "--stage", "-z") != index:
            raise SnapshotError("Index changed during snapshot; retry in a fresh output")
        if git_read(repo, "rev-parse", "--verify", "HEAD").decode().strip() != commit:
            raise SnapshotError("HEAD changed during snapshot; retry in a fresh output")
    else:
        files, missing = committed_files(repo, entries, output), []
    check_links(output)
    record = {"mode": "working-tree" if args.working_tree else "ref", "commit": commit,
              "files": files, "missing_tracked": missing, "selected_untracked": selected,
              "submodules": [{"path": path, "commit": oid} for path, (mode, oid) in entries.items() if mode == "160000"]}
    descriptor = os.open(manifest, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
        json.dump(record, stream, sort_keys=True, indent=2)
        stream.write("\n")
    print(manifest)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path)
    parser.add_argument("--output", type=Path)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--ref")
    mode.add_argument("--working-tree", action="store_true")
    mode.add_argument("--check-links", type=Path)
    parser.add_argument("--include-untracked", action="append", default=[])
    args = parser.parse_args()
    if args.check_links is not None:
        if args.repo or args.output or args.include_untracked:
            parser.error("--check-links takes only the tree to inspect")
    elif args.repo is None or args.output is None:
        parser.error("snapshot mode requires --repo and --output")
    if args.include_untracked and not args.working_tree:
        parser.error("--include-untracked requires --working-tree")
    try:
        if args.check_links is not None:
            check_links(args.check_links)
        else:
            snapshot(args)
    except subprocess.CalledProcessError as error:
        return error.returncode if error.returncode > 0 else 1
    except (SnapshotError, OSError, ValueError) as error:
        print(f"Snapshot failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
