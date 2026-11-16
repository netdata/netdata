#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Verify pinned browser assets; --upstream additionally reads public archives."""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import urllib.request


def check(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def download(url):
    print(f"Read {url}", file=sys.stderr)
    with urllib.request.urlopen(url, timeout=30) as response:
        return response.read()


def archive(entry, cache):
    url = entry.get("tarball", entry.get("url"))
    if url not in cache:
        raw = download(url)
        algorithm, expected = entry["integrity"].split("-", 1)
        actual = base64.b64encode(hashlib.new(algorithm, raw).digest()).decode()
        check(actual == expected, f"Tarball integrity mismatch: {url}")
        # Inspect members in memory; never extract files or execute package scripts.
        cache[url] = tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz")
    return cache[url]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upstream", action="store_true", help="also download and verify pinned public sources")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    manifest = json.loads((root / "manifest.json").read_text())
    notices = (root / "NOTICES.txt").read_bytes()
    check(sha256(notices) == manifest["notices_sha256"], "Local notices digest mismatch")
    cache = {}
    for bundle in manifest["bundles"]:
        content = (root / bundle["file"]).read_bytes()
        check(len(content) == bundle["bytes"], f"Size mismatch: {bundle['file']}")
        check(sha256(content) == bundle["sha256"], f"Digest mismatch: {bundle['file']}")
        if args.upstream:
            upstream = archive(bundle, cache)
            check(upstream.extractfile(bundle["member"]).read() == content, f"Published bytes differ: {bundle['file']}")
            source_map = upstream.extractfile(bundle["source_map_member"]).read()
            check(sha256(source_map) == bundle["source_map_sha256"], "Source map digest mismatch")
            check(json.loads(source_map)["sources"] == bundle["sources"], "Source inventory mismatch")
    if args.upstream:
        check(sha256(download(manifest["lockfile_url"])) == manifest["lockfile_sha256"], "Lockfile digest mismatch")
        for dependency in manifest["dependencies"]:
            upstream = archive(dependency, cache)
            for member, digest in dependency["notices"].items():
                content = upstream.extractfile(member).read()
                check(sha256(content) == digest, f"License digest mismatch: {dependency['name']} {member}")
                check(content in notices, f"Missing license text: {dependency['name']} {member}")
        for entry in manifest["supplemental_notices"]:
            content = archive(entry, cache).extractfile(entry["member"]).read()
            check(sha256(content) == entry["sha256"], "Supplemental notice digest mismatch")
            check(content in notices, "Missing supplemental notice text")
    print("Pinned Faro assets and notices verified" + (" against upstream sources" if args.upstream else " locally"))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, tarfile.TarError) as error:
        print(f"Asset verification failed (working directory: {Path.cwd()}): {error}", file=sys.stderr)
        sys.exit(1)
