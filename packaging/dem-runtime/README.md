# DEM portable runtime bundle

This release-owned builder produces one private Linux runtime for `amd64` or
`arm64`. Node, locked production npm dependencies, full Chromium, FFmpeg, the
private ELF loader/library closure, fonts, and notices travel together. Agent
runner adapters and `nd-run` remain outside the bundle.

Run from the repository root with Python 3.9 or newer and Docker:

```sh
python3 packaging/dem-runtime/build.py --arch arm64 \
  --output .local/audits/dem-runtime-delivery/build-arm64
python3 -m unittest discover -s packaging/dem-runtime -v
```

The output directory MUST be empty. The builder runs a disposable, architecture
selected Docker container; it installs nothing on the host. Building another
architecture requires a compatible Docker worker or emulation. An emulated build
is not native execution acceptance. Downloads and npm execute only during the
release build. Consumers MUST NOT run apt, npm, or upstream browser installers.

## Inputs and release identity

`inputs.json` pins an independent runtime version, deterministic timestamp,
architecture-specific immutable donor image manifests, Node release checksums,
and patchelf release checksums. The Playwright donor provides browser artifacts,
libraries, fonts, Python, and build utilities; no mutable apt repositories are
consulted. Node comes from its checksum-verified official release. npm uses
`src/go/plugin/dem/synthetic/runner/assets/package-lock.json`, with lifecycle
scripts, auditing, and funding network requests disabled. The builder verifies
installed versions and Chromium revision against the runner's `manifest.json`.
The two manifests and the npm lock are repository-owned inputs; changes require
rebuilding and requalifying the runtime.

The output contains:

- `<sha256>.tar.gz`: deterministic sorted tar entries, normalized owner/mode/time,
  gzip with fixed timestamp and no original filename;
- `manifest.json`: format 1 runtime identity, exact compressed/tar/regular-file
  bytes, entry count, layout, and authenticated ELF bindings;
- `provenance.json`: source/recipe hashes, complete lock inputs, and original
  private-library sources/hashes;
- `sbom.cdx.json`: CycloneDX 1.5 inventory of Node, Chromium, npm integrity pins,
  and copied Debian library/font package versions and source package identities.

The archive includes the SBOM/provenance, hashes and sizes for its other regular
files (`files.json`), Node's bundled license, npm's original notices, Chromium's
embedded third-party credits, and the owning distro packages' copyright files.
Fontconfig generates its own user cache; donor-generated X11 indexes are omitted.
The Chromium credits export runs in the root build container with Playwright's
build-time sandbox disabled. It only extracts embedded notices and MUST NOT be
counted as sandbox qualification. The prepared runtime launcher never disables
the browser sandbox.

Without `--url-prefix`, the manifest is offline-only (`archive.url` is empty).
After release owners establish an approved HTTPS endpoint, this explicit argument
constructs `<prefix>/dem-runtime/<version>/<architecture>/<digest>.tar.gz`.
The builder neither publishes artifacts nor overwrites remote objects. Review and
install the manifest with the Agent: a manifest fetched beside an archive MUST
NOT become the trust anchor. Archive identity excludes the publication URL.
Release publication, retention, security rebuild ownership, and a public endpoint
must be established separately before shipping download-enabled manifests.

## ELF preparation contract

The builder uses patchelf 0.15.0. Newer tested versions (0.18 and 0.19.1) corrupted
the pinned arm64 Node program-header layout in the portability investigation.
Version 0.15.0 is checksum-pinned and its output is checked; changing it requires
repeating execution qualification on both architectures.

Every dynamic payload object receives `$ORIGIN`-relative private library search
paths. The loader and libc themselves retain their donor ABI layout; libc is a
library, not a runtime execution entry point. Runtime-loaded NSS modules are
included explicitly in addition to the transitive `ldd` closure. Unresolved
libraries and colliding library basenames fail the build. Statically linked
FFmpeg requires no private dynamic-library closure.

Interpreter-bearing executables receive an exact **4096-byte PT_INTERP slot**.
Linux's `PATH_MAX` bounds the encoded absolute interpreter pathname to 4095 bytes
plus its terminating NUL. This deliberately accounts for the entire final
prefix, generation digest, `lib/`, and loader filename; individual path components
still have filesystem limits. The build parses ELF64 program headers, checks
architecture, file bounds and PHDR coverage by LOAD, then records the actual
PT_INTERP offset and complete expected bytes as base64. String searching cannot
select a binding. The build executes Node, Node re-execution, and Chromium after
binding copies under a prefix containing spaces/quotes and at the exact 4095-byte
interpreter-path limit. These are executable integrity checks, not sandbox tests.

The Agent verifies the archive's installed digest/bounds before extraction and
replaces only these exact slots with the FINAL immutable generation's loader
path, padded with NUL bytes. A longer path MUST fail before publication. No ELF
rearrangement, package installation, or arbitrary patching occurs on the host.
`chrome`, `fonts.conf`, and `prepared.json` are reserved generated files and MUST
NOT occur in the archive. The runtime store generates safely quoted/escaped final
prefix configuration. Internal symlinks are permitted; escaping links, special
files and privileged modes are not emitted.

## Qualification and maintenance

Unit tests cover deterministic archive metadata, independent size accounting,
reserved paths, escaping symlinks, true ELF offsets, malformed ELF bounds and the
known PHDR failure class. The command wrapper is checked for success, original
failure status, metacharacters/spaces, and synthetic credential redaction.

A release MUST also exercise the built archive through the actual runtime store,
Engine, and supervised `nd-run`, with sandbox enabled, on native amd64 and arm64.
Verify the supported kernel floor, native Ubuntu AppArmor/systemd context, all
installation methods, offline/read-only preloads, custom prefixes, and node/browser
re-execution. Docker tests establish their userland/kernel context only. Check
SBOM/notices and source availability obligations when updating any dependency;
retaining notices alone does not establish every redistribution obligation.

Repeat a build in the same pinned environment and compare archive/manifests to
establish reproducibility. A dependency security rebuild updates the independent
runtime version and affected pins, repeats qualification, and ships a new Agent
manifest. Existing immutable archives MUST NOT be replaced in place.
