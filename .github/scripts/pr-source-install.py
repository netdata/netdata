#!/usr/bin/env python3
"""Bind a successful PR source installation to the archives in the final R2 layout."""

import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

KICKSTART = Path("packaging/installer/kickstart.sh")
OPTIONS = ["--build-only", "--dont-start-it", "--disable-telemetry", "--dont-wait"]
ALIASES = ("netdata-latest.tar.gz", "netdata-latest.tar.zst")


def acquire_without_rebuilding(directory):
    # Exercise kickstart's real HTTP selection, checksum, extraction and installer discovery.
    # At the build boundary the early job has already tested compilation and installation.
    before, marker, _ = KICKSTART.read_text().partition("\n# Main program\n")
    if not marker:
        sys.exit("Cannot locate the kickstart entry point")
    probe = directory / "kickstart.sh"
    reached = directory / "ready-to-build"
    download = directory / "download"
    download.mkdir()
    probe.write_text(before + """
install_local_build_dependencies() { return 0; }
build_and_install() { test -x ./netdata-installer.sh && touch "${PR_SOURCE_READY}"; }
try_build_install || exit 1
trap - EXIT
""")
    subprocess.run(["sh", str(probe)], check=True, cwd=download,
                   env={**os.environ, "PR_SOURCE_READY": str(reached),
                        "tmpdir": str(download), "NO_CLEANUP": "1"})
    if not reached.is_file():
        sys.exit("Kickstart did not reach the extracted source installer")


def source_identity(container):
    # httpd serves this job's immutable latest/ directory. Read its access log after kickstart exits.
    log = subprocess.check_output(["docker", "logs", container], stderr=subprocess.STDOUT, text=True)
    gets = re.findall(r'"GET /latest/(netdata-[^ /"]+\.tar\.(?:gz|zst)) HTTP/[^"]+" 200 (\d+)', log)
    if len(gets) != 1 or gets[0][0] not in ALIASES:
        sys.exit(f"Expected one successful source archive GET, found {gets}")

    latest = Path("latest")
    selected, sent = gets[0]
    if int(sent) != (latest / selected).stat().st_size:
        sys.exit(f"HTTP response for {selected} sent only {sent} bytes")
    manifest = (latest / "Manifest").read_text().splitlines()
    rows = [line for line in manifest if re.match(r"^netdata-.*\.tar\.(gz|zst) ", line)]
    actual = []
    for name in ALIASES:
        archive = latest / name
        with archive.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
        actual.append(f"{name} {archive.stat().st_size} {digest}")
    if sorted(rows) != actual:
        sys.exit(f"Source manifest does not match the two served archives: {rows}")
    return " | ".join([f"GET {selected}", *actual])


def main(mode, container):
    if mode == "install":
        subprocess.run(["sh", "-x", str(KICKSTART), *OPTIONS], check=True)
    elif mode == "verify":
        with tempfile.TemporaryDirectory() as temp:
            acquire_without_rebuilding(Path(temp))
    else:
        sys.exit(f"Unknown source check mode: {mode}")

    identity = source_identity(container)
    print(f"Served source: {identity}", flush=True)
    if mode == "install":
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            print(f"identity={identity}", file=output)
    elif identity != os.environ["TESTED_SOURCE"]:
        sys.exit(f"Final source differs from the tested source: {os.environ['TESTED_SOURCE']}")


if __name__ == "__main__":
    main(*sys.argv[1:])
