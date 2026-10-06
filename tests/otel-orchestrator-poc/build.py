#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Build the local POC pair and real nd-run, or a Linux install pair."""

import argparse
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
BUILD = ROOT / ".build"
BUILDINFO = "github.com/netdata/netdata/go/plugins/pkg/buildinfo"
SECRET = re.compile(r"(?i)(token|password|secret|credential|authorization)")


def display(command):
    """Redact credential arguments and URL components without changing execution."""
    result = []
    hide_next = False
    for value in map(str, command):
        key, sep, _ = value.partition("=")
        if hide_next or "://" in value:
            result.append("[redacted]")
        elif SECRET.search(key):
            result.append(key + "=[redacted]" if sep else key)
        else:
            result.append(value)
        hide_next = bool(SECRET.search(key) and not sep and value.startswith("-"))
    return shlex.join(result)


def run(command, cwd=ROOT, env=None):
    print(f"+ {display(command)} (cwd: {cwd})", file=sys.stderr, flush=True)
    result = subprocess.run(command, cwd=cwd, env=env, stdout=sys.stderr, stderr=sys.stderr)
    if result.returncode:
        print(f"build operation failed in {cwd}, status {result.returncode}: {display(command)}",
              file=sys.stderr)
        raise SystemExit(result.returncode)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", choices=("build", "linux-amd64", "linux-arm64"), nargs="?", default="build")
    args = parser.parse_args()
    BUILD.mkdir(exist_ok=True)
    go = os.environ.get("GO", "go")
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    # OCB runs on the build host, even when producing the cross-compiled pair.
    run([go, "run", "go.opentelemetry.io/collector/cmd/builder@v0.157.0",
         "--config", "builder-config.yaml"], env=env)
    if args.target.startswith("linux-"):
        output = BUILD / args.target
        output.mkdir(exist_ok=True)
        env.update(GOOS="linux", GOARCH=args.target.removeprefix("linux-"))
        run([go, "build", "-trimpath", "-o", str(output / "otel-worker"), "."],
            cwd=BUILD / "collector", env=env)
        bindir = "/usr/sbin"
    else:
        output = BUILD
        shutil.copy2(BUILD / "collector/otel-worker", BUILD / "otel-worker")
        helperdir = BUILD / "bin"
        helperdir.mkdir(exist_ok=True)
        config = BUILD / "include"
        config.mkdir(exist_ok=True)
        definitions = '#define _GNU_SOURCE 1\n#define NETDATA_USER "nobody"\n'
        if sys.platform.startswith(("linux", "freebsd", "openbsd")):
            definitions += "#define HAVE_SETRESUID 1\n#define HAVE_SETRESGID 1\n"
        (config / "config.h").write_text(definitions)
        sources = REPO / "src/collectors/utils"
        run([*shlex.split(os.environ.get("CC", "cc")), "-std=gnu11", "-Wall", "-Wextra",
             "-Werror", "-O2", "-I", str(config), "-I", str(sources),
             *[str(sources / name) for name in ("nd-run.c", "nd-file-reader.c", "nd-process-tree.c")],
             "-o", str(helperdir / "nd-run")])
        bindir = str(helperdir)
    # Go's linker argument parser needs its own quotes when the checkout has spaces.
    ldflags = shlex.join(["-X", f"{BUILDINFO}.NetdataBinDir={bindir}"])
    run([go, "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags,
         "-o", str(output / "otel-orchestrator.plugin"), "./cmd/otelcolpocplugin"],
        cwd=REPO / "src/go", env=env)


if __name__ == "__main__":
    main()
