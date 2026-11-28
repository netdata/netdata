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


def source_scope(status, path):
    if path == "README.md" or (
        path.endswith(".md") and path.startswith(("docs/", "src/crates/", "packaging/"))
    ):
        # Removed or replaced documentation may be a required packaging resource.
        return None if status == "M" else "ordinary"
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


def package_scope(changes):
    scopes = {source_scope(status, path) for status, path in changes}
    scopes.discard(None)
    if scopes == {"tool"}:
        return "skip"
    if scopes == {"netflow"} or scopes == {"otel"}:
        return "representative"
    return "ordinary"


if __name__ == "__main__":
    base, head = sys.argv[1:]
    # Keep both sides of renames, type changes and submodule pointer changes.
    changed = subprocess.check_output([
        "git", "diff", "--no-renames", "--ignore-submodules=none", "--name-status",
        "-z", f"{base}...{head}", "--",
    ])
    fields = iter(os.fsdecode(changed).split("\0")[:-1])
    print(package_scope(zip(fields, fields, strict=True)))
