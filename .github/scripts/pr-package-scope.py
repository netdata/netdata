#!/usr/bin/env python3
"""Select native package coverage from a cumulative pull-request diff."""
import os
import subprocess
import sys

# Runtime and storage crates used by otel-plugin. Keep shared plugin SDK crates
# on the ordinary matrix, and leave new or unclassified crates there by default.
OTEL_CRATES = {
    "chunk-file", "ferryboat", "file-cache", "file-lifecycle", "file-registry",
    "flatten-otel", "ng-flatten", "ng-index", "otel-bridge", "otel-catalog",
    "otel-derived-metrics", "otel-ingestor", "otel-job-scheduler", "otel-ledger",
    "otel-logs-identity", "otel-plugin", "otel-service-map", "sfsq", "sfsq-cli",
    "sfst", "treight", "wal",
}


def source_scope(path):
    if path == "README.md" or (
        path.endswith(".md") and path.startswith(("docs/", "src/crates/", "packaging/"))
    ):
        return None
    if path.startswith("src/crates/netflow-plugin/") and path.endswith(".rs"):
        return "netflow"
    if path.startswith("src/crates/") and path.endswith(".rs"):
        crate, _, relative = path.removeprefix("src/crates/").partition("/")
        if relative.startswith(("src/", "tests/")):
            if crate == "otel-streams":
                return "tool"
            if crate in OTEL_CRATES:
                return "otel"
    return "ordinary"


def package_scope(paths):
    scopes = {scope for path in paths if (scope := source_scope(path)) is not None}
    if scopes == {"tool"}:
        return "skip"
    if scopes == {"netflow"} or scopes == {"otel"}:
        return "representative"
    return "ordinary"


if __name__ == "__main__":
    base, head = sys.argv[1:]
    # Keep both sides of renames, type changes and submodule pointer changes.
    changed = subprocess.check_output([
        "git", "diff", "--no-renames", "--ignore-submodules=none", "--name-only",
        "-z", f"{base}...{head}", "--",
    ])
    print(package_scope(os.fsdecode(p) for p in changed.split(b"\0") if p))
